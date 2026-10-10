package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSteeringRulesDifferOnlyInArtifactsMention(t *testing.T) {
	if !strings.Contains(SteeringRule, "Claude artifacts") {
		t.Error("Claude rule should steer away from Claude artifacts")
	}
	if strings.Contains(SteeringRuleOther, "artifact") {
		t.Error("Codex/Cursor rule must not mention artifacts")
	}
	for _, r := range []string{SteeringRule, SteeringRuleOther, SkillText} {
		if !strings.Contains(r, sandboxNote) {
			t.Errorf("missing sandbox note in %.40q", r)
		}
	}
}

func TestInstallCodexAndCursorUseRuleWithoutArtifacts(t *testing.T) {
	d, _ := testDeps(t)
	d.HasCommand = func(string) bool { return true }
	for _, target := range []string{"codex", "cursor"} {
		if _, err := Install(target, d); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{
		filepath.Join(d.Home, ".codex", "AGENTS.md"),
		filepath.Join(d.Home, ".cursor", "rules", "glim.mdc"),
	} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), SteeringRuleOther) || strings.Contains(string(b), "artifact") {
			t.Errorf("%s has the wrong rule:\n%s", p, b)
		}
	}
}

func TestUpgradeFromEveryLegacyRuleWording(t *testing.T) {
	for i, legacy := range legacySteeringRules {
		path := filepath.Join(t.TempDir(), "AGENTS.md")
		if err := upsertBlock(path, blockBeginMD, blockEndMD, legacy); err != nil {
			t.Fatal(err)
		}
		if err := upsertBlock(path, blockBeginMD, blockEndMD, SteeringRuleOther); err != nil {
			t.Fatalf("legacy %d: %v", i, err)
		}
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), legacy) || !strings.Contains(string(b), SteeringRuleOther) {
			t.Errorf("legacy %d not replaced:\n%s", i, b)
		}
	}
}

func TestUninstallRemovesEveryLegacyCursorRule(t *testing.T) {
	for i, legacy := range legacySteeringRules {
		d, _ := testDeps(t)
		rule := filepath.Join(d.Home, ".cursor", "rules", "glim.mdc")
		os.MkdirAll(filepath.Dir(rule), 0o755)
		os.WriteFile(rule, []byte(cursorRuleBody(cursorRuleDescription, legacy)), 0o644)
		if _, err := Uninstall("cursor", d); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(rule); !os.IsNotExist(err) {
			t.Errorf("legacy %d rule not removed", i)
		}
	}
}

func TestUninstallRemovesEveryLegacySkill(t *testing.T) {
	for i, legacy := range legacySkillTexts {
		d, _ := testDeps(t)
		dir := filepath.Join(d.Home, ".claude", "skills", "glim")
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(legacy), 0o644)
		if _, err := Uninstall("claude", d); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("legacy skill %d not removed", i)
		}
	}
}

func TestUninstallRemovesCodexRuleBlock(t *testing.T) {
	d, _ := testDeps(t)
	path := filepath.Join(d.Home, ".codex", "AGENTS.md")
	if err := upsertBlock(path, blockBeginMD, blockEndMD, SteeringRuleOther); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall("codex", d); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); strings.Contains(string(b), "glim") {
		t.Errorf("block not removed:\n%s", b)
	}
}
