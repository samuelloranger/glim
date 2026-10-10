package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveReloadDefaultsOnAndIsConfigurable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GLIM_LIVE_RELOAD", "")
	if !Load().LiveReloadEnabled() {
		t.Fatal("live reload should default to on")
	}
	if err := os.MkdirAll(filepath.Join(home, ".glim"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), []byte(`{"live_reload": false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if Load().LiveReloadEnabled() {
		t.Fatal("live_reload=false in the file should disable it")
	}
	t.Setenv("GLIM_LIVE_RELOAD", "true")
	if !Load().LiveReloadEnabled() {
		t.Fatal("env should override the file")
	}
}

func TestTTLDurationAcceptsDays(t *testing.T) {
	if got := (Config{TTL: "2d"}).TTLDuration(); got != 48*time.Hour {
		t.Fatalf("TTLDuration(2d) = %s", got)
	}
}
