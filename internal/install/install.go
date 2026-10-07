package install

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const SteeringRule = "To show the user any HTML, Markdown or other visual preview, call the glim `present` MCP tool " +
	"and give the user the returned link. To update a preview, call `present` again with the same `name`: " +
	"the link stays the same, and open tabs usually refresh by themselves (live reload, on by default). " +
	"Do not use Claude artifacts or other built-in preview mechanisms."

const (
	blockBeginMD   = "<!-- glim:managed -->"
	blockEndMD     = "<!-- /glim:managed -->"
	blockBeginTOML = "# glim:managed"
	blockEndTOML   = "# /glim:managed"
)

type Deps struct {
	Home       string
	GlimPath   string
	HasCommand func(string) bool
	Run        func(name string, args ...string) error
}

func Install(target string, d Deps) ([]string, error) {
	switch target {
	case "claude":
		return installClaude(d)
	case "codex":
		return installCodex(d)
	case "cursor":
		return installCursor(d)
	default:
		return nil, fmt.Errorf("unknown target %q (want claude|codex|cursor)", target)
	}
}

func installClaude(d Deps) ([]string, error) {
	if d.HasCommand != nil && !d.HasCommand("claude") {
		return nil, fmt.Errorf("claude is not installed")
	}
	var steps []string

	if err := d.Run("claude", "mcp", "add", "--scope", "user", "glim", "--", d.GlimPath, "mcp"); err != nil {
		return nil, fmt.Errorf("claude mcp add failed: %w", err)
	}
	steps = append(steps, "registered glim MCP server (claude mcp add --scope user)")

	path := filepath.Join(d.Home, ".claude", "CLAUDE.md")
	if err := upsertBlock(path, blockBeginMD, blockEndMD, SteeringRule); err != nil {
		return steps, err
	}
	steps = append(steps, "wrote steering rule to "+path)
	return steps, nil
}

func installCodex(d Deps) ([]string, error) {
	if d.HasCommand != nil && !d.HasCommand("codex") {
		return nil, fmt.Errorf("codex is not installed")
	}
	var steps []string

	cfg := filepath.Join(d.Home, ".codex", "config.toml")
	toml := fmt.Sprintf("[mcp_servers.glim]\ncommand = %q\nargs = [\"mcp\"]", d.GlimPath)
	if err := upsertBlock(cfg, blockBeginTOML, blockEndTOML, toml); err != nil {
		return nil, err
	}
	steps = append(steps, "registered glim MCP server in "+cfg)

	path := filepath.Join(d.Home, ".codex", "AGENTS.md")
	if err := upsertBlock(path, blockBeginMD, blockEndMD, SteeringRule); err != nil {
		return steps, err
	}
	steps = append(steps, "wrote steering rule to "+path)
	return steps, nil
}

func installCursor(d Deps) ([]string, error) {
	var steps []string

	mcpPath := filepath.Join(d.Home, ".cursor", "mcp.json")
	if err := mergeCursorMCP(mcpPath, "glim", d.GlimPath, []string{"mcp"}); err != nil {
		return nil, err
	}
	steps = append(steps, "registered glim MCP server in "+mcpPath)

	rule := filepath.Join(d.Home, ".cursor", "rules", "glim.mdc")
	body := "---\ndescription: Prefer glim for HTML and other visual previews\nalwaysApply: true\n---\n\n" + SteeringRule + "\n"
	if err := os.MkdirAll(filepath.Dir(rule), 0o755); err != nil {
		return steps, err
	}
	if err := os.WriteFile(rule, []byte(body), 0o644); err != nil {
		return steps, err
	}
	steps = append(steps, "wrote steering rule to "+rule)
	return steps, nil
}

// legacySteeringRules are earlier wordings of SteeringRule that glim wrote
// into managed blocks. They are recognised as glim's own content so an
// upgrade replaces them instead of treating them as foreign text.
var legacySteeringRules = []string{
	"To show the user any HTML or visual preview, call the glim `present` MCP tool " +
		"and give the user the returned link. Do not use Claude artifacts or other built-in preview mechanisms.",
}

// detectEOL returns the line ending glim should use for the separators it
// inserts: CRLF if the file already uses it, LF otherwise.
func detectEOL(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// splitLines splits text into lines without their line endings (LF or CRLF).
// A final line terminator does not produce an empty last line.
func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }

