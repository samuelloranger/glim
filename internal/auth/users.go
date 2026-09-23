package auth

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidUsername  = errors.New("invalid username")
	ErrPasswordTooShort = errors.New("password too short")
	ErrPasswordTooLong  = errors.New("password too long")
	ErrUserExists       = errors.New("username taken")
	ErrNoSuchUser       = errors.New("no such user")
	ErrBadCredentials   = errors.New("wrong username or password")
)

const (
	MinPasswordChars = 12
	MaxPasswordBytes = 72 // bcrypt's input limit; longer is rejected, never truncated
)

type User struct {
	ID        int64
	Username  string
	CreatedAt time.Time
}

var usernameRE = regexp.MustCompile(`^[a-z0-9._-]{1,32}$`)

func NormalizeUsername(s string) (string, error) {
	u := strings.ToLower(strings.TrimSpace(s))
	if !usernameRE.MatchString(u) {
		return "", ErrInvalidUsername
	}
	return u, nil
}

func ValidatePassword(p string) error {
	if len(p) > MaxPasswordBytes {
		return ErrPasswordTooLong
	}
	if utf8.RuneCountInString(p) < MinPasswordChars {
		return ErrPasswordTooShort
	}
	return nil
}

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (db *DB) hash(p string) (string, error) {
	cost := db.BcryptCost
	if cost == 0 {
		cost = DefaultBcryptCost
	}
	b, err := bcrypt.GenerateFromPassword([]byte(p), cost)
	return string(b), err
}

var (
	dummyOnce sync.Once
	dummyHash []byte
)

// dummyCompare spends the same bcrypt time as a real check so unknown
// usernames can't be told apart by response time.
func (db *DB) dummyCompare(password string) {
	dummyOnce.Do(func() {
		h, _ := db.hash("glim-dummy-password-for-timing")
		dummyHash = []byte(h)
	})
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}

func (db *DB) CreateUser(ctx context.Context, username, password string) (User, error) {
	u, err := NormalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return User{}, err
	}
	h, err := db.hash(password)
	if err != nil {
		return User{}, err
	}
	return db.insertUser(ctx, db.sql, u, h)
}

func (db *DB) insertUser(ctx context.Context, q querier, username, hash string) (User, error) {
	created := db.now().Unix()
	res, err := q.ExecContext(ctx,
		`INSERT INTO users(username, password_hash, created_at) VALUES (?, ?, ?)`, username, hash, created)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return User{}, ErrUserExists
		}
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return User{ID: id, Username: username, CreatedAt: time.Unix(created, 0).UTC()}, nil
}

func (db *DB) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (db *DB) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT id, username, created_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		var created int64
		if err := rows.Scan(&u.ID, &u.Username, &created); err != nil {
			return nil, err
		}
		u.CreatedAt = time.Unix(created, 0).UTC()
		out = append(out, u)
	}
	return out, rows.Err()
}

func (db *DB) GetUser(ctx context.Context, username string) (User, error) {
	u, err := NormalizeUsername(username)
	if err != nil {
		return User{}, ErrNoSuchUser
	}
	var user User
	var created int64
	err = db.sql.QueryRowContext(ctx,
		`SELECT id, username, created_at FROM users WHERE username = ?`, u).Scan(&user.ID, &user.Username, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNoSuchUser
	}
	if err != nil {
		return User{}, err
	}
	user.CreatedAt = time.Unix(created, 0).UTC()
	return user, nil
}

func (db *DB) Authenticate(ctx context.Context, username, password string) (User, error) {
	u, err := NormalizeUsername(username)
	if err != nil {
		db.dummyCompare(password)
		return User{}, ErrBadCredentials
	}
	var user User
	var hash string
	var created int64
	err = db.sql.QueryRowContext(ctx,
		`SELECT id, username, password_hash, created_at FROM users WHERE username = ?`, u).
		Scan(&user.ID, &user.Username, &hash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		db.dummyCompare(password)
		return User{}, ErrBadCredentials
	}
	if err != nil {
		return User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, ErrBadCredentials
	}
	user.CreatedAt = time.Unix(created, 0).UTC()
	return user, nil
}

func (db *DB) DeleteUser(ctx context.Context, username string) error {
	u, err := NormalizeUsername(username)
	if err != nil {
		return ErrNoSuchUser
	}
	res, err := db.sql.ExecContext(ctx, `DELETE FROM users WHERE username = ?`, u)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoSuchUser
	}
	return nil
}

// SetPassword replaces a user's password (operator recovery) and signs that
// user out everywhere.
func (db *DB) SetPassword(ctx context.Context, username, password string) error {
	user, err := db.GetUser(ctx, username)
	if err != nil {
		return err
	}
	if err := ValidatePassword(password); err != nil {
		return err
	}
	h, err := db.hash(password)
	if err != nil {
		return err
	}
	if _, err := db.sql.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, h, user.ID); err != nil {
		return err
	}
	_, err = db.sql.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, user.ID)
	return err
}
