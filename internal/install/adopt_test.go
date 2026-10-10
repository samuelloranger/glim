package install

import (
	"os"
	"strings"
	"testing"
)

func TestUpsertAdoptsUnmanagedGlimTable(t *testing.T) {
	cases := []struct {
		name, file, want string
	}{
		{
			name: "plain table is replaced in place",
			file: "model = \"x\"\n\n" + tomlOldBody + "\n\n" + tomlOther + "\n",
			want: "model = \"x\"\n\n" + tomlBlock(tomlBody) + "\n\n" + tomlOther + "\n",
		},
		{
			name: "user sub-table and extra key stay attached inside the block",
			file: "[mcp_servers.glim]\ncommand = \"/old/glim\"\nargs = [\"mcp\"]\nstartup_timeout_sec = 20\n\n[mcp_servers.glim.env]\nK = \"v\"\n",
			want: tomlBlock(tomlBody+"\nstartup_timeout_sec = 20\n\n[mcp_servers.glim.env]\nK = \"v\"") + "\n",
		},
		{
			name: "sub-table separated from the table by another table",
			file: tomlOldBody + "\n\n" + tomlOther + "\n\n[mcp_servers.glim.env]\nK = \"v\"\n",
			want: tomlBlock(tomlBody+"\n\n[mcp_servers.glim.env]\nK = \"v\"") + "\n\n" + tomlOther + "\n",
		},
		{
			name: "comment before the next table stays outside",
			file: tomlOldBody + "\n\n# about other\n" + tomlOther + "\n",
			want: tomlBlock(tomlBody) + "\n\n# about other\n" + tomlOther + "\n",
		},
		{
			name: "crlf file",
			file: crlf(tomlOldBody + "\n\n" + tomlOther + "\n"),
			want: crlf(tomlBlock(tomlBody) + "\n\n" + tomlOther + "\n"),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeTemp(t, "config.toml", c.file)
			if err := upsertBlock(path, blockBeginTOML, blockEndTOML, tomlBody); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, path); got != c.want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, c.want)
			}
			// Idempotent, and never a second glim table.
			if err := upsertBlock(path, blockBeginTOML, blockEndTOML, tomlBody); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, path); got != c.want {
				t.Fatalf("second run changed file:\n%q", got)
			}
			if n := strings.Count(readFile(t, path), "[mcp_servers.glim]"); n != 1 {
				t.Fatalf("%d glim tables", n)
			}
		})
	}
}

func TestUpsertRefusesUnadoptableGlimDefinitions(t *testing.T) {
	cases := map[string]string{
		"dotted key at root":   "mcp_servers.glim.command = \"x\"\n",
		"dotted key in parent": "[mcp_servers]\nglim.command = \"x\"\n",
		"inline table":         "mcp_servers = { glim = { command = \"x\" } }\n",
		"array of tables":      "[[mcp_servers.glim]]\ncommand = \"x\"\n",
		"repeated table":       tomlOldBody + "\n\n" + tomlOldBody + "\n",
		"multi-line string":    "note = \"\"\"\nhi\n\"\"\"\n\n" + tomlOldBody + "\n",
		"multi-line array":     "list = [\n  \"a\",\n]\n\n" + tomlOldBody + "\n",
	}
	for name, file := range cases {
		t.Run(name, func(t *testing.T) {
			path := writeTemp(t, "config.toml", file)
			err := upsertBlock(path, blockBeginTOML, blockEndTOML, tomlBody)
			if err == nil || !strings.Contains(err.Error(), "outside glim's markers") {
				t.Fatalf("want refusal, got %v", err)
			}
			if got := readFile(t, path); got != file {
				t.Fatalf("file changed: %q", got)
			}
		})
	}
}

func TestUpsertRefusesGlimTableOutsideExistingBlock(t *testing.T) {
	file := tomlBlock(tomlOldBody) + "\n\n[mcp_servers.glim.env]\nK = \"v\"\n"
	path := writeTemp(t, "config.toml", file)
	err := upsertBlock(path, blockBeginTOML, blockEndTOML, tomlBody)
	if err == nil || !strings.Contains(err.Error(), "outside glim's markers") {
		t.Fatalf("want refusal, got %v", err)
	}
	if readFile(t, path) != file {
		t.Fatal("file changed")
	}
}

