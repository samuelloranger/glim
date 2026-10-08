package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertCalls(t *testing.T, got [][]string, want []string) {
	t.Helper()
	var joined []string
	for _, c := range got {
		joined = append(joined, strings.Join(c, " "))
	}
	if strings.Join(joined, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(joined, "\n"), strings.Join(want, "\n"))
	}
}

func TestInstallClaudeFreshRegisters(t *testing.T) {
	d, calls := testDeps(t)
	d.Run = func(name string, args ...string) error {
		*calls = append(*calls, append([]string{name}, args...))
		if args[1] == "remove" {
			return os.ErrNotExist // not registered yet
		}
		return nil
	}
	steps, err := Install("claude", d)
	if err != nil {
		t.Fatal(err)
	}
	assertCalls(t, *calls, []string{
		"claude mcp remove --scope user glim",
		"claude mcp add --scope user glim -- /usr/bin/glim mcp",
	})
	if !strings.HasPrefix(steps[0], "registered glim MCP server") {
		t.Fatalf("steps: %v", steps)
	}
	md, _ := os.ReadFile(filepath.Join(d.Home, ".claude", "CLAUDE.md"))
	if !strings.Contains(string(md), SteeringRule) {
		t.Fatal("steering rule not written")
	}
}

func TestInstallClaudeRerunUpdatesExistingRegistration(t *testing.T) {
	registered := map[string]string{}
	var calls [][]string
	d, _ := testDeps(t)
	d.Run = func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		switch args[1] {
		case "remove":
			if _, ok := registered["glim"]; !ok {
				return os.ErrNotExist
			}
			delete(registered, "glim")
		case "add":
			if _, ok := registered["glim"]; ok {
				return os.ErrExist // claude: "already exists in user config"
			}
			registered["glim"] = args[6]
		}
		return nil
	}
	if _, err := Install("claude", d); err != nil {
		t.Fatal(err)
	}
	// glim moved: re-run with a new binary path.
	d.GlimPath = "/opt/new/glim"
	calls = nil
	steps, err := Install("claude", d)
	if err != nil {
		t.Fatalf("re-run failed: %v", err)
	}
	assertCalls(t, calls, []string{
		"claude mcp remove --scope user glim",
		"claude mcp add --scope user glim -- /opt/new/glim mcp",
	})
	if registered["glim"] != "/opt/new/glim" {
		t.Fatalf("registration not updated: %v", registered)
	}
	if !strings.HasPrefix(steps[0], "updated glim MCP server registration") {
		t.Fatalf("steps: %v", steps)
	}
	if len(steps) != 2 || !strings.HasPrefix(steps[1], "wrote steering rule") {
		t.Fatalf("steering rule step missing: %v", steps)
	}
	md, _ := os.ReadFile(filepath.Join(d.Home, ".claude", "CLAUDE.md"))
	if strings.Count(string(md), blockBeginMD) != 1 {
		t.Fatalf("expected one managed block:\n%s", md)
	}
}

func TestInstallClaudeAddFailureStillErrors(t *testing.T) {
	d, _ := testDeps(t)
	d.Run = func(string, ...string) error { return os.ErrPermission }
	if _, err := Install("claude", d); err == nil {
		t.Fatal("expected add failure to be reported")
	}
}

func TestInstallClaudeNotInstalledUnchanged(t *testing.T) {
	d, calls := testDeps(t)
	d.HasCommand = func(string) bool { return false }
	if _, err := Install("claude", d); err == nil || len(*calls) != 0 {
		t.Fatalf("err=%v calls=%v", err, *calls)
	}
}
