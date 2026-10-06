package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
)

func TestLockUnlockCLI(t *testing.T) {
	s := store.New(t.TempDir(), "http://127.0.0.1:1")
	page := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(page, []byte("<h1>x</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(page, "", "", "", time.Hour, "demo-1234"); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := runLock(s, "demo-1234", bufio.NewReader(strings.NewReader("short\n")), &out); err == nil {
		t.Fatal("too-short password must be rejected")
	}
	if err := runLock(s, "demo-1234", bufio.NewReader(strings.NewReader("long enough pw\n")), &out); err != nil {
		t.Fatal(err)
	}
	m, _ := s.Get("demo-1234")
	if !auth.CheckPassword(m.PasswordHash, "long enough pw") {
		t.Fatal("lock did not store a matching bcrypt hash")
	}
	if err := runUnlock(s, "demo-1234", &out); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.Get("demo-1234"); m.Locked() {
		t.Fatal("unlock left the preview locked")
	}
	if err := runLock(s, "nope-9999", bufio.NewReader(strings.NewReader("long enough pw\n")), &out); err == nil {
		t.Fatal("locking a missing preview must fail")
	}
}

func TestPasswordFlagTakesNoValue(t *testing.T) {
	entry, flagArgs := splitEntry([]string{"--password", "page.html", "--ttl", "1h"})
	if entry != "page.html" || len(flagArgs) != 3 {
		t.Fatalf("entry=%q flags=%v", entry, flagArgs)
	}
	// A password on the command line is an unexpected argument, never a value.
	if err := cmdPublish([]string{"page.html", "--password", "hunter2hunter2"}); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("err = %v, want usage error", err)
	}
}