func TestUpsertUnrelatedMultilineStringStillAppends(t *testing.T) {
	file := "note = \"\"\"\nhi\n\"\"\"\n"
	path := writeTemp(t, "config.toml", file)
	if err := upsertBlock(path, blockBeginTOML, blockEndTOML, tomlBody); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != file+"\n"+tomlBlock(tomlBody)+"\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCodexInstallAdoptsThenUninstallLeavesNoGlimTable(t *testing.T) {
	d, _ := testDeps(t)
	cfg := d.Home + "/.codex/config.toml"
	os.MkdirAll(d.Home+"/.codex", 0o755)
	orig := "model = \"x\"\n\n[mcp_servers.glim]\ncommand = \"/old/glim\"\nargs = [\"mcp\"]\n\n[mcp_servers.glim.env]\nK = \"v\"\n\n" + tomlOther + "\n"
	os.WriteFile(cfg, []byte(orig), 0o644)
	if _, err := Install("codex", d); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, cfg)
	if strings.Count(got, "[mcp_servers.glim]") != 1 || !strings.Contains(got, "/usr/bin/glim") || !strings.Contains(got, "K = \"v\"") {
		t.Fatalf("bad adoption:\n%s", got)
	}
	if _, err := Uninstall("codex", d); err != nil {
		t.Fatal(err)
	}
	after := readFile(t, cfg)
	if strings.Contains(after, "glim") {
		t.Fatalf("stray glim content left:\n%s", after)
	}
	if after != "model = \"x\"\n\n"+tomlOther+"\n" {
		t.Fatalf("unexpected: %q", after)
	}
}

func TestCodexInstallRefusalLeavesConfigUntouched(t *testing.T) {
	d, _ := testDeps(t)
	cfg := d.Home + "/.codex/config.toml"
	os.MkdirAll(d.Home+"/.codex", 0o755)
	orig := "[mcp_servers]\nglim = { command = \"x\" }\n"
	os.WriteFile(cfg, []byte(orig), 0o644)
	if _, err := Install("codex", d); err == nil {
		t.Fatal("want error")
	}
	if readFile(t, cfg) != orig {
		t.Fatal("config changed")
	}
	if _, err := os.Stat(d.Home + "/.codex/AGENTS.md"); err == nil {
		t.Fatal("steering rule written despite refusal")
	}
}

func TestAdoptCRLFWithoutFinalNewlineEndsWithCRLF(t *testing.T) {
	file := strings.TrimSuffix(crlf(tomlOther+"\n\n"+tomlOldBody+"\n"), "\r\n")
	path := writeTemp(t, "config.toml", file)
	if err := upsertBlock(path, blockBeginTOML, blockEndTOML, tomlBody); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if !strings.HasSuffix(got, blockEndTOML+"\r\n") {
		t.Fatalf("want a CRLF terminated end marker, got %q", got)
	}
	if strings.Count(strings.ReplaceAll(got, "\r\n", ""), "\r") != 0 {
		t.Fatalf("lone CR in %q", got)
	}
}

func TestBOMFileGlimTableIsAdoptedAndRemoved(t *testing.T) {
	file := "\ufeff" + tomlOldBody + "\n\n" + tomlOther + "\n"
	path := writeTemp(t, "config.toml", file)
	if err := upsertBlock(path, blockBeginTOML, blockEndTOML, tomlBody); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if want := "\ufeff" + tomlBlock(tomlBody) + "\n\n" + tomlOther + "\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if n := strings.Count(got, "[mcp_servers.glim]"); n != 1 {
		t.Fatalf("%d glim tables", n)
	}
	// Second run is stable, and a block on the BOM line is found again.
	if err := upsertBlock(path, blockBeginTOML, blockEndTOML, tomlBody); err != nil {
		t.Fatal(err)
	}
	if readFile(t, path) != got {
		t.Fatalf("second run changed file: %q", readFile(t, path))
	}
	ok, err := removeBlock(path, blockBeginTOML, blockEndTOML)
	if err != nil || !ok {
		t.Fatalf("remove: %v %v", ok, err)
	}
	if after := readFile(t, path); after != "\ufeff"+tomlOther+"\n" {
		t.Fatalf("unexpected: %q", after)
	}
}

func TestInlineMCPServersOnlyRefusedWhenItDefinesGlim(t *testing.T) {
	ok := "[mcp_servers]\nother = { note = \"glimmer\" }\n"
	if scanGlimUse(splitLines(ok)).any() {
		t.Fatal("unrelated value mentioning glim as a substring was treated as a glim definition")
	}
	for _, bad := range []string{
		"mcp_servers = { glim = { command = \"x\" } }\n",
		"mcp_servers = { \"glim\" = { command = \"x\" } }\n",
		"mcp_servers = { other = {}, glim.command = \"x\" }\n",
	} {
		if !scanGlimUse(splitLines(bad)).any() {
			t.Fatalf("missed glim definition in %q", bad)
		}
	}
}

func TestCodexInstallReportsAdoption(t *testing.T) {
	d, _ := testDeps(t)
	cfg := d.Home + "/.codex/config.toml"
	os.MkdirAll(d.Home+"/.codex", 0o755)
	os.WriteFile(cfg, []byte(tomlOldBody+"\n"), 0o644)
	steps, err := Install("codex", d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(steps, "\n"), "adopted the existing [mcp_servers.glim]") {
		t.Fatalf("no adoption notice: %v", steps)
	}
	steps, err = Install("codex", d)
	if err != nil || strings.Contains(strings.Join(steps, "\n"), "adopted") {
		t.Fatalf("rerun must not report adoption: %v %v", steps, err)
	}
}
