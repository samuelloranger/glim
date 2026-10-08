package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	tomlBody    = "[mcp_servers.glim]\ncommand = \"/usr/bin/glim\"\nargs = [\"mcp\"]"
	tomlOldBody = "[mcp_servers.glim]\ncommand = \"/old/glim\"\nargs = [\"mcp\"]"
	tomlOther   = "[mcp_servers.other]\ncommand = \"npx\"\nargs = [\"-y\", \"thing\"]"
)

func tomlBlock(inner string) string { return blockBeginTOML + "\n" + inner + "\n" + blockEndTOML }
func mdBlock(inner string) string   { return blockBeginMD + "\n" + inner + "\n" + blockEndMD }

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestUpsertBlockKeepsForeignContent(t *testing.T) {
	cases := []struct {
		name       string
		file       string
		begin, end string
		body       string
		want       string
		wantErr    string
	}{
		{
			name:  "toml clean block is replaced in place",
			file:  "model = \"x\"\n\n" + tomlBlock(tomlOldBody) + "\n",
			begin: blockBeginTOML, end: blockEndTOML, body: tomlBody,
			want: "model = \"x\"\n\n" + tomlBlock(tomlBody) + "\n",
		},
		{
			name:  "toml foreign table after glim body moves outside the markers",
			file:  "model = \"x\"\n\n" + tomlBlock(tomlOldBody+"\n\n"+tomlOther) + "\n",
			begin: blockBeginTOML, end: blockEndTOML, body: tomlBody,
			want: "model = \"x\"\n\n" + tomlBlock(tomlBody) + "\n\n" + tomlOther + "\n",
		},
		{
			name:  "toml foreign table before glim body moves outside the markers",
			file:  tomlBlock(tomlOther+"\n\n"+tomlOldBody) + "\n",
			begin: blockBeginTOML, end: blockEndTOML, body: tomlBody,
			want: tomlBlock(tomlBody) + "\n\n" + tomlOther + "\n",
		},
		{
			name:  "toml foreign table with no glim table inside the block",
			file:  tomlBlock(tomlOther) + "\n\n[tui]\ntheme = \"dark\"\n",
			begin: blockBeginTOML, end: blockEndTOML, body: tomlBody,
			want: tomlBlock(tomlBody) + "\n\n" + tomlOther + "\n\n[tui]\ntheme = \"dark\"\n",
		},
		{
			name:  "toml foreign tables on both sides of glim body keep their order",
			file:  tomlBlock("[a]\nx = 1\n\n"+tomlOldBody+"\n\n[b]\ny = 2") + "\n",
			begin: blockBeginTOML, end: blockEndTOML, body: tomlBody,
			want: tomlBlock(tomlBody) + "\n\n[a]\nx = 1\n\n[b]\ny = 2\n",
		},
		{
			name:  "toml quoted-segment glim sub-table stays attached to glim",
			file:  tomlBlock(tomlOldBody+"\n\n[mcp_servers.\"glim\".env]\nK = \"v\"\n\n"+tomlOther) + "\n",
			begin: blockBeginTOML, end: blockEndTOML, body: tomlBody,
			want: tomlBlock(tomlBody+"\n\n[mcp_servers.\"glim\".env]\nK = \"v\"") + "\n\n" + tomlOther + "\n",
		},
		{
			name:  "toml table whose name merely starts with glim is foreign",
			file:  tomlBlock(tomlOldBody+"\n\n[mcp_servers.glimmer]\ncommand = \"g\"") + "\n",
			begin: blockBeginTOML, end: blockEndTOML, body: tomlBody,
			want: tomlBlock(tomlBody) + "\n\n[mcp_servers.glimmer]\ncommand = \"g\"\n",
		},
		{
			name:  "toml comment inside glim table is kept",
			file:  tomlBlock("# keep me\n"+tomlOldBody) + "\n",
			begin: blockBeginTOML, end: blockEndTOML, body: tomlBody,
			want: tomlBlock(tomlBody) + "\n\n# keep me\n",
		},
		{
			name:  "toml bare key before any table is refused",
			file:  tomlBlock("stray = 1\n"+tomlOldBody) + "\n",
			begin: blockBeginTOML, end: blockEndTOML, body: tomlBody,
			wantErr: "key/value lines before any table header",
		},
		{
			name:  "markdown clean block is replaced in place",
			file:  "# Mine\n\n" + mdBlock(legacySteeringRules[0]) + "\n\ntail\n",
			begin: blockBeginMD, end: blockEndMD, body: SteeringRule,
			want: "# Mine\n\n" + mdBlock(SteeringRule) + "\n\ntail\n",
		},
		{
			name:  "markdown foreign lines after the rule move outside the markers",
			file:  mdBlock(SteeringRule+"\n\nmy own note") + "\n\ntail\n",
			begin: blockBeginMD, end: blockEndMD, body: SteeringRule,
			want: mdBlock(SteeringRule) + "\n\nmy own note\n\ntail\n",
		},
		{
			name:  "markdown foreign lines around a legacy rule move outside the markers",
			file:  mdBlock("before\n"+legacySteeringRules[0]+"\nafter") + "\n",
			begin: blockBeginMD, end: blockEndMD, body: SteeringRule,
			want: mdBlock(SteeringRule) + "\n\nbefore\nafter\n",
		},
		{
			name:  "markdown unrecognised content is refused",
			file:  mdBlock("something else entirely") + "\n",
			begin: blockBeginMD, end: blockEndMD, body: SteeringRule,
			wantErr: "does not recognise",
		},
		{
			name:  "markdown empty block is filled",
			file:  blockBeginMD + "\n" + blockEndMD + "\n",
			begin: blockBeginMD, end: blockEndMD, body: SteeringRule,
			want: mdBlock(SteeringRule) + "\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTemp(t, "f", tc.file)
			err := upsertBlock(path, tc.begin, tc.end, tc.body)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				if got := readFile(t, path); got != tc.file {
					t.Fatalf("file modified on refusal:\n%s", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, path); got != tc.want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, tc.want)
			}
			// Idempotent: a second run changes nothing.
			if err := upsertBlock(path, tc.begin, tc.end, tc.body); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, path); got != tc.want {
				t.Fatalf("second run changed the file:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

func TestRemoveBlockKeepsForeignContent(t *testing.T) {
	cases := []struct {
		name       string
		file       string
		begin, end string
		want       string
		wantErr    string
	}{
		{
			name: "toml clean block", begin: blockBeginTOML, end: blockEndTOML,
			file: "model = \"x\"\n\n" + tomlBlock(tomlBody) + "\n",
			want: "model = \"x\"\n",
		},
		{
			name: "toml foreign table inside the block stays", begin: blockBeginTOML, end: blockEndTOML,
			file: "model = \"x\"\n\n" + tomlBlock(tomlBody+"\n\n"+tomlOther) + "\n",
			want: "model = \"x\"\n\n" + tomlOther + "\n",
		},
		{
			name: "toml foreign table before glim body, content after the block", begin: blockBeginTOML, end: blockEndTOML,
			file: tomlBlock(tomlOther+"\n\n"+tomlBody) + "\n\n[tui]\ntheme = \"dark\"\n",
			want: tomlOther + "\n\n[tui]\ntheme = \"dark\"\n",
		},
		{
			name: "toml several foreign tables stay", begin: blockBeginTOML, end: blockEndTOML,
			file: tomlBlock(tomlBody+"\n\n[a]\nx = 1\n\n[b]\ny = 2") + "\n",
			want: "[a]\nx = 1\n\n[b]\ny = 2\n",
		},
		{
			name: "toml bare key before any table is refused", begin: blockBeginTOML, end: blockEndTOML,
			file:    tomlBlock("stray = 1\n"+tomlBody) + "\n",
			wantErr: "key/value lines",
		},
		{
			name: "markdown clean block", begin: blockBeginMD, end: blockEndMD,
			file: "top\n\n" + mdBlock(SteeringRule) + "\n\nbottom\n",
			want: "top\n\nbottom\n",
		},
		{
			name: "markdown foreign lines inside the block stay", begin: blockBeginMD, end: blockEndMD,
			file: "top\n\n" + mdBlock(SteeringRule+"\n\nmy own note") + "\n\nbottom\n",
			want: "top\n\nmy own note\n\nbottom\n",
		},
		{
			name: "markdown unrecognised content is refused", begin: blockBeginMD, end: blockEndMD,
			file:    mdBlock("something else entirely") + "\n",
			wantErr: "does not recognise",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTemp(t, "f", tc.file)
			ok, err := removeBlock(path, tc.begin, tc.end)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				if got := readFile(t, path); got != tc.file {
					t.Fatalf("file modified on refusal:\n%s", got)
				}
				return
			}
			if err != nil || !ok {
				t.Fatal(ok, err)
			}
			if got := readFile(t, path); got != tc.want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

func TestInstallThenUninstallLeavesOnlyForeignContent(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := "model = \"x\"\n\n" + tomlBlock(tomlOldBody+"\n\n"+tomlOther) + "\n"
	if err := os.WriteFile(cfg, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	d := Deps{Home: home, GlimPath: "/usr/bin/glim", HasCommand: func(string) bool { return true }}
	for i := 0; i < 2; i++ {
		if _, err := Install("codex", d); err != nil {
			t.Fatal(err)
		}
	}
	got := readFile(t, cfg)
	if !strings.Contains(got, tomlOther) || strings.Count(got, blockBeginTOML) != 1 || !strings.Contains(got, tomlBody) {
		t.Fatalf("after install:\n%s", got)
	}
	if ok, err := removeBlock(cfg, blockBeginTOML, blockEndTOML); err != nil || !ok {
		t.Fatal(ok, err)
	}
	want := "model = \"x\"\n\n" + tomlOther + "\n"
	if got := readFile(t, cfg); got != want {
		t.Fatalf("after uninstall:\n%q\nwant:\n%q", got, want)
	}
}
