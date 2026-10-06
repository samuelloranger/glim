package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveFileThroughSymlinkedDirKeepsLink(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "real")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "other"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	ok, err := removeFile(filepath.Join(link, "SKILL.md"))
	if ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal("symlink removed")
	}
}
