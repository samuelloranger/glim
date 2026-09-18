package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertBlockCreatesAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "CLAUDE.md")
	if err := upsertBlock(path, blockBeginMD, blockEndMD, "RULE ONE"); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	if !strings.Contains(string(first), "RULE ONE") {
		t.Fatal("body not written")
	}
	// Re-running with new body replaces, does not duplicate.
	if err := upsertBlock(path, blockBeginMD, blockEndMD, "RULE TWO"); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	s := string(second)
	if strings.Count(s, blockBeginMD) != 1 || strings.Count(s, blockEndMD) != 1 {
		t.Fatalf("expected exactly one managed block, got:\n%s", s)
	}
	if strings.Contains(s, "RULE ONE") || !strings.Contains(s, "RULE TWO") {
		t.Fatalf("block not replaced:\n%s", s)
	}
}

func TestUpsertBlockPreservesSurroundingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	os.WriteFile(path, []byte("# My rules\n\nkeep me\n"), 0o644)
	if err := upsertBlock(path, blockBeginMD, blockEndMD, "RULE"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "keep me") {
		t.Fatalf("clobbered existing content:\n%s", got)
	}
}

func TestMergeCursorMCPPreservesOtherServers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	os.WriteFile(path, []byte(`{"mcpServers":{"other":{"command":"x"}},"someKey":1}`), 0o644)
	if err := mergeCursorMCP(path, "glim", "/usr/bin/glim", []string{"mcp"}); err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if root["someKey"].(float64) != 1 {
		t.Error("top-level key lost")
	}
	servers := root["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Error("existing server lost")
	}
	glim := servers["glim"].(map[string]any)
	if glim["command"] != "/usr/bin/glim" {
		t.Errorf("glim command = %v", glim["command"])
	}
}

func TestMergeCursorMCPCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "mcp.json")
	if err := mergeCursorMCP(path, "glim", "/usr/bin/glim", []string{"mcp"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestInstallCursorEndToEnd(t *testing.T) {
	home := t.TempDir()
	steps, err := Install("cursor", Deps{Home: home, GlimPath: "/usr/bin/glim"})
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 {
		t.Fatalf("steps = %v", steps)
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); err != nil {
		t.Error("mcp.json missing")
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "rules", "glim.mdc")); err != nil {
		t.Error("rule file missing")
	}
}

func TestInstallCodexWritesConfigAndRule(t *testing.T) {
	home := t.TempDir()
	steps, err := Install("codex", Deps{
		Home: home, GlimPath: "/usr/bin/glim",
		HasCommand: func(string) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 {
		t.Fatalf("steps = %v", steps)
	}
	cfg, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if !strings.Contains(string(cfg), "[mcp_servers.glim]") {
		t.Fatalf("codex config missing server:\n%s", cfg)
	}
}

func TestInstallUnknownTarget(t *testing.T) {
	if _, err := Install("emacs", Deps{Home: t.TempDir()}); err == nil {
		t.Fatal("expected error for unknown target")
	}
}
