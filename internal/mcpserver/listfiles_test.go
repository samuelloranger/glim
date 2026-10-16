package mcpserver

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListFilesSkipsGlimFiles(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"index.html", ".glim.json", ".glim-listen/audio.mp3", ".glim-listen/result.json", "img/a.png", "notes.md"} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, truncated := listFiles(dir)
	if want := []string{"img/a.png", "notes.md"}; !reflect.DeepEqual(got, want) || truncated {
		t.Fatalf("listFiles = %v (truncated %v), want %v", got, truncated, want)
	}
}
