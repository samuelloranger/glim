package serve

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestProbeHostMapping(t *testing.T) {
	for bind, want := range map[string]string{
		"":          "127.0.0.1",
		"0.0.0.0":   "127.0.0.1",
		"::":        "::1",
		"[::]":      "::1",
		"127.0.0.1": "127.0.0.1",
		"10.1.2.3":  "10.1.2.3",
		"::1":       "::1",
	} {
		if got := ProbeHost(bind); got != want {
			t.Errorf("ProbeHost(%q) = %q, want %q", bind, got, want)
		}
	}
}

func TestLoopbackURL(t *testing.T) {
	if got := (State{Port: 8787}).LoopbackURL(); got != "http://127.0.0.1:8787" {
		t.Errorf("old state = %q", got)
	}
	if got := (State{Port: 80, Bind: "::"}).LoopbackURL(); got != "http://[::1]:80" {
		t.Errorf("v6 = %q", got)
	}
}

func TestProbeUsesRecordedBind(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	for _, bind := range []string{"", "0.0.0.0", "127.0.0.1"} {
		if !Probe(State{Port: port, Bind: bind}) {
			t.Errorf("bind %q: listener on loopback should be reachable", bind)
		}
	}
	ln.Close()
	if Probe(State{Port: port}) {
		t.Error("closed port reported open")
	}
}

func TestStateRoundTripAndOldFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := WriteState(State{Port: 1234, Bind: "0.0.0.0", PID: 7, Root: "/r"}); err != nil {
		t.Fatal(err)
	}
	st, ok := ReadState()
	if !ok || st.Bind != "0.0.0.0" || st.Port != 1234 {
		t.Fatalf("round trip = %+v ok=%v", st, ok)
	}
	old := `{"port": 4321, "pid": 9, "root": "/r"}`
	if err := os.WriteFile(filepath.Join(home, ".glim", "serve.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	st, ok = ReadState()
	if !ok || st.Bind != "" || st.LoopbackURL() != "http://127.0.0.1:"+strconv.Itoa(4321) {
		t.Fatalf("old file = %+v ok=%v", st, ok)
	}
}
