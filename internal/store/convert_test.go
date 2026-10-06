package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublishConvertsFormats(t *testing.T) {
	cases := []struct {
		file, content, want string
	}{
		{"notes.md", "# Title\n\n**bold**\n", "<strong>bold</strong>"},
		{"out.log", "a < b\n", "a &lt; b"},
		{"data.json", `{"k":1}`, "&#34;k&#34;: 1"},
		{"pic.png", "\x89PNG", `<img src="pic.png"`},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			s := newTestStore(t)
			res, err := s.Publish(writeTemp(t, c.file, c.content), "", "", "", time.Hour, "")
			if err != nil {
				t.Fatal(err)
			}
			idx, err := os.ReadFile(filepath.Join(s.Root, res.Name, "index.html"))
			if err != nil || !strings.Contains(string(idx), c.want) {
				t.Fatalf("index.html = %q (%v), want containing %q", idx, err, c.want)
			}
			if !strings.Contains(string(idx), `href="`+c.file+`"`) {
				t.Error("missing raw link")
			}
			raw, err := os.ReadFile(filepath.Join(s.Root, res.Name, c.file))
			if err != nil || string(raw) != c.content {
				t.Fatalf("original not kept: %q %v", raw, err)
			}
		})
	}
}

func TestPublishInvalidJSONLeavesNothing(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Publish(writeTemp(t, "bad.json", "{nope"), "", "", "", time.Hour, "")
	if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("err = %v", err)
	}
	entries, _ := os.ReadDir(s.Root)
	if len(entries) != 0 {
		t.Fatalf("residue: %v", entries)
	}
}
