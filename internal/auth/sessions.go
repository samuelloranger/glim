package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"
)

const (
	SessionTTL = 30 * 24 * time.Hour
	touchEvery = time.Hour
)

var ErrNoSession = errors.New("no such session")

type Session struct {
	Token     string // raw cookie value; only its SHA-256 is stored
	CSRF      string
	User      User
	ExpiresAt time.Time
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func (db *DB) CreateSession(ctx context.Context, user User) (Session, error) {
	token, csrf := randomToken(32), randomToken(32)
	now := db.now().Unix()
	exp := now + int64(SessionTTL/time.Second)
	_, err := db.sql.ExecContext(ctx,
		`INSERT INTO sessions(token_hash, user_id, csrf, created_at, last_seen_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		hashToken(token), user.ID, csrf, now, now, exp)
	if err != nil {
		return Session{}, err
	}
	return Session{Token: token, CSRF: csrf, User: user, ExpiresAt: time.Unix(exp, 0).UTC()}, nil
}

func (db *DB) LookupSession(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrNoSession
	}
	th := hashToken(token)
	var s Session
	var lastSeen, exp, created int64
	err := db.sql.QueryRowContext(ctx, `
		SELECT s.csrf, s.last_seen_at, s.expires_at, u.id, u.username, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ?`, th).
		Scan(&s.CSRF, &lastSeen, &exp, &s.User.ID, &s.User.Username, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNoSession
	}
	if err != nil {
		return Session{}, err
	}
	now := db.now().Unix()
	if exp <= now {
		_, _ = db.sql.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, th)
		return Session{}, ErrNoSession
	}
	if now-lastSeen >= int64(touchEvery/time.Second) {
		exp = now + int64(SessionTTL/time.Second)
		if _, err := db.sql.ExecContext(ctx,
			`UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE token_hash = ?`, now, exp, th); err != nil {
			return Session{}, err
		}
	}
	s.Token = token
	s.User.CreatedAt = time.Unix(created, 0).UTC()
	s.ExpiresAt = time.Unix(exp, 0).UTC()
	return s, nil
}

func (db *DB) DeleteSession(ctx context.Context, token string) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	return err
}

func (db *DB) PruneSessions(ctx context.Context) (int64, error) {
	res, err := db.sql.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, db.now().Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ChangePassword verifies current, stores next, and signs the user out of
// every session except keepToken.
func (db *DB) ChangePassword(ctx context.Context, user User, current, next, keepToken string) error {
	if _, err := db.Authenticate(ctx, user.Username, current); err != nil {
		return err
	}
	if err := ValidatePassword(next); err != nil {
		return err
	}
	h, err := db.hash(next)
	if err != nil {
		return err
	}
	if _, err := db.sql.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, h, user.ID); err != nil {
		return err
	}
	_, err = db.sql.ExecContext(ctx,
		`DELETE FROM sessions WHERE user_id = ? AND token_hash != ?`, user.ID, hashToken(keepToken))
	return err
}
