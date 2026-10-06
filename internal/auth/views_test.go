package auth

import (
	"context"
	"testing"
	"time"
)

func TestViewsRecordStatsDelete(t *testing.T) {
	db, c := openTest(t)
	ctx := context.Background()
	if err := db.RecordView(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	c.advance(time.Minute)
	for i := 0; i < 2; i++ {
		if err := db.RecordView(ctx, "a"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.RecordView(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	st, err := db.ViewStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st["a"].Count != 3 || !st["a"].LastSeen.Equal(c.t) || st["b"].Count != 1 {
		t.Fatalf("stats = %+v", st)
	}
	if err := db.DeleteViews(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	st, _ = db.ViewStats(ctx)
	if _, ok := st["a"]; ok || len(st) != 1 {
		t.Fatalf("after delete = %+v", st)
	}
}

func TestOwnerToken(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	u, err := db.CreateUser(ctx, "owner@example.com", "hunter22!")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := db.CreateSession(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := db.OwnerToken(u.ID, sess.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !db.IsOwnerToken(tok) {
		t.Fatal("valid token rejected")
	}
	for _, bad := range []string{"", "1", "1.", ".abc", "1.ab.cd", "2" + tok[1:], tok + "0", tok[:len(tok)-1]} {
		if db.IsOwnerToken(bad) {
			t.Errorf("forged token %q accepted", bad)
		}
	}
	db2, _ := openTest(t)
	if db2.IsOwnerToken(tok) {
		t.Error("token from another database accepted")
	}
}

func TestOwnerTokenDiesWithSession(t *testing.T) {
	ctx := context.Background()
	mk := func(db *DB, email string) (User, Session, string) {
		u, err := db.CreateUser(ctx, email, "hunter22!")
		if err != nil {
			t.Fatal(err)
		}
		s, err := db.CreateSession(ctx, u)
		if err != nil {
			t.Fatal(err)
		}
		tok, err := db.OwnerToken(u.ID, s.Token)
		if err != nil {
			t.Fatal(err)
		}
		return u, s, tok
	}
	db, clk := openTest(t)
	_, s, tok := mk(db, "a@example.com")
	if err := db.DeleteSession(ctx, s.Token); err != nil {
		t.Fatal(err)
	}
	if db.IsOwnerToken(tok) {
		t.Error("token valid after logout")
	}
	_, _, tok = mk(db, "b@example.com")
	if err := db.SetPassword(ctx, "b@example.com", "another-pass1"); err != nil {
		t.Fatal(err)
	}
	if db.IsOwnerToken(tok) {
		t.Error("token valid after password reset")
	}
	_, _, tok = mk(db, "c@example.com")
	if err := db.DeleteUser(ctx, "c@example.com"); err != nil {
		t.Fatal(err)
	}
	if db.IsOwnerToken(tok) {
		t.Error("token valid after user removal")
	}
	_, _, tok = mk(db, "d@example.com")
	clk.t = clk.t.Add(SessionTTL + time.Hour)
	if db.IsOwnerToken(tok) {
		t.Error("token valid after session expiry")
	}
}

func TestOwnerSecretPersists(t *testing.T) {
	db, _ := openTest(t)
	u, _ := db.CreateUser(context.Background(), "owner@example.com", "hunter22!")
	sess, _ := db.CreateSession(context.Background(), u)
	tok, _ := db.OwnerToken(u.ID, sess.Token)
	db.owner = ownerKeyCache{} // simulate a restart: the key must reload from disk
	if !db.IsOwnerToken(tok) {
		t.Fatal("token invalid after secret reload")
	}
}

func TestViewRecorderDrainsOnClose(t *testing.T) {
	db, _ := openTest(t)
	writes := make(chan struct{}, 10)
	r := db.NewViewRecorder(func() { writes <- struct{}{} })
	r.Record("x")
	r.Record("x")
	r.Close()
	r.Record("x") // after close: ignored, no panic
	st, _ := db.ViewStats(context.Background())
	if st["x"].Count != 2 || len(writes) != 2 {
		t.Fatalf("stats = %+v writes = %d", st, len(writes))
	}
}
