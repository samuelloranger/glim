package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samuelloranger/glim/internal/serve"
)

func TestServerLine(t *testing.T) {
	st := serve.State{Port: 8787, PID: 4242}
	if got := serverLine(st, true); got != "running on port 8787 (pid 4242)" {
		t.Errorf("running = %q", got)
	}
	if got := serverLine(st, false); got != "not running" {
		t.Errorf("stopped = %q", got)
	}
}

func TestServiceBinPathKeepsSymlink(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "glim-1.2.3")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "glim")
	if err := os.Symlink(bin, link); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other")
	if err := os.WriteFile(other, []byte("y"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := os.Args[0]
	defer func() { os.Args[0] = old }()

	os.Args[0] = link
	if got := serviceBinPath(bin); got != link {
		t.Errorf("symlink invoked: got %q, want %q", got, link)
	}
	os.Args[0] = other
	if got := serviceBinPath(bin); got != bin {
		t.Errorf("argv[0] naming a different binary: got %q, want %q", got, bin)
	}
	os.Args[0] = "no-such-command-on-path"
	if got := serviceBinPath(bin); got != bin {
		t.Errorf("unresolvable argv[0]: got %q, want %q", got, bin)
	}
}
