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
	tok, err := db.OwnerToken(7)
	if err != nil {
		t.Fatal(err)
	}
	if !db.IsOwnerToken(tok) {
		t.Fatal("valid token rejected")
	}
	for _, bad := range []string{"", "7", "7.", ".abc", "8" + tok[1:], tok + "0", "7.deadbeef", tok[:len(tok)-1]} {
		if db.IsOwnerToken(bad) {
			t.Errorf("forged token %q accepted", bad)
		}
	}
	db2, _ := openTest(t)
	if db2.IsOwnerToken(tok) {
		t.Error("token from another database accepted")
	}
}

func TestOwnerSecretPersists(t *testing.T) {
	db, _ := openTest(t)
	tok, _ := db.OwnerToken(1)
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
