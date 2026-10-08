package install

import (
	"strings"
	"testing"
)

func TestUpsertBlockRound3(t *testing.T) {
	const tomlB, tomlE = blockBeginTOML, blockEndTOML
	crlfBody := strings.ReplaceAll(tomlBody, "\n", "\r\n")
	runUpsertCases(t, []blockCase{
		// 1. multi-line values are refused wherever they appear.
		{
			name:  "multi-line array in glim table is refused",
			file:  tomlBlock(tomlOldBody+"\nextras = [\n  [\"x\"]\n]") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			wantErr: "multi-line value",
		},
		{
			name:  "multi-line array in foreign table is refused",
			file:  tomlBlock(tomlOldBody+"\n\n[a]\nlist = [\n  1,\n  2,\n]") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			wantErr: "multi-line value",
		},
		{
			name:  "multi-line inline table is refused",
			file:  tomlBlock(tomlOldBody+"\n\n[a]\nt = {\n  x = 1 }") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			wantErr: "multi-line value",
		},
		{
			name:  "balanced nested single-line array is kept",
			file:  tomlBlock(tomlOldBody+"\nextras = [[\"x\"], [\"y\"]]") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			want: tomlBlock(tomlBody+"\nextras = [[\"x\"], [\"y\"]]") + "\n",
		},
		// 2. glim-owned lines with trailing comments are refused.
		{
			name:  "trailing comment on a glim-owned key is refused",
			file:  tomlBlock("[mcp_servers.glim]\ncommand = \"/old/glim\" # user note\nargs = [\"mcp\"]") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			wantErr: "trailing comment",
		},
		{
			name:  "trailing comment on another key and full-line comments are kept",
			file:  tomlBlock("[mcp_servers.glim]\n# why\ncommand = \"/old/glim\"\nargs = [\"mcp\"]\nenabled = false # off for now") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			want: tomlBlock(tomlBody+"\n# why\nenabled = false # off for now") + "\n",
		},
		{
			name:  "hash inside a glim-owned string value is not a comment",
			file:  tomlBlock("[mcp_servers.glim]\ncommand = \"/old/#glim\"\nargs = [\"mcp\"]") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			want: tomlBlock(tomlBody) + "\n",
		},
		// 3. blank lines between kept lines in glim's table survive.
		{
			name:  "blank lines between kept lines in glim table are kept",
			file:  tomlBlock("[mcp_servers.glim]\n# note\nenabled = false\n\ncommand = \"/old/glim\"\n\ntools = [\"a\"]\nargs = [\"mcp\"]\n\n") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			want: tomlBlock(tomlBody+"\n# note\nenabled = false\n\n\ntools = [\"a\"]") + "\n",
		},
		// 4. mixed line endings: foreign lines keep their own terminator.
		{
			name:  "mixed line endings in a foreign table are kept",
			file:  "model = \"x\"\r\n\r\n" + blockBeginTOML + "\r\n" + strings.ReplaceAll(tomlOldBody, "\n", "\r\n") + "\r\n\r\n[a]\r\nx = 1\ny = 2\r\n" + blockEndTOML + "\r\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			want: "model = \"x\"\r\n\r\n" + blockBeginTOML + "\r\n" + crlfBody + "\r\n" + blockEndTOML + "\r\n\r\n[a]\r\nx = 1\ny = 2\r\n",
		},
		// 5. foreign span is moved verbatim; only glim's separator is normalised.
		{
			name:  "foreign tables keep their own blank lines and adjacency",
			file:  tomlBlock(tomlOldBody+"\n\n[a]\nx = 1\n\n\n\n[b]\ny = 2\n[c]\nz = 3\n\n\n") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			want: tomlBlock(tomlBody) + "\n\n[a]\nx = 1\n\n\n\n[b]\ny = 2\n[c]\nz = 3\n",
		},
		// 6. no trailing newline is added at EOF.
		{
			name:  "block at EOF without trailing newline stays without one",
			file:  tomlBlock(tomlOldBody + "\n\n" + tomlOther),
			begin: tomlB, end: tomlE, body: tomlBody,
			want: tomlBlock(tomlBody) + "\n\n" + tomlOther,
		},
	})
}

func TestRemoveBlockRound3(t *testing.T) {
	cases := []struct {
		name, file, want, wantErr string
	}{
		{
			name:    "multi-line array is refused",
			file:    tomlBlock(tomlBody+"\n\n[a]\nlist = [\n  [\"x\"]\n]") + "\n",
			wantErr: "multi-line value",
		},
		{
			name: "trailing comment on glim-owned key is dropped with the table",
			file: tomlBlock("[mcp_servers.glim]\ncommand = \"/g\" # note\nargs = [\"mcp\"]\n\n"+tomlOther) + "\n",
			want: tomlOther + "\n",
		},
		{
			name: "mixed line endings in a foreign table are kept",
			file: "a = 1\r\n\r\n" + blockBeginTOML + "\r\n" + strings.ReplaceAll(tomlBody, "\n", "\r\n") + "\r\n\r\n[a]\r\nx = 1\ny = 2\r\n" + blockEndTOML + "\r\n",
			want: "a = 1\r\n\r\n[a]\r\nx = 1\ny = 2\r\n",
		},
		{
			name: "foreign tables keep their own blank lines",
			file: tomlBlock(tomlBody+"\n\n[a]\nx = 1\n\n\n[b]\ny = 2\n\n") + "\n",
			want: "[a]\nx = 1\n\n\n[b]\ny = 2\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTemp(t, "f", tc.file)
			ok, err := removeBlock(path, blockBeginTOML, blockEndTOML)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				if got := readFile(t, path); got != tc.file {
					t.Fatalf("file modified on refusal:\n%q", got)
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
