package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// captureStdout runs fn and returns everything it wrote to os.Stdout.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	runErr := fn()
	os.Stdout = old
	w.Close()
	b, _ := io.ReadAll(r)
	return string(b), runErr
}

func jsonEnv(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GLIM_ROOT", t.TempDir())
	t.Setenv("GLIM_DOMAIN", "https://glim.example.com")
	page := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(page, []byte("<h1>x</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return page
}

func publishJSON(t *testing.T, page string, extra ...string) map[string]any {
	t.Helper()
	out, err := captureStdout(t, func() error {
		return cmdPublish(append([]string{page, "--json"}, extra...))
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("stdout is not a single JSON object: %q: %v", out, err)
	}
	return m
}

func TestPublishJSONShape(t *testing.T) {
	page := jsonEnv(t)
	// --qr must not add anything to stdout: --json wins.
	m := publishJSON(t, page, "--title", "Demo", "--qr")
	for _, k := range []string{"url", "name", "expires"} {
		if s, _ := m[k].(string); s == "" {
			t.Errorf("missing %q in %v", k, m)
		}
	}
	if _, ok := m["locked"]; ok {
		t.Errorf("locked must be omitted when false: %v", m)
	}
	if !strings.HasPrefix(m["url"].(string), "https://glim.example.com/") {
		t.Errorf("url = %v", m["url"])
	}
	if _, err := time.Parse(time.RFC3339, m["expires"].(string)); err != nil {
		t.Errorf("expires not RFC3339: %v", err)
	}
}

func TestListJSONShapeAndProjectFilter(t *testing.T) {
	page := jsonEnv(t)
	a := publishJSON(t, page, "--title", "A", "--project", "alpha")
	b := publishJSON(t, page, "--title", "B", "--project", "beta")
	if _, err := captureStdout(t, func() error { return cmdPin([]string{b["name"].(string)}) }); err != nil {
		t.Fatal(err)
	}

	list := func(args ...string) []map[string]any {
		out, err := captureStdout(t, func() error { return cmdList(args) })
		if err != nil {
			t.Fatal(err)
		}
		var rows []map[string]any
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatalf("stdout is not only a JSON array: %q: %v", out, err)
		}
		return rows
	}

	rows := list("--json")
	if len(rows) != 2 {
		t.Fatalf("rows = %v", rows)
	}
	for _, r := range rows {
		for _, k := range []string{"name", "url", "pinned", "views"} {
			if _, ok := r[k]; !ok {
				t.Errorf("missing %q in %v", k, r)
			}
		}
		pinned := r["pinned"].(bool)
		_, hasExpires := r["expires"]
		if pinned == hasExpires {
			t.Errorf("pinned=%v but expires present=%v: %v", pinned, hasExpires, r)
		}
	}

	only := list("--json", "--project", "alpha")
	if len(only) != 1 || only[0]["name"] != a["name"] || only[0]["project"] != "alpha" {
		t.Errorf("project filter = %v", only)
	}
	if none := list("--json", "--project", "nope"); len(none) != 0 {
		t.Errorf("unknown project = %v", none)
	}
}

func TestListProjectFilterPlain(t *testing.T) {
	page := jsonEnv(t)
	publishJSON(t, page, "--title", "Keepme", "--project", "alpha")
	publishJSON(t, page, "--title", "Dropme", "--project", "beta")
	out, err := captureStdout(t, func() error { return cmdList([]string{"--project", "alpha"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Keepme") || strings.Contains(out, "Dropme") {
		t.Errorf("plain ls --project output = %q", out)
	}
}

func TestPublishJSONErrorLeavesStdoutEmpty(t *testing.T) {
	jsonEnv(t)
	out, err := captureStdout(t, func() error {
		return cmdPublish([]string{filepath.Join(t.TempDir(), "missing.html"), "--json"})
	})
	if err == nil {
		t.Fatal("want error for a missing entry")
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}
}
