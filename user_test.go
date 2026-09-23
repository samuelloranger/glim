package main

import (
	"bufio"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samuelloranger/glim/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

func testDB(t *testing.T) *auth.DB {
	t.Helper()
	db, err := auth.Open(filepath.Join(t.TempDir(), "glim.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.BcryptCost = bcrypt.MinCost
	t.Cleanup(func() { db.Close() })
	return db
}

func run(t *testing.T, db *auth.DB, input string, args ...string) (string, error) {
	t.Helper()
	var out strings.Builder
	err := runUser(args, db, bufio.NewReader(strings.NewReader(input)), &out)
	return out.String(), err
}

func TestUserCLI(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	if out, err := run(t, db, "", "ls"); err != nil || !strings.Contains(out, "no accounts") {
		t.Fatalf("empty ls = %q, %v", out, err)
	}
	if _, err := db.CreateUser(ctx, "sam@example.com", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	if out, _ := run(t, db, "", "ls"); !strings.Contains(out, "sam@example.com") {
		t.Fatalf("ls = %q", out)
	}
	if _, err := run(t, db, "new passphrase one\nmismatch passphrase\n", "passwd", "sam@example.com"); err == nil {
		t.Fatal("mismatched passwords accepted")
	}
	if _, err := run(t, db, "new passphrase one\nnew passphrase one\n", "passwd", "sam@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Authenticate(ctx, "sam@example.com", "new passphrase one"); err != nil {
		t.Fatal("passwd did not take:", err)
	}
	if _, err := run(t, db, "", "rm", "ghost@example.com"); err == nil {
		t.Fatal("rm ghost should fail")
	}
	if _, err := run(t, db, "", "rm", "sam@example.com"); err != nil {
		t.Fatal(err)
	}
	if n, _ := db.CountUsers(ctx); n != 0 {
		t.Fatalf("users = %d", n)
	}
	if _, err := run(t, db, "", "bogus"); err == nil {
		t.Fatal("unknown subcommand should fail")
	}
}
