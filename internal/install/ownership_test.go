package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hasStep(steps []string, sub string) bool {
	for _, s := range steps {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestUninstallKeepsUserSkill(t *testing.T) {
	d, _ := testDeps(t)
	dir := filepath.Join(d.Home, ".claude", "skills", "glim")
	os.MkdirAll(dir, 0o755)
	mine := "---\nname: glim\ndescription: my own\n---\nmine\n"
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(mine), 0o644)
	steps, err := Uninstall("claude", d)
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, "SKILL.md")); got != mine {
		t.Fatalf("user skill changed: %q", got)
	}
	if !hasStep(steps, "left "+filepath.Join(dir, "SKILL.md")+" in place") {
		t.Fatalf("steps: %v", steps)
	}
}

func TestUninstallKeepsUserSkillThroughSymlinkedDir(t *testing.T) {
	d, _ := testDeps(t)
	real := filepath.Join(t.TempDir(), "my-skills", "glim")
	os.MkdirAll(real, 0o755)
	os.WriteFile(filepath.Join(real, "SKILL.md"), []byte("user skill\n"), 0o644)
	link := filepath.Join(d.Home, ".claude", "skills", "glim")
	os.MkdirAll(filepath.Dir(link), 0o755)
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	if _, err := Uninstall("claude", d); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(real, "SKILL.md")); got != "user skill\n" {
		t.Fatalf("user skill changed: %q", got)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal("symlink removed")
	}
}

func TestUninstallRemovesGlimSkillThroughSymlinkedDirKeepsLink(t *testing.T) {
	d, _ := testDeps(t)
	real := filepath.Join(t.TempDir(), "glim")
	os.MkdirAll(real, 0o755)
	os.WriteFile(filepath.Join(real, "SKILL.md"), []byte(SkillText), 0o644)
	link := filepath.Join(d.Home, ".claude", "skills", "glim")
	os.MkdirAll(filepath.Dir(link), 0o755)
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	if _, err := Uninstall("claude", d); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(real, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("glim skill not removed")
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal("symlink removed")
	}
}

func TestUninstallRemovesLegacyGlimSkill(t *testing.T) {
	d, _ := testDeps(t)
	dir := filepath.Join(d.Home, ".claude", "skills", "glim")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(legacySkillTexts[0]), 0o644)
	if _, err := Uninstall("claude", d); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("legacy skill dir not removed")
	}
}

func TestUninstallCursorKeepsUserRuleAndServer(t *testing.T) {
	d, _ := testDeps(t)
	rule := filepath.Join(d.Home, ".cursor", "rules", "glim.mdc")
	os.MkdirAll(filepath.Dir(rule), 0o755)
	os.WriteFile(rule, []byte("my own rule\n"), 0o644)
	mcp := filepath.Join(d.Home, ".cursor", "mcp.json")
	own := `{"mcpServers":{"glim":{"command":"node","args":["server.js"],"env":{"A":"b"}}}}`
	os.WriteFile(mcp, []byte(own), 0o644)
	steps, err := Uninstall("cursor", d)
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, rule) != "my own rule\n" || readFile(t, mcp) != own {
		t.Fatal("user files changed")
	}
	if !hasStep(steps, "left "+rule) || !hasStep(steps, "left the glim entry in "+mcp) {
		t.Fatalf("steps: %v", steps)
	}
}

func TestUninstallCursorRemovesGlimOwnRule(t *testing.T) {
	d, _ := testDeps(t)
	if _, err := Install("cursor", d); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall("cursor", d); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d.Home, ".cursor", "rules", "glim.mdc")); !os.IsNotExist(err) {
		t.Fatal("rule not removed")
	}
}