// trimBlankEnds drops blank lines from both ends of lines.
func trimBlankEnds(lines []string) []string {
	for len(lines) > 0 && isBlank(lines[0]) {
		lines = lines[1:]
	}
	for len(lines) > 0 && isBlank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// splitBlock separates what glim writes between the markers from anything
// else another tool or the user put there. It returns the new inner content
// of glim's block and the foreign content to keep outside of it (LF
// separated; callers convert to the file's line ending). With remove set,
// glim's own content is dropped entirely and the first result is empty.
// It returns an error when glim's content and foreign content cannot be told
// apart safely. TOML blocks are split structurally by table; every other
// block is matched against the bodies glim has written.
func splitBlock(path, begin, end, inner, body string, remove bool) (string, string, error) {
	if begin == blockBeginTOML {
		return splitTOMLBlock(path, begin, end, inner, body, remove)
	}
	foreign, err := splitTextBlock(path, begin, end, inner, body)
	if remove {
		return "", foreign, err
	}
	return body, foreign, err
}

// splitTextBlock treats whole lines equal to the current body, SteeringRule
// or a known legacy rule as glim's own content. Everything else is foreign.
// A rule embedded in a longer line is not glim's. If the block holds text but
// no known body, an older glim rule cannot be told from foreign text, so it
// refuses rather than guess.
func splitTextBlock(path, begin, end, inner, body string) (string, error) {
	lines := splitLines(inner)
	var known [][]string
	for _, k := range append([]string{body, SteeringRule}, legacySteeringRules...) {
		if k != "" {
			known = append(known, strings.Split(k, "\n"))
		}
	}
	type item struct {
		text string
		own  bool
	}
	var items []item
	found, anyText := false, false
	for i := 0; i < len(lines); {
		n := 0
		for _, kl := range known {
			if i+len(kl) <= len(lines) && equalLines(lines[i:i+len(kl)], kl) {
				n = len(kl)
				break
			}
		}
		if n > 0 {
			found = true
			items = append(items, item{own: true})
			i += n
			continue
		}
		if !isBlank(lines[i]) {
			anyText = true
		}
		items = append(items, item{text: lines[i]})
		i++
	}
	if !found && anyText {
		return "", fmt.Errorf("%s: the block between %q and %q has content glim does not recognise; "+
			"move or delete those lines by hand (keep anything you want outside the markers), then re-run", path, begin, end)
	}
	var out []string
	for i, it := range items {
		if it.own {
			continue
		}
		// Blank lines that only separated glim's rule from its neighbours go with it.
		if isBlank(it.text) && ((i > 0 && items[i-1].own) || (i+1 < len(items) && items[i+1].own)) {
			continue
		}
		out = append(out, it.text)
	}
	return strings.Join(trimBlankEnds(out), "\n"), nil
}

func equalLines(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return len(a) == len(b)
}

func isBareKeyChar(c byte) bool {
	return c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// parseKeySegs parses a TOML dotted key (bare, "basic" or 'literal' segments,
// whitespace allowed around dots) at the start of s. It returns the unquoted
// segments and the remainder starting at the first character after the key.
func parseKeySegs(s string) (segs []string, rest string, ok bool) {
	i := 0
	skip := func() {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
	}
	for {
		skip()
		if i >= len(s) {
			return nil, "", false
		}
		var seg string
		switch c := s[i]; {
		case c == '"':
			j := i + 1
			for j < len(s) && s[j] != '"' {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(s) {
				return nil, "", false
			}
			u, err := strconv.Unquote(s[i : j+1])
			if err != nil {
				u = s[i+1 : j]
			}
			seg, i = u, j+1
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, "", false
			}
			seg, i = s[i+1:i+1+j], i+j+2
		case isBareKeyChar(c):
			j := i
			for j < len(s) && isBareKeyChar(s[j]) {
				j++
			}
			seg, i = s[i:j], j
		default:
			return nil, "", false
		}
		segs = append(segs, seg)
		skip()
		if i < len(s) && s[i] == '.' {
			i++
			continue
		}
		return segs, s[i:], true
	}
}

// parseTOMLHeader returns the key segments of a [table] or [[array]] header
// line. A line that merely starts with "[" (such as an array element) is not
// a header.
func parseTOMLHeader(line string) ([]string, bool) {
	t := strings.TrimSpace(line)
	closer := "]"
	if strings.HasPrefix(t, "[[") {
		t, closer = t[2:], "]]"
	} else if strings.HasPrefix(t, "[") {
		t = t[1:]
	} else {
		return nil, false
	}
	segs, rest, ok := parseKeySegs(t)
	if !ok || !strings.HasPrefix(rest, closer) {
		return nil, false
	}
	rest = strings.TrimSpace(rest[len(closer):])
	if rest != "" && !strings.HasPrefix(rest, "#") {
		return nil, false
	}
	return segs, true
}

func isGlimSegs(segs []string) bool {
	return len(segs) >= 2 && segs[0] == "mcp_servers" && segs[1] == "glim"
}

func sameSegs(a, b []string) bool { return equalLines(a, b) }

// bracketDelta counts how many array/inline-table brackets a line opens minus
// closes, ignoring quoted strings and trailing comments.
func bracketDelta(s string) int {
	d := 0
	var q byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case q == '"':
			if c == '\\' {
				i++
			} else if c == '"' {
				q = 0
			}
		case q == '\'':
			if c == '\'' {
				q = 0
			}
		case c == '"' || c == '\'':
			q = c
		case c == '#':
			return d
		case c == '[' || c == '{':
			d++
		case c == ']' || c == '}':
			d--
		}
	}
	return d
}

