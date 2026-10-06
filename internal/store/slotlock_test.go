package store

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRepublishFailsClosedWhenManifestUnreadable(t *testing.T) {
	st := New(t.TempDir(), "https://glim.example.com")
	src := filepath.Join(t.TempDir(), "a.html")
	if err := os.WriteFile(src, []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PublishLocked(src, "t", "", "", time.Hour, "demo-1234", "hash"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.Root, "demo-1234", ManifestFile), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PublishLocked(src, "t", "", "", time.Hour, "demo-1234", ""); err == nil {
		t.Fatal("republish with an unreadable manifest must fail, not drop the lock")
	}
}

func TestConcurrentRepublishAndLockKeepPassword(t *testing.T) {
	st := New(t.TempDir(), "https://glim.example.com")
	src := filepath.Join(t.TempDir(), "a.html")
	if err := os.WriteFile(src, []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PublishLocked(src, "t", "", "", time.Hour, "demo-1234", "hash0"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := st.PublishLocked(src, "t", "", "", time.Hour, "demo-1234", ""); err != nil {
				t.Errorf("republish: %v", err)
			}
		}()
	}
	wg.Wait()
	m, err := st.Get("demo-1234")
	if err != nil || m.PasswordHash != "hash0" {
		t.Fatalf("after concurrent republishes = %+v, %v", m, err)
	}
	if err := st.SetPasswordHash("demo-1234", "hash1"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = st.PublishLocked(src, "t", "", "", time.Hour, "demo-1234", "")
		}()
		go func() {
			defer wg.Done()
			_ = st.SetPasswordHash("demo-1234", "hash1")
		}()
	}
	wg.Wait()
	if m, _ := st.Get("demo-1234"); m.PasswordHash != "hash1" {
		t.Fatalf("password lost: %+v", m)
	}
}
