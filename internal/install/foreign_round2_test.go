package install

import (
	"os"
	"strings"
	"testing"
)

func crlf(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }

type blockCase struct {
	name       string
	file       string
	begin, end string
	body       string
	want       string
	wantErr    string
}

func runUpsertCases(t *testing.T, cases []blockCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTemp(t, "f", tc.file)
			err := upsertBlock(path, tc.begin, tc.end, tc.body)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				if got := readFile(t, path); got != tc.file {
					t.Fatalf("file modified on refusal:\n%q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, path); got != tc.want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, tc.want)
			}
			if err := upsertBlock(path, tc.begin, tc.end, tc.body); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, path); got != tc.want {
				t.Fatalf("second run changed the file:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

func TestUpsertBlockRound2(t *testing.T) {
	const tomlB, tomlE = blockBeginTOML, blockEndTOML
	runUpsertCases(t, []blockCase{
		{
			name:  "extra key in glim table is kept after glim's keys",
			file:  tomlBlock(tomlOldBody+"\nenabled = false") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			want: tomlBlock(tomlBody+"\nenabled = false") + "\n",
		},
		{
			name:  "extra multi-line key and comment in glim table are kept",
			file:  tomlBlock("[mcp_servers.glim]\n# note\nenabled = false\ncommand = \"/old/glim\"\ntools = [\n  \"a\", # c\n  \"b\",\n]\nargs = [\n  \"mcp\",\n  \"x\",\n]") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			want: tomlBlock(tomlBody+"\n# note\nenabled = false\ntools = [\n  \"a\", # c\n  \"b\",\n]") + "\n",
		},
		{
			name:  "user-added glim sub-table stays attached to glim",
			file:  tomlBlock(tomlOldBody+"\n\n[mcp_servers.glim.env]\nK = \"v\"\n\n"+tomlOther) + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			want: tomlBlock(tomlBody+"\n\n[mcp_servers.glim.env]\nK = \"v\"") + "\n\n" + tomlOther + "\n",
		},
		{
			name:  "quoted single-key header is a different table",
			file:  tomlBlock(tomlOldBody+"\n\n[\"mcp_servers.glim\"]\nx = 1") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			want: tomlBlock(tomlBody) + "\n\n[\"mcp_servers.glim\"]\nx = 1\n",
		},
		{
			name:  "spaced and quoted segments are the glim table",
			file:  tomlBlock("[ mcp_servers . \"glim\" ]\ncommand = \"/old/glim\"\nargs = [\"mcp\"]\nenabled = false") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			want: tomlBlock(tomlBody+"\nenabled = false") + "\n",
		},
		{
			name:  "multi-line basic string is refused",
			file:  tomlBlock(tomlOldBody+"\n\n[a]\ns = \"\"\"\n[mcp_servers.glim]\n\"\"\"") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			wantErr: "multi-line string",
		},
		{
			name:  "multi-line literal string is refused",
			file:  tomlBlock(tomlOldBody+"\n\n[a]\ns = '''\n[b]\n'''") + "\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			wantErr: "multi-line string",
		},
		{
			name:  "marker text inside a value does not close the block",
			file:  tomlBlock(tomlOldBody+"\n\n[a]\nnote = \"# /glim:managed\"") + "\n\n[b]\nx = 1\n",
			begin: tomlB, end: tomlE, body: tomlBody,
			want: tomlBlock(tomlBody) + "\n\n[a]\nnote = \"# /glim:managed\"\n\n[b]\nx = 1\n",
		},
		{
			name:  "markdown rule embedded mid-line is refused",
			file:  mdBlock("Preface: "+SteeringRule+" suffix") + "\n",
			begin: blockBeginMD, end: blockEndMD, body: SteeringRule,
			wantErr: "does not recognise",
		},
		{
			name:  "markdown end marker mid-line does not close the block",
			file:  mdBlock(SteeringRule+"\nsee <!-- /glim:managed --> here") + "\n\ntail\n",
			begin: blockBeginMD, end: blockEndMD, body: SteeringRule,
			want: mdBlock(SteeringRule) + "\n\nsee <!-- /glim:managed --> here\n\ntail\n",
		},
		{
			name:  "crlf toml foreign table moves outside",
			file:  crlf("model = \"x\"\n\n" + tomlBlock(tomlOldBody+"\nenabled = false\n\n"+tomlOther) + "\n"),
			begin: tomlB, end: tomlE, body: tomlBody,
			want: crlf("model = \"x\"\n\n" + tomlBlock(tomlBody+"\nenabled = false") + "\n\n" + tomlOther + "\n"),
		},
		{
			name:  "crlf markdown foreign lines move outside",
			file:  crlf("top\n\n" + mdBlock(SteeringRule+"\nmy note") + "\n\ntail\n"),
			begin: blockBeginMD, end: blockEndMD, body: SteeringRule,
			want: crlf("top\n\n" + mdBlock(SteeringRule) + "\n\nmy note\n\ntail\n"),
		},
	})
}

func TestRemoveBlockRound2(t *testing.T) {
	cases := []struct {
		name       string
		file       string
		begin, end string
		want       string
		wantErr    string
	}{
		{
			name: "glim table with extra keys and sub-table is dropped", begin: blockBeginTOML, end: blockEndTOML,
			file: tomlBlock(tomlBody+"\nenabled = false\n\n[mcp_servers.glim.env]\nK = \"v\"\n\n"+tomlOther) + "\n",
			want: tomlOther + "\n",
		},
		{
			name: "quoted single-key header stays", begin: blockBeginTOML, end: blockEndTOML,
			file: tomlBlock(tomlBody+"\n\n[\"mcp_servers.glim\"]\nx = 1") + "\n",
			want: "[\"mcp_servers.glim\"]\nx = 1\n",
		},
		{
			name: "multi-line string is refused", begin: blockBeginTOML, end: blockEndTOML,
			file:    tomlBlock(tomlBody+"\n\n[a]\ns = \"\"\"\nx\n\"\"\"") + "\n",
			wantErr: "multi-line string",
		},
		{
			name: "markdown rule embedded mid-line is refused", begin: blockBeginMD, end: blockEndMD,
			file:    mdBlock("Preface: "+SteeringRule+" suffix") + "\n",
			wantErr: "does not recognise",
		},
		{
			name: "marker text inside a value does not close the block", begin: blockBeginTOML, end: blockEndTOML,
			file: tomlBlock(tomlBody+"\n\n[a]\nnote = \"# /glim:managed\"") + "\n\n[b]\nx = 1\n",
			want: "[a]\nnote = \"# /glim:managed\"\n\n[b]\nx = 1\n",
		},
		{
			name: "crlf toml", begin: blockBeginTOML, end: blockEndTOML,
			file: crlf("model = \"x\"\n\n" + tomlBlock(tomlBody+"\n\n"+tomlOther) + "\n"),
			want: crlf("model = \"x\"\n\n" + tomlOther + "\n"),
		},
		{
			name: "crlf markdown", begin: blockBeginMD, end: blockEndMD,
			file: crlf("top\n\n" + mdBlock(SteeringRule+"\nmy note") + "\n\nbottom\n"),
			want: crlf("top\n\nmy note\n\nbottom\n"),
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
				b, _ := os.ReadFile(path)
				if string(b) != tc.file {
					t.Fatalf("file modified on refusal:\n%q", b)
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
