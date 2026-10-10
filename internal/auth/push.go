package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"

	"github.com/SherClockHolmes/webpush-go"
)

// PushSub is one browser's web push subscription, owned by a dashboard user.
type PushSub struct {
	Endpoint string
	P256dh   string
	Auth     string
	UserID   int64
}

var vapidMu sync.Mutex

// VAPIDKeys returns the server's VAPID key pair (private, public), generating
// and persisting it in the settings table on first use. The pair must stay
// stable: every existing subscription is bound to the public key.
func (db *DB) VAPIDKeys(ctx context.Context) (private, public string, err error) {
	vapidMu.Lock()
	defer vapidMu.Unlock()
	read := func() (string, bool, error) {
		var v []byte
		err := db.sql.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'vapid_keys'`).Scan(&v)
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return string(v), err == nil, err
	}
	val, ok, err := read()
	if err != nil {
		return "", "", err
	}
	if !ok {
		priv, pub, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			return "", "", err
		}
		if _, err := db.sql.ExecContext(ctx,
			`INSERT OR IGNORE INTO settings(key, value) VALUES ('vapid_keys', ?)`, priv+"\n"+pub); err != nil {
			return "", "", err
		}
		if val, _, err = read(); err != nil {
			return "", "", err
		}
	}
	private, public, _ = strings.Cut(val, "\n")
	return private, public, nil
}

// ErrPushEndpointTaken means the endpoint is already subscribed by another user.
var ErrPushEndpointTaken = errors.New("push endpoint belongs to another account")

// SavePushSub stores a subscription. Re-subscribing the same endpoint as the
// same user refreshes its keys; an endpoint owned by another user is never
// moved, so one account can't take over or silence another's subscription.
func (db *DB) SavePushSub(ctx context.Context, s PushSub) error {
	res, err := db.sql.ExecContext(ctx,
		`INSERT INTO push_subscriptions(endpoint, p256dh, auth, user_id, created_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(endpoint) DO UPDATE SET p256dh = excluded.p256dh, auth = excluded.auth
		 WHERE push_subscriptions.user_id = excluded.user_id`,
		s.Endpoint, s.P256dh, s.Auth, s.UserID, db.now().Unix())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPushEndpointTaken
	}
	return nil
}

// DeletePushSub removes one user's subscription for an endpoint.
func (db *DB) DeletePushSub(ctx context.Context, userID int64, endpoint string) error {
	_, err := db.sql.ExecContext(ctx,
		`DELETE FROM push_subscriptions WHERE endpoint = ? AND user_id = ?`, endpoint, userID)
	return err
}

// DeletePushEndpoint removes a subscription the push service reported gone.
func (db *DB) DeletePushEndpoint(ctx context.Context, endpoint string) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE endpoint = ?`, endpoint)
	return err
}

// PushSubs lists every subscription.
func (db *DB) PushSubs(ctx context.Context) ([]PushSub, error) {
	rows, err := db.sql.QueryContext(ctx,
		`SELECT endpoint, p256dh, auth, user_id FROM push_subscriptions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PushSub
	for rows.Next() {
		var s PushSub
		if err := rows.Scan(&s.Endpoint, &s.P256dh, &s.Auth, &s.UserID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
