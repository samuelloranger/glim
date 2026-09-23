package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSetupCodeShape(t *testing.T) {
	c := NewSetupCode()
	if len(c) != 10 || strings.ContainsAny(c, "01ILOU") {
		t.Fatalf("code %q", c)
	}
	if f := FormatSetupCode("ABCDEFGHJK"); f != "ABCD-EFGH-JK" {
		t.Fatalf("format = %q", f)
	}
}

func TestEnsureAndCompleteSetup(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "setup-code")

	code, err := db.EnsureSetupCode(ctx, path)
	if err != nil || code == "" {
		t.Fatalf("ensure = %q, %v", code, err)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %v", st.Mode().Perm())
	}
	if got, ok := ReadSetupCode(path); !ok || got != code {
		t.Fatalf("read = %q %v", got, ok)
	}
	if _, err := db.CompleteSetup(ctx, path, "WRONGWRONG", "sam", goodPass); !errors.Is(err, ErrBadSetupCode) {
		t.Fatalf("wrong code = %v", err)
	}
	if _, err := db.CompleteSetup(ctx, path, code, "bad name", goodPass); !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("bad name = %v", err)
	}
	messy := " " + strings.ToLower(FormatSetupCode(code)) + " "
	u, err := db.CompleteSetup(ctx, path, messy, "sam", goodPass)
	if err != nil || u.Username != "sam" {
		t.Fatalf("complete = %+v, %v", u, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("code file should be deleted")
	}
	if _, err := db.CompleteSetup(ctx, path, code, "eve", goodPass); !errors.Is(err, ErrSetupDone) {
		t.Fatalf("second setup = %v", err)
	}
	// With a user present, Ensure removes stale files and returns "".
	os.WriteFile(path, []byte("STALESTALE\n"), 0o600)
	if code, err := db.EnsureSetupCode(ctx, path); err != nil || code != "" {
		t.Fatalf("ensure with users = %q, %v", code, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stale code file should be removed")
	}
}

func TestConcurrentSetupHasOneWinner(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "setup-code")
	code, _ := db.EnsureSetupCode(ctx, path)

	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := db.CompleteSetup(ctx, path, code, "user"+string(rune('a'+i)), goodPass); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if n, _ := db.CountUsers(ctx); wins != 1 || n != 1 {
		t.Fatalf("wins = %d, users = %d; want 1, 1", wins, n)
	}
}
