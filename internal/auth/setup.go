package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrSetupDone    = errors.New("setup already complete")
	ErrBadSetupCode = errors.New("setup code does not match")
)

// Unambiguous alphabet: no 0/O, 1/I/L, U.
const setupAlphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"
const setupCodeLen = 10

func NewSetupCode() string {
	b := make([]byte, setupCodeLen)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i, v := range b {
		b[i] = setupAlphabet[int(v)%len(setupAlphabet)]
	}
	return string(b)
}

func FormatSetupCode(code string) string {
	if len(code) != setupCodeLen {
		return code
	}
	return code[:4] + "-" + code[4:8] + "-" + code[8:]
}

func normalizeCode(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, strings.ToUpper(strings.TrimSpace(s)))
}

func ReadSetupCode(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	code := strings.TrimSpace(string(data))
	return code, code != ""
}

func (db *DB) EnsureSetupCode(ctx context.Context, path string) (string, error) {
	n, err := db.CountUsers(ctx)
	if err != nil {
		return "", err
	}
	if n > 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return "", err
		}
		return "", nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	code := NewSetupCode()
	if err := os.WriteFile(path, []byte(code+"\n"), 0o600); err != nil {
		return "", err
	}
	return code, nil
}

func (db *DB) CompleteSetup(ctx context.Context, path, code, username, password string) (User, error) {
	if n, err := db.CountUsers(ctx); err != nil {
		return User{}, err
	} else if n > 0 {
		return User{}, ErrSetupDone
	}
	want, ok := ReadSetupCode(path)
	if !ok || subtle.ConstantTimeCompare([]byte(normalizeCode(code)), []byte(normalizeCode(want))) != 1 {
		return User{}, ErrBadSetupCode
	}
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
	tx, err := db.sql.BeginTx(ctx, nil) // BEGIN IMMEDIATE via _txlock
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return User{}, err
	}
	if n > 0 {
		return User{}, ErrSetupDone
	}
	user, err := db.insertUser(ctx, tx, u, h)
	if err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	_ = os.Remove(path)
	return user, nil
}
