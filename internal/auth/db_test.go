package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func openTest(t *testing.T) (*DB, *clock) {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "sub", "glim.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c := &clock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	db.Now = c.now
	db.BcryptCost = bcrypt.MinCost
	return db, c
}

func TestOpenCreatesPrivateFileAndMigratesIdempotently(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub")
	path := filepath.Join(dir, "glim.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Errorf("db perm = %v, want 0600", st.Mode().Perm())
	}
	if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
		t.Errorf("dir perm = %v, want 0700", st.Mode().Perm())
	}
	db, err = Open(path) // second open must not re-run migrations
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("schema_migrations rows = %d, want 1", n)
	}
	for _, table := range []string{"users", "sessions"} {
		if _, err := db.sql.Exec(`SELECT 1 FROM ` + table + ` LIMIT 1`); err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}
}
