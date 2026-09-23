package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSessionLifecycle(t *testing.T) {
	db, c := openTest(t)
	ctx := context.Background()
	u, _ := db.CreateUser(ctx, "sam", goodPass)

	s, err := db.CreateSession(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Token) < 40 || len(s.CSRF) < 40 || s.Token == s.CSRF {
		t.Fatalf("weak tokens: %+v", s)
	}
	got, err := db.LookupSession(ctx, s.Token)
	if err != nil || got.User.Username != "sam" || got.CSRF != s.CSRF || got.Token != s.Token {
		t.Fatalf("lookup = %+v, %v", got, err)
	}
	var stored []byte
	db.sql.QueryRow(`SELECT token_hash FROM sessions`).Scan(&stored)
	if string(stored) == s.Token {
		t.Fatal("raw token stored")
	}
	if _, err := db.LookupSession(ctx, "nope"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("unknown = %v", err)
	}
	if _, err := db.LookupSession(ctx, ""); !errors.Is(err, ErrNoSession) {
		t.Fatalf("empty = %v", err)
	}

	// Sliding: activity after 29 days pushes expiry out again.
	c.advance(29 * 24 * time.Hour)
	if _, err := db.LookupSession(ctx, s.Token); err != nil {
		t.Fatal("should still be valid at day 29:", err)
	}
	c.advance(29 * 24 * time.Hour)
	if _, err := db.LookupSession(ctx, s.Token); err != nil {
		t.Fatal("sliding refresh should keep it alive at day 58:", err)
	}
	// Idle past TTL: gone.
	c.advance(SessionTTL + time.Minute)
	if _, err := db.LookupSession(ctx, s.Token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("expired = %v", err)
	}
}

func TestDeleteAndPruneSessions(t *testing.T) {
	db, c := openTest(t)
	ctx := context.Background()
	u, _ := db.CreateUser(ctx, "sam", goodPass)
	a, _ := db.CreateSession(ctx, u)
	b, _ := db.CreateSession(ctx, u)
	if err := db.DeleteSession(ctx, a.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := db.LookupSession(ctx, a.Token); !errors.Is(err, ErrNoSession) {
		t.Fatal("deleted session still valid")
	}
	c.advance(SessionTTL + time.Hour)
	n, err := db.PruneSessions(ctx)
	if err != nil || n != 1 {
		t.Fatalf("pruned = %d, %v", n, err)
	}
	_ = b
}

func TestChangePasswordRevokesOthers(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	u, _ := db.CreateUser(ctx, "sam", goodPass)
	keep, _ := db.CreateSession(ctx, u)
	other, _ := db.CreateSession(ctx, u)

	if err := db.ChangePassword(ctx, u, "not the password", "another good passphrase", keep.Token); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("wrong current = %v", err)
	}
	if err := db.ChangePassword(ctx, u, goodPass, "short", keep.Token); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("weak next = %v", err)
	}
	if err := db.ChangePassword(ctx, u, goodPass, "another good passphrase", keep.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := db.LookupSession(ctx, keep.Token); err != nil {
		t.Fatal("current session should survive:", err)
	}
	if _, err := db.LookupSession(ctx, other.Token); !errors.Is(err, ErrNoSession) {
		t.Fatal("other session should be revoked")
	}
}

func TestDeleteUserCascadesSessions(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	u, _ := db.CreateUser(ctx, "sam", goodPass)
	s, _ := db.CreateSession(ctx, u)
	if err := db.DeleteUser(ctx, "sam"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.LookupSession(ctx, s.Token); !errors.Is(err, ErrNoSession) {
		t.Fatal("session outlived its user")
	}
}
