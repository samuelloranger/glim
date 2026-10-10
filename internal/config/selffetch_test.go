package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelfFetchDefaultsOnAndIsConfigurable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GLIM_SELF_FETCH", "")
	if !Load().SelfFetchEnabled() {
		t.Fatal("self fetch should default to on")
	}
	if err := os.MkdirAll(filepath.Join(home, ".glim"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), []byte(`{"self_fetch": false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if Load().SelfFetchEnabled() {
		t.Fatal("self_fetch=false in the file should disable it")
	}
	t.Setenv("GLIM_SELF_FETCH", "true")
	if !Load().SelfFetchEnabled() {
		t.Fatal("env should override the file")
	}
	t.Setenv("GLIM_SELF_FETCH", "false")
	if Load().SelfFetchEnabled() {
		t.Fatal("env false should disable it")
	}
}
