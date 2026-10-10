package serve

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/store"
)

// A single Markdown file whose name and sibling image contain a colon must be
// reachable through the escaped relative URLs the page links to.
func TestColonNamedFilesServeThroughEscapedLinks(t *testing.T) {
	st := store.New(t.TempDir(), "http://127.0.0.1")
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "notes:v2.md"), []byte("# T\n\n![c](chart%3Av2.png)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "chart:v2.png"), []byte("PNGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Publish(filepath.Join(src, "notes:v2.md"), "", "", "", time.Hour, "colon"); err != nil {
		t.Fatal(err)
	}
	h := PreviewHandler(st)
	idx := get(t, h, "/colon/")
	if !strings.Contains(idx.Body.String(), `href="notes%3Av2.md"`) {
		t.Fatalf("raw link not escaped: %s", idx.Body.String())
	}
	for path, want := range map[string]string{"/colon/notes%3Av2.md": "# T", "/colon/chart%3Av2.png": "PNGDATA"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s = %d %q", path, rec.Code, rec.Body.String())
		}
	}
}
