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

var claudeGet = []string{"claude", "mcp", "get", "glim"}

// fakeClaude models claude's user-scope registry: get fails when glim is not
// registered, add fails when it is. failAdd/failRemove force those commands
// to fail. Every call is recorded.
type fakeClaude struct {
	registered string // user-scope glim path, "" when not registered
	otherScope bool   // glim also registered in project/local scope
	failAdd    error
	failRemove error
	calls      [][]string
}

func (f *fakeClaude) run(name string, args ...string) error {
	f.calls = append(f.calls, append([]string{name}, args...))
	switch args[1] {
	case "get":
		if f.registered == "" && !f.otherScope {
			return errors.New("no MCP server found with name: glim")
		}
	case "remove":
		if f.failRemove != nil {
			return f.failRemove
		}
		if f.registered == "" {
			return errors.New("no user-scoped MCP server found with name: glim")
		}
		f.registered = ""
	case "add":
		if f.failAdd != nil {
			return f.failAdd
		}
		if f.registered != "" {
			return errors.New("MCP server glim already exists in user config")
		}
		f.registered = args[6]
	}
	return nil
}

func ruleWritten(d Deps) bool {
	md, err := os.ReadFile(filepath.Join(d.Home, ".claude", "CLAUDE.md"))
	return err == nil && strings.Contains(string(md), SteeringRule)
}

func TestInstallClaudeFreshRegisters(t *testing.T) {
	d, _ := testDeps(t)
	f := &fakeClaude{}
	d.Run = f.run
	steps, err := Install("claude", d)
	if err != nil {
		t.Fatal(err)
	}
	assertCalls(t, f.calls, [][]string{claudeGet, claudeAdd("/usr/bin/glim")})
	if f.registered != "/usr/bin/glim" {
		t.Fatalf("registered %q", f.registered)
	}
	if !strings.HasPrefix(steps[0], "registered glim MCP server") || len(steps) != 2 {
		t.Fatalf("steps: %v", steps)
	}
	if !ruleWritten(d) {
		t.Fatal("steering rule not written")
	}
}

func TestInstallClaudeRerunUpdatesExistingRegistration(t *testing.T) {
	d, _ := testDeps(t)
	f := &fakeClaude{registered: "/usr/bin/glim"}
	d.Run = f.run
	// glim moved to a path containing a space.
	d.GlimPath = "/opt/new dir/glim"
	steps, err := Install("claude", d)
	if err != nil {
		t.Fatalf("re-run failed: %v", err)
	}
	assertCalls(t, f.calls, [][]string{claudeGet, claudeRemove, claudeAdd("/opt/new dir/glim")})
	if f.registered != "/opt/new dir/glim" {
		t.Fatalf("registration not updated: %q", f.registered)
	}
	if !strings.HasPrefix(steps[0], "updated glim MCP server registration") || len(steps) != 2 {
		t.Fatalf("steps: %v", steps)
	}
	if !ruleWritten(d) {
		t.Fatal("steering rule not written")
	}
}

func TestInstallClaudeFreshAddFailureRemovesNothing(t *testing.T) {
	d, _ := testDeps(t)
	addErr := errors.New("add boom")
	f := &fakeClaude{failAdd: addErr}
	d.Run = f.run
	_, err := Install("claude", d)
	if !errors.Is(err, addErr) {
		t.Fatalf("err = %v, want wrapping %v", err, addErr)
	}
	assertCalls(t, f.calls, [][]string{claudeGet, claudeAdd("/usr/bin/glim")})
	if ruleWritten(d) {
		t.Fatal("steering rule written despite error")
	}
}

func TestInstallClaudeRemoveFailureLeavesRegistrationUnchanged(t *testing.T) {
	d, _ := testDeps(t)
	rmErr := errors.New("remove boom")
	f := &fakeClaude{registered: "/old/glim", failRemove: rmErr}
	d.Run = f.run
	_, err := Install("claude", d)
	if !errors.Is(err, rmErr) || !strings.Contains(err.Error(), "left unchanged") {
		t.Fatalf("err = %v", err)
	}
	assertCalls(t, f.calls, [][]string{claudeGet, claudeRemove, claudeAdd("/usr/bin/glim")})
	if f.registered != "/old/glim" {
		t.Fatalf("registration changed: %q", f.registered)
	}
	if ruleWritten(d) {
		t.Fatal("steering rule written despite error")
	}
}

func TestInstallClaudeOtherScopeOnlyStillRegistersUserScope(t *testing.T) {
	d, _ := testDeps(t)
	f := &fakeClaude{otherScope: true}
	d.Run = f.run
	steps, err := Install("claude", d)
	if err != nil {
		t.Fatal(err)
	}
	assertCalls(t, f.calls, [][]string{claudeGet, claudeRemove, claudeAdd("/usr/bin/glim")})
	if f.registered != "/usr/bin/glim" {
		t.Fatalf("registered %q", f.registered)
	}
	if !strings.HasPrefix(steps[0], "registered glim MCP server") {
		t.Fatalf("steps: %v", steps)
	}
	if !ruleWritten(d) {
		t.Fatal("steering rule not written")
	}
}

func TestInstallClaudeSecondAddFailureReportsUnregistered(t *testing.T) {
	d, _ := testDeps(t)
	addErr := errors.New("add boom")
	f := &fakeClaude{registered: "/old/glim", failAdd: addErr}
	d.Run = f.run
	d.GlimPath = "/opt/it's here/glim"
	_, err := Install("claude", d)
	if !errors.Is(err, addErr) {
		t.Fatalf("err = %v, want wrapping %v", err, addErr)
	}
	want := `claude mcp add --scope user glim -- '/opt/it'\''s here/glim' mcp`
	for _, w := range []string{"UNREGISTERED", want} {
		if !strings.Contains(err.Error(), w) {
			t.Fatalf("error %q missing %q", err, w)
		}
	}
	assertCalls(t, f.calls, [][]string{claudeGet, claudeRemove, claudeAdd("/opt/it's here/glim")})
	if ruleWritten(d) {
		t.Fatal("steering rule written despite error")
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/usr/bin/glim":  `'/usr/bin/glim'`,
		"/opt/a b/glim":  `'/opt/a b/glim'`,
		"/opt/it's/glim": `'/opt/it'\''s/glim'`,
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
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
