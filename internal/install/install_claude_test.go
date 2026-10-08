package install

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func assertCalls(t *testing.T, got, want [][]string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls:\n%q\nwant:\n%q", got, want)
	}
}

func claudeAdd(path string) []string {
	return []string{"claude", "mcp", "add", "--scope", "user", "glim", "--", path, "mcp"}
}

var claudeRemove = []string{"claude", "mcp", "remove", "--scope", "user", "glim"}

// scripted makes Run return errs[i] for call i (nil past the end) and
// records every call.
func scripted(d *Deps, calls *[][]string, errs ...error) {
	d.Run = func(name string, args ...string) error {
		i := len(*calls)
		*calls = append(*calls, append([]string{name}, args...))
		if i < len(errs) {
			return errs[i]
		}
		return nil
	}
}

func ruleWritten(d Deps) bool {
	md, err := os.ReadFile(filepath.Join(d.Home, ".claude", "CLAUDE.md"))
	return err == nil && strings.Contains(string(md), SteeringRule)
}

func TestInstallClaudeFreshAddSucceeds(t *testing.T) {
	d, _ := testDeps(t)
	var calls [][]string
	scripted(&d, &calls)
	steps, err := Install("claude", d)
	if err != nil {
		t.Fatal(err)
	}
	assertCalls(t, calls, [][]string{claudeAdd("/usr/bin/glim")})
	if !strings.HasPrefix(steps[0], "registered glim MCP server") || len(steps) != 2 {
		t.Fatalf("steps: %v", steps)
	}
	if !ruleWritten(d) {
		t.Fatal("steering rule not written")
	}
}

func TestInstallClaudeRerunUpdatesExistingRegistration(t *testing.T) {
	registered := ""
	var calls [][]string
	d, _ := testDeps(t)
	d.Run = func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		switch args[1] {
		case "remove":
			if registered == "" {
				return os.ErrNotExist
			}
			registered = ""
		case "add":
			if registered != "" {
				return os.ErrExist // claude: "already exists in user config"
			}
			registered = args[6]
		}
		return nil
	}
	if _, err := Install("claude", d); err != nil {
		t.Fatal(err)
	}
	// glim moved to a path containing a space: re-run.
	d.GlimPath = "/opt/new dir/glim"
	calls = nil
	steps, err := Install("claude", d)
	if err != nil {
		t.Fatalf("re-run failed: %v", err)
	}
	assertCalls(t, calls, [][]string{
		claudeAdd("/opt/new dir/glim"),
		claudeRemove,
		claudeAdd("/opt/new dir/glim"),
	})
	if registered != "/opt/new dir/glim" {
		t.Fatalf("registration not updated: %q", registered)
	}
	if !strings.HasPrefix(steps[0], "updated glim MCP server registration") || len(steps) != 2 {
		t.Fatalf("steps: %v", steps)
	}
	if !ruleWritten(d) {
		t.Fatal("steering rule not written")
	}
}

func TestInstallClaudeRemoveFailureLeavesRegistrationUnchanged(t *testing.T) {
	d, _ := testDeps(t)
	var calls [][]string
	addErr, rmErr := errors.New("add boom"), errors.New("remove boom")
	scripted(&d, &calls, addErr, rmErr)
	_, err := Install("claude", d)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"add boom", "remove boom", "left unchanged"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
	assertCalls(t, calls, [][]string{claudeAdd("/usr/bin/glim"), claudeRemove})
	if ruleWritten(d) {
		t.Fatal("steering rule written despite error")
	}
}

func TestInstallClaudeSecondAddFailureReportsUnregistered(t *testing.T) {
	d, _ := testDeps(t)
	var calls [][]string
	scripted(&d, &calls, errors.New("first"), nil, errors.New("second boom"))
	_, err := Install("claude", d)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"UNREGISTERED", "second boom", "claude mcp add --scope user glim -- /usr/bin/glim mcp"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
	assertCalls(t, calls, [][]string{claudeAdd("/usr/bin/glim"), claudeRemove, claudeAdd("/usr/bin/glim")})
	if ruleWritten(d) {
		t.Fatal("steering rule written despite error")
	}
}

func TestInstallClaudeNotInstalledMakesNoCalls(t *testing.T) {
	d, calls := testDeps(t)
	d.HasCommand = func(string) bool { return false }
	if _, err := Install("claude", d); err == nil || len(*calls) != 0 {
		t.Fatalf("err=%v calls=%v", err, *calls)
	}
	if ruleWritten(d) {
		t.Fatal("steering rule written")
	}
}
