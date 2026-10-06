package main

import (
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
