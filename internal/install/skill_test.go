package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testDeps(t *testing.T) (Deps, *[][]string) {
	var calls [][]string
	return Deps{
		Home:     t.TempDir(),
		GlimPath: "/usr/bin/glim",
		Run: func(name string, args ...string) error {
			calls = append(calls, append([]string{name}, args...))
			return nil
		},
	}, &calls
}

func allNothing(t *testing.T, steps []string) {
	t.Helper()
	for _, s := range steps {
		if !strings.HasPrefix(s, "nothing to remove") {
			t.Fatalf("second run not a no-op: %v", steps)
		}
	}
}

func TestInstallSkillWritesAndOverwrites(t *testing.T) {
	for target, rel := range map[string]string{
		"claude": ".claude/skills/glim/SKILL.md",
		"codex":  ".agents/skills/glim/SKILL.md",
		"cursor": ".cursor/skills/glim/SKILL.md",
	} {
		d, calls := testDeps(t)
		path := filepath.Join(d.Home, rel)
		for i := 0; i < 2; i++ {
			steps, err := InstallSkill(target, d)
			if err != nil || len(steps) != 1 {
				t.Fatalf("%s: %v %v", target, steps, err)
			}
		}
		got, _ := os.ReadFile(path)
		if string(got) != SkillText || !strings.HasPrefix(string(got), "---\nname: glim\ndescription: ") {
			t.Fatalf("%s: bad skill:\n%s", target, got)
		}
		if len(*calls) != 0 {
			t.Fatalf("%s: skill mode must not run commands: %v", target, *calls)
		}
	}
	d, _ := testDeps(t)
	if _, err := InstallSkill("nope", d); err == nil {
		t.Fatal("want error for unknown target")
	}
}

func TestRemoveBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.md")
	os.WriteFile(path, []byte("top\n\nbottom\n"), 0o644)
	if err := upsertBlock(path, blockBeginMD, blockEndMD, SteeringRule); err != nil {
		t.Fatal(err)
	}
	ok, err := removeBlock(path, blockBeginMD, blockEndMD)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "top\n\nbottom\n" {
		t.Fatalf("unexpected: %q", got)
	}
	if ok, _ := removeBlock(path, blockBeginMD, blockEndMD); ok {
		t.Fatal("second removal should be a no-op")
	}
	os.WriteFile(path, []byte("a\n\n"+blockBeginMD+"\n"+SteeringRule+"\n"+blockEndMD+"\n\nb\n"), 0o644)
	removeBlock(path, blockBeginMD, blockEndMD)
	got, _ = os.ReadFile(path)
	if string(got) != "a\n\nb\n" {
		t.Fatalf("got %q", got)
	}
	if ok, err := removeBlock(filepath.Join(t.TempDir(), "missing"), blockBeginMD, blockEndMD); ok || err != nil {
		t.Fatal("missing file should be a no-op")
	}
}

func TestUninstallClaude(t *testing.T) {
	d, calls := testDeps(t)
	md := filepath.Join(d.Home, ".claude", "CLAUDE.md")
	os.MkdirAll(filepath.Dir(md), 0o755)
	os.WriteFile(md, []byte("# mine\n\nkeep me\n"), 0o644)
	if _, err := Install("claude", d); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallSkill("claude", d); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(d.Home, ".claude", "skills", "glim")
	os.WriteFile(filepath.Join(dir, "other.txt"), []byte("x"), 0o644)
	*calls = nil

	if _, err := Uninstall("claude", d); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || strings.Join((*calls)[0], " ") != "claude mcp remove --scope user glim" {
		t.Fatalf("calls: %v", *calls)
	}
	got, _ := os.ReadFile(md)
	if string(got) != "# mine\n\nkeep me\n" {
		t.Fatalf("CLAUDE.md: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("SKILL.md not removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "other.txt")); err != nil {
		t.Fatal("foreign file deleted")
	}
	d.Run = func(string, ...string) error { return os.ErrNotExist }
	steps, err := Uninstall("claude", d)
	if err != nil {
		t.Fatal(err)
	}
	allNothing(t, steps)
}

func TestUninstallCodex(t *testing.T) {
	d, _ := testDeps(t)
	cfg := filepath.Join(d.Home, ".codex", "config.toml")
	os.MkdirAll(filepath.Dir(cfg), 0o755)
	os.WriteFile(cfg, []byte("model = \"x\"\n"), 0o644)
	if _, err := Install("codex", d); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallSkill("codex", d); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall("codex", d); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(cfg)
	if string(got) != "model = \"x\"\n" {
		t.Fatalf("config.toml: %q", got)
	}
	agents, _ := os.ReadFile(filepath.Join(d.Home, ".codex", "AGENTS.md"))
	if strings.Contains(string(agents), "glim") {
		t.Fatalf("AGENTS.md: %q", agents)
	}
	if _, err := os.Stat(filepath.Join(d.Home, ".agents", "skills", "glim")); !os.IsNotExist(err) {
		t.Fatal("skill dir not removed")
	}
	steps, _ := Uninstall("codex", d)
	allNothing(t, steps)
}

func TestUninstallCursor(t *testing.T) {
	d, _ := testDeps(t)
	mcp := filepath.Join(d.Home, ".cursor", "mcp.json")
	os.MkdirAll(filepath.Dir(mcp), 0o755)
	os.WriteFile(mcp, []byte(`{"mcpServers":{"other":{"command":"x"}},"someKey":1}`), 0o644)
	if _, err := Install("cursor", d); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallSkill("cursor", d); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall("cursor", d); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(mcp)
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	servers := root["mcpServers"].(map[string]any)
	if _, ok := servers["glim"]; ok {
		t.Fatal("glim not removed")
	}
	if _, ok := servers["other"]; !ok || root["someKey"] == nil {
		t.Fatalf("other content lost: %s", data)
	}
	for _, p := range []string{".cursor/rules/glim.mdc", ".cursor/skills/glim"} {
		if _, err := os.Stat(filepath.Join(d.Home, p)); !os.IsNotExist(err) {
			t.Fatalf("%s not removed", p)
		}
	}
	steps, _ := Uninstall("cursor", d)
	allNothing(t, steps)
}
