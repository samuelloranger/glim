package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ViewStat is how often a preview was opened and when it last was.
type ViewStat struct {
	Count    int64
	LastSeen time.Time
}

// RecordView counts one open of the named preview.
func (db *DB) RecordView(ctx context.Context, name string) error {
	_, err := db.sql.ExecContext(ctx, `
		INSERT INTO preview_views(name, count, last_seen) VALUES (?, 1, ?)
		ON CONFLICT(name) DO UPDATE SET count = count + 1, last_seen = excluded.last_seen`,
		name, db.now().Unix())
	return err
}

// ViewStats returns the stats of every preview that has been opened.
func (db *DB) ViewStats(ctx context.Context) (map[string]ViewStat, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT name, count, last_seen FROM preview_views`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ViewStat{}
	for rows.Next() {
		var name string
		var count, ts int64
		if err := rows.Scan(&name, &count, &ts); err != nil {
			return nil, err
		}
		out[name] = ViewStat{Count: count, LastSeen: time.Unix(ts, 0).UTC()}
	}
	return out, rows.Err()
}

// DeleteViews forgets a preview's stats (it was removed or expired).
func (db *DB) DeleteViews(ctx context.Context, name string) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM preview_views WHERE name = ?`, name)
	return err
}

// OwnerCookie is the cookie that marks the dashboard owner on preview paths.
const OwnerCookie = "glim_owner"

// --- owner cookie ---

type ownerKeyCache struct {
	mu  sync.Mutex
	key []byte
}

// ownerKey returns the server secret behind the owner cookie, generating and
// persisting it on first use. It is cached, so verifying costs no query.
func (db *DB) ownerKey() ([]byte, error) {
	c := &db.owner
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.key != nil {
		return c.key, nil
	}
	ctx := context.Background()
	fresh := make([]byte, 32)
	if _, err := rand.Read(fresh); err != nil {
		return nil, err
	}
	if _, err := db.sql.ExecContext(ctx,
		`INSERT OR IGNORE INTO settings(key, value) VALUES ('owner_secret', ?)`, fresh); err != nil {
		return nil, err
	}
	var key []byte
	err := db.sql.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'owner_secret'`).Scan(&key)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	c.key = key
	return key, nil
}

func ownerMAC(key []byte, uid string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(uid))
	return hex.EncodeToString(m.Sum(nil))
}

// OwnerToken is the value of the glim_owner cookie for a user: "<id>.<hmac>".
func (db *DB) OwnerToken(userID int64) (string, error) {
	key, err := db.ownerKey()
	if err != nil {
		return "", err
	}
	uid := strconv.FormatInt(userID, 10)
	return uid + "." + ownerMAC(key, uid), nil
}

// IsOwnerToken reports whether v was minted by OwnerToken under this
// server's secret. It does not touch the database after the first call.
func (db *DB) IsOwnerToken(v string) bool {
	uid, mac, ok := strings.Cut(v, ".")
	if !ok || uid == "" {
		return false
	}
	key, err := db.ownerKey()
	if err != nil || len(key) == 0 {
		return false
	}
	return hmac.Equal([]byte(mac), []byte(ownerMAC(key, uid)))
}

// UnlockToken is the value of a preview's unlock cookie: an HMAC under the
// server secret of the slug and its current password hash, so changing or
// removing the password invalidates every earlier unlock.
func (db *DB) UnlockToken(slug, passwordHash string) string {
	key, err := db.ownerKey()
	if err != nil || len(key) == 0 {
		return ""
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte("unlock\x00" + slug + "\x00" + passwordHash))
	return hex.EncodeToString(m.Sum(nil))
}

// --- asynchronous recording ---

// ViewRecorder writes view counts from a background goroutine so serving a
// preview never waits on SQLite. Records are dropped if the queue is full.
type ViewRecorder struct {
	db      *DB
	ch      chan string
	done    chan struct{}
	mu      sync.RWMutex
	closed  bool
	onWrite func()
}

// NewViewRecorder starts the writer. onWrite (optional) runs after each stored
// view and must not block.
func (db *DB) NewViewRecorder(onWrite func()) *ViewRecorder {
	r := &ViewRecorder{db: db, ch: make(chan string, 256), done: make(chan struct{}), onWrite: onWrite}
	go func() {
		defer close(r.done)
		for name := range r.ch {
			if err := db.RecordView(context.Background(), name); err == nil && r.onWrite != nil {
				r.onWrite()
			}
		}
	}()
	return r
}

func (r *ViewRecorder) Record(name string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return
	}
	select {
	case r.ch <- name:
	default:
	}
}

// Close drains queued views and stops the writer.
func (r *ViewRecorder) Close() {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.ch)
	}
	r.mu.Unlock()
	<-r.done
}
