package auth

import (
	"context"
	"errors"
)

var ErrSetupDone = errors.New("setup already complete")

// CompleteSetup creates the first account. It succeeds only while no account
// exists; the check and the insert share one transaction, so concurrent
// first visits cannot both win.
func (db *DB) CompleteSetup(ctx context.Context, email, password string) (User, error) {
	e, err := NormalizeEmail(email)
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
	user, err := db.insertUser(ctx, tx, e, h)
	if err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return user, nil
}