type tomlTable struct {
	header string
	segs   []string
	raw    []string // every line after the header, up to the next header
}

// tomlUnit is one key (with any continuation lines of a multi-line value), a
// full-line comment, or an unclassified line inside a table.
type tomlUnit struct {
	key     string // dotted key, empty for comments and unclassified lines
	comment bool
	lines   []string
}

// unitsOf groups a table's lines into units. Blank lines are dropped.
func unitsOf(raw []string) []tomlUnit {
	var units []tomlUnit
	depth := 0
	for _, l := range raw {
		t := strings.TrimSpace(l)
		if n := len(units); n > 0 && depth > 0 {
			units[n-1].lines = append(units[n-1].lines, l)
			depth += bracketDelta(l)
			continue
		}
		switch {
		case t == "":
		case strings.HasPrefix(t, "#"):
			units = append(units, tomlUnit{comment: true, lines: []string{l}})
		default:
			u := tomlUnit{lines: []string{l}}
			depth = 0
			if segs, rest, ok := parseKeySegs(t); ok && strings.HasPrefix(rest, "=") {
				u.key = strings.Join(segs, ".")
				if depth = bracketDelta(rest[1:]); depth < 0 {
					depth = 0
				}
			}
			units = append(units, u)
		}
	}
	return units
}

// parseTOML splits lines into tables. Comment lines before the first header
// are returned separately; any other content before it is an error.
func parseTOML(path, begin, end string, lines []string) (preamble []string, tables []tomlTable, err error) {
	for _, l := range lines {
		if segs, ok := parseTOMLHeader(l); ok {
			tables = append(tables, tomlTable{header: l, segs: segs})
			continue
		}
		if len(tables) == 0 {
			switch t := strings.TrimSpace(l); {
			case t == "":
			case strings.HasPrefix(t, "#"):
				preamble = append(preamble, l)
			default:
				return nil, nil, fmt.Errorf("%s: the block between %q and %q has key/value lines before any table header; "+
					"move or delete them by hand (keep anything you want outside the markers), then re-run", path, begin, end)
			}
			continue
		}
		last := &tables[len(tables)-1]
		last.raw = append(last.raw, l)
	}
	return preamble, tables, nil
}

func (t tomlTable) lines() []string {
	return append([]string{t.header}, trimEndBlank(t.raw)...)
}

func trimEndBlank(lines []string) []string {
	for len(lines) > 0 && isBlank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// splitTOMLBlock works on TOML structure, comparing parsed table headers
// (never raw text). Tables other than [mcp_servers.glim] and its sub-tables
// are foreign and kept byte for byte.
//
// On install, glim's body replaces the keys it writes. Any other key (and
// comment) the user or another tool added to glim's table is re-emitted in
// the new table after glim's keys, and sub-tables the body does not write
// (for example a user-added .env) stay attached to glim inside the block.
// On remove, glim's table and sub-tables are dropped (the server is being
// removed); only full-line comments from them are kept.
//
// It refuses (rather than guesses) when the block has bare key/value lines
// before any table header, which would attach to a different table once
// moved, or any multi-line string, inside which a header-looking line cannot
// be told from content.
func splitTOMLBlock(path, begin, end, inner, body string, remove bool) (string, string, error) {
	if strings.Contains(inner, `'''`) || strings.Contains(inner, `"""`) {
		return "", "", fmt.Errorf("%s: the block between %q and %q contains a multi-line string, so glim cannot safely tell "+
			"its own table from other content; move the other tables outside the markers by hand, then re-run", path, begin, end)
	}
	comments, tables, err := parseTOML(path, begin, end, splitLines(inner))
	if err != nil {
		return "", "", err
	}
	var bodyTables []tomlTable
	if !remove {
		if _, bodyTables, err = parseTOML(path, begin, end, splitLines(body)); err != nil {
			return "", "", err
		}
	}
	extras := make([][]string, len(bodyTables))
	var subs, foreignTables []string
	for _, tb := range tables {
		if !isGlimSegs(tb.segs) {
			foreignTables = append(foreignTables, strings.Join(tb.lines(), "\n"))
			continue
		}
		units := unitsOf(tb.raw)
		if remove {
			for _, u := range units {
				if u.comment {
					comments = append(comments, u.lines...)
				}
			}
			continue
		}
		bi := -1
		for i, bt := range bodyTables {
			if sameSegs(bt.segs, tb.segs) {
				bi = i
				break
			}
		}
		if bi < 0 {
			subs = append(subs, strings.Join(tb.lines(), "\n"))
			continue
		}
		own := map[string]bool{}
		for _, u := range unitsOf(bodyTables[bi].raw) {
			if u.key != "" {
				own[u.key] = true
			}
		}
		for _, u := range units {
			if u.key == "" || !own[u.key] {
				extras[bi] = append(extras[bi], u.lines...)
			}
		}
	}
	var parts []string
	for i, bt := range bodyTables {
		parts = append(parts, strings.Join(append(bt.lines(), extras[i]...), "\n"))
	}
	newInner := strings.Join(append(parts, subs...), "\n\n")

	foreign := strings.Join(comments, "\n")
	if len(foreignTables) > 0 {
		if foreign != "" {
			foreign += "\n\n"
		}
		foreign += strings.Join(foreignTables, "\n\n")
	}
	return newInner, foreign, nil
}

// blockSpan holds byte offsets of a managed block: start of the begin-marker
// line, the inner content (after the begin line's terminator, up to the start
// of the end-marker line) and the end of the end marker.
type blockSpan struct{ start, innerStart, innerEnd, end int }

// findBlock locates the managed block. A marker only counts when it is the
// whole line (ignoring surrounding whitespace), so marker text inside another
// tool's content or a string value cannot open or close the block.
func findBlock(content, begin, end string) (blockSpan, bool) {
	var sp blockSpan
	begun := false
	for pos := 0; pos <= len(content); {
		lineEnd := len(content)
		if nl := strings.IndexByte(content[pos:], '\n'); nl != -1 {
			lineEnd = pos + nl
		}
		t := strings.TrimRight(content[pos:lineEnd], "\r")
		switch {
		case !begun && strings.TrimSpace(t) == begin:
			begun = true
			sp.start = pos
			sp.innerStart = lineEnd + 1
		case begun && strings.TrimSpace(t) == end:
			sp.innerEnd, sp.end = pos, pos+len(t)
			return sp, true
		}
		pos = lineEnd + 1
	}
	return sp, false
}

// upsertBlock writes glim's block between the markers. Anything inside the
// markers that glim did not write (see splitBlock) is moved to just after the
// end marker instead of being overwritten. Separators glim inserts use the
// file's existing line ending.
func upsertBlock(path, begin, end, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(existing)
	eol := detectEOL(content)
	conv := func(s string) string { return strings.ReplaceAll(s, "\n", eol) }
	body = strings.ReplaceAll(body, "\r\n", "\n")

	if sp, ok := findBlock(content, begin, end); ok {
		inner, foreign, err := splitBlock(path, begin, end, content[sp.innerStart:sp.innerEnd], body, false)
		if err != nil {
			return err
		}
		block := begin + eol + conv(inner) + eol + end
		if foreign != "" {
			block += eol + eol + conv(foreign)
			if tail := content[sp.end:]; !strings.HasPrefix(tail, "\n") && !strings.HasPrefix(tail, "\r\n") {
				block += eol
			}
		}
		content = content[:sp.start] + block + content[sp.end:]
		return os.WriteFile(path, []byte(content), 0o644)
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += eol
	}
	if content != "" {
		content += eol
	}
	content += begin + eol + conv(body) + eol + end + eol
	return os.WriteFile(path, []byte(content), 0o644)
}

// removeBlock deletes glim's own content and the markers from path, leaving
// surrounding content intact without stacked blank lines. Foreign content
// found inside the markers stays where the block was. It reports whether a
// block was removed.
func removeBlock(path, begin, end string) (bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	content := string(data)
	sp, ok := findBlock(content, begin, end)
	if !ok {
		return false, nil
	}
	_, foreign, err := splitBlock(path, begin, end, content[sp.innerStart:sp.innerEnd], "", true)
	if err != nil {
		return false, err
	}
	eol := detectEOL(content)
	before := strings.TrimRight(content[:sp.start], "\r\n")
	rest := strings.TrimLeft(content[sp.end:], "\r\n")
	var parts []string
	for _, p := range []string{before, strings.ReplaceAll(foreign, "\n", eol), rest} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	out := strings.Join(parts, eol+eol)
	if rest == "" && out != "" {
		out += eol
	}
	return true, os.WriteFile(path, []byte(out), 0o644)
}

func mergeCursorMCP(path, name, command string, args []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("existing %s is not valid JSON: %w", path, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers[name] = map[string]any{"command": command, "args": args}
	root["mcpServers"] = servers
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

func DefaultRun(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
