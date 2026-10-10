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

// sandboxNote tells an agent what a preview page can and cannot do. Previews
// are served with a sandbox Content-Security-Policy and no allow-same-origin,
// so every page runs in an opaque origin.
const sandboxNote = "Previews run in a sandbox: write a self-contained page; localStorage, sessionStorage, cookies, " +
	"IndexedDB and service workers throw (wrap them in try/catch), fetch/XHR of the page's own files fails " +
	"(inline the data), and scripts, styles, images and fonts from a CDN work."

const steeringBase = "To show the user any HTML, Markdown or other visual preview, call the glim `present` MCP tool " +
	"and give the user the returned link. To update a preview, call `present` again with the same `name`: " +
	"the link stays the same, and open tabs usually refresh by themselves (live reload, on by default). "

// SteeringRule is the rule written for Claude, which has its own artifacts.
const SteeringRule = steeringBase + sandboxNote +
	" Do not use Claude artifacts or other built-in preview mechanisms."

// SteeringRuleOther is the rule written for Codex and Cursor, which have no
// Claude artifacts to steer away from.
const SteeringRuleOther = steeringBase + sandboxNote +
	" Prefer it over other built-in preview mechanisms."

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

	addArgs := []string{"mcp", "add", "--scope", "user", "glim", "--", d.GlimPath, "mcp"}
	// `claude mcp get` exits 0 only when glim is registered. A registration is
	// replaced (remove + add) so a re-run repoints it at the current binary;
	// nothing is removed unless one exists.
	if d.Run("claude", "mcp", "get", "glim") != nil {
		if err := d.Run("claude", addArgs...); err != nil {
			return nil, fmt.Errorf("claude mcp add failed: %w", err)
		}
		steps = append(steps, "registered glim MCP server (claude mcp add --scope user)")
	} else if rmErr := d.Run("claude", "mcp", "remove", "--scope", "user", "glim"); rmErr != nil {
		// get also sees project/local-scope registrations; with no user-scope
		// one, remove fails and nothing was removed, so a plain add is safe.
		if err := d.Run("claude", addArgs...); err != nil {
			return nil, fmt.Errorf("claude mcp remove --scope user failed (%w) and claude mcp add failed (%w); any existing glim registration was left unchanged", rmErr, err)
		}
		steps = append(steps, "registered glim MCP server (claude mcp add --scope user)")
	} else {
		if err := d.Run("claude", addArgs...); err != nil {
			return nil, fmt.Errorf("glim is now UNREGISTERED from Claude: claude mcp add failed after removing the old registration (%w); re-add it with: claude mcp add --scope user glim -- %s mcp",
				err, shellQuote(d.GlimPath))
		}
		steps = append(steps, "updated glim MCP server registration (claude mcp remove + add --scope user)")
	}

	path := filepath.Join(d.Home, ".claude", "CLAUDE.md")
	if err := upsertBlock(path, blockBeginMD, blockEndMD, SteeringRule); err != nil {
		return steps, err
	}
	steps = append(steps, "wrote steering rule to "+path)
	return steps, nil
}

// shellQuote quotes s for a POSIX shell, so a printed command can be pasted
// as is even when the path holds spaces or quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func installCodex(d Deps) ([]string, error) {
	if d.HasCommand != nil && !d.HasCommand("codex") {
		return nil, fmt.Errorf("codex is not installed")
	}
	var steps []string

	cfg := filepath.Join(d.Home, ".codex", "config.toml")
	toml := fmt.Sprintf("[mcp_servers.glim]\ncommand = %q\nargs = [\"mcp\"]", d.GlimPath)
	adopted, err := upsertBlockReport(cfg, blockBeginTOML, blockEndTOML, toml)
	if err != nil {
		return nil, err
	}
	steps = append(steps, "registered glim MCP server in "+cfg)
	if adopted {
		steps = append(steps, "adopted the existing [mcp_servers.glim] table into glim's managed block; its extra keys and sub-tables are kept but `glim uninstall codex` will remove them with the block")
	}

	path := filepath.Join(d.Home, ".codex", "AGENTS.md")
	if err := upsertBlock(path, blockBeginMD, blockEndMD, SteeringRuleOther); err != nil {
		return steps, err
	}
	steps = append(steps, "wrote steering rule to "+path)
	return steps, nil
}

// cursorRuleDescription is the glim.mdc frontmatter description glim writes;
// legacyCursorRuleDescriptions are earlier ones, still recognised on uninstall.
const cursorRuleDescription = "Prefer glim for HTML and other visual previews"

var legacyCursorRuleDescriptions = []string{"Prefer glim for HTML previews"}

// cursorRuleBody is the full glim.mdc file for a given description and
// steering rule wording.
func cursorRuleBody(desc, rule string) string {
	return "---\ndescription: " + desc + "\nalwaysApply: true\n---\n\n" + rule + "\n"
}

func installCursor(d Deps) ([]string, error) {
	var steps []string

	mcpPath := filepath.Join(d.Home, ".cursor", "mcp.json")
	if err := mergeCursorMCP(mcpPath, "glim", d.GlimPath, []string{"mcp"}); err != nil {
		return nil, err
	}
	steps = append(steps, "registered glim MCP server in "+mcpPath)

	rule := filepath.Join(d.Home, ".cursor", "rules", "glim.mdc")
	body := cursorRuleBody(cursorRuleDescription, SteeringRuleOther)
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
	// The wording before the sandbox note, written for every agent.
	"To show the user any HTML, Markdown or other visual preview, call the glim `present` MCP tool " +
		"and give the user the returned link. To update a preview, call `present` again with the same `name`: " +
		"the link stays the same, and open tabs usually refresh by themselves (live reload, on by default). " +
		"Do not use Claude artifacts or other built-in preview mechanisms.",
	"To show the user any HTML or visual preview, call the glim `present` MCP tool " +
		"and give the user the returned link. Do not use Claude artifacts or other built-in preview mechanisms.",
}

// detectEOL returns the file's dominant line ending, used for the separators
// glim inserts: CRLF if it outnumbers bare LF, LF otherwise. Lines that glim
// moves keep their own terminators.
func detectEOL(content string) string {
	crlf := strings.Count(content, "\r\n")
	if crlf > strings.Count(content, "\n")-crlf {
		return "\r\n"
	}
	return "\n"
}

// splitLines splits text on LF. Each line keeps a trailing CR if it had one,
// so joining lines with "\n" restores the original bytes. A final line
// terminator does not produce an empty last line.
func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func stripCR(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimSuffix(l, "\r")
	}
	return out
}

// joinMoved joins lines for moving into a new place: lines keep their own
// terminators, and the final line's CR is dropped because the caller supplies
// the terminator that follows.
func joinMoved(lines []string) string {
	return strings.TrimSuffix(strings.Join(lines, "\n"), "\r")
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
// of glim's block (LF separated) and the foreign content to keep outside of
// it (original bytes, lines keep their own terminators). With remove set,
// glim's own content is dropped entirely and the first result is empty. eol
// is the separator glim itself inserts inside the foreign content.
//
// The policy is conservative: whenever glim's content cannot be told from
// foreign content with certainty it returns an error instead of guessing, and
// content it does not own is moved verbatim. TOML blocks are split
// structurally by table; every other block is matched against the bodies glim
// has written.
func splitBlock(path, begin, end, inner, body string, remove bool, eol string) (string, string, error) {
	if begin == blockBeginTOML {
		return splitTOMLBlock(path, begin, end, inner, body, remove, eol)
	}
	foreign, err := splitTextBlock(path, begin, end, inner, body)
	if remove {
		return "", foreign, err
	}
	return body, foreign, err
}

// splitTextBlock treats whole lines equal to the current body, either
// steering rule or a known legacy rule as glim's own content. Everything else is foreign.
// A rule embedded in a longer line is not glim's. If the block holds text but
// no known body, an older glim rule cannot be told from foreign text, so it
// refuses rather than guess.
func splitTextBlock(path, begin, end, inner, body string) (string, error) {
	lines := splitLines(inner)
	var known [][]string
	for _, k := range append([]string{body, SteeringRule, SteeringRuleOther}, legacySteeringRules...) {
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
			if i+len(kl) <= len(lines) && equalLines(stripCR(lines[i:i+len(kl)]), kl) {
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
	return joinMoved(trimBlankEnds(out)), nil
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

// scanValue reports how many array/inline-table brackets a value opens minus
// closes and whether it has a trailing comment, ignoring quoted strings.
func scanValue(s string) (depth int, comment bool) {
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
			return depth, true
		case c == '[' || c == '{':
			depth++
		case c == ']' || c == '}':
			depth--
		}
	}
	return depth, false
}

// valueOf splits a key/value line into its dotted key (empty if the key is
// not one glim can parse) and the text after the "=". It reports false for
// blank lines, comments and lines without "=".
func valueOf(line string) (key, value string, ok bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return "", "", false
	}
	if segs, rest, ok := parseKeySegs(t); ok && strings.HasPrefix(rest, "=") {
		return strings.Join(segs, "."), rest[1:], true
	}
	if i := strings.Index(t, "="); i >= 0 {
		return "", t[i+1:], true
	}
	return "", "", false
}

type tomlTable struct {
	header string
	segs   []string
	raw    []string // every line after the header, up to the next header
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

// lines returns the header and body of a table without trailing blank lines
// or CRs. It is used for content glim re-emits inside its own block.
func (t tomlTable) lines() []string {
	raw := t.raw
	for len(raw) > 0 && isBlank(raw[len(raw)-1]) {
		raw = raw[:len(raw)-1]
	}
	return stripCR(append([]string{t.header}, raw...))
}

// splitTOMLBlock works on TOML structure, comparing parsed table headers
// (never raw text). Tables other than [mcp_servers.glim] and its sub-tables
// are foreign and moved byte for byte.
//
// On install, glim's body replaces the keys it writes. Every other line of
// glim's table (keys, full-line comments, blank lines) is re-emitted after
// glim's keys in its original order, and sub-tables the body does not write
// (for example a user-added .env) stay attached to glim inside the block. On
// remove, glim's table and sub-tables are dropped (the server is being
// removed); only full-line comments from them are kept.
//
// Conservative refusals, each leaving the file untouched:
//   - any multi-line string (triple-quoted), inside which a header-looking line
//     cannot be told from content;
//   - any key whose value opens an array or inline table without closing it
//     on the same line, for the same reason (a continuation line such as
//     ["x"] looks like a table header);
//   - bare key/value lines before any table header, which would attach to a
//     different table once moved;
//   - on install, a trailing comment on a line glim owns (a key its body
//     writes): glim would overwrite that line and lose the comment.
func splitTOMLBlock(path, begin, end, inner, body string, remove bool, eol string) (string, string, error) {
	if strings.Contains(inner, `'''`) || strings.Contains(inner, `"""`) {
		return "", "", fmt.Errorf("%s: the block between %q and %q contains a multi-line string, so glim cannot safely tell "+
			"its own table from other content; move the other tables outside the markers by hand, then re-run", path, begin, end)
	}
	lines := splitLines(inner)
	for _, l := range lines {
		if _, ok := parseTOMLHeader(l); ok {
			continue
		}
		if _, v, ok := valueOf(l); ok {
			if d, _ := scanValue(v); d > 0 {
				return "", "", fmt.Errorf("%s: the block between %q and %q contains a multi-line value (an array or inline table "+
					"spread over several lines), so glim cannot safely tell its own table from other content; "+
					"put each such value on one line or move it outside the markers by hand, then re-run", path, begin, end)
			}
		}
	}
	comments, tables, err := parseTOML(path, begin, end, lines)
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
	var subs []string
	var foreignLines []string
	for _, tb := range tables {
		if !isGlimSegs(tb.segs) {
			foreignLines = append(append(foreignLines, tb.header), tb.raw...)
			continue
		}
		if remove {
			for _, l := range tb.raw {
				if strings.HasPrefix(strings.TrimSpace(l), "#") {
					comments = append(comments, l)
				}
			}
			continue
		}
		bi := -1
		for i, bt := range bodyTables {
			if equalLines(bt.segs, tb.segs) {
				bi = i
				break
			}
		}
		if bi < 0 {
			subs = append(subs, strings.Join(tb.lines(), "\n"))
			continue
		}
		own := map[string]bool{}
		for _, l := range bodyTables[bi].raw {
			if k, _, ok := valueOf(l); ok && k != "" {
				own[k] = true
			}
		}
		var rest []string
		for _, l := range tb.raw {
			if k, v, ok := valueOf(l); ok && k != "" && own[k] {
				if _, c := scanValue(v); c {
					return "", "", fmt.Errorf("%s: %q in the glim table has a trailing comment; glim owns that line and would "+
						"overwrite it, so move the comment onto its own line (or delete it), then re-run", path, strings.TrimSpace(l))
				}
				continue
			}
			rest = append(rest, l)
		}
		extras[bi] = stripCR(trimBlankEnds(rest))
	}
	var parts []string
	for i, bt := range bodyTables {
		parts = append(parts, strings.Join(append(bt.lines(), extras[i]...), "\n"))
	}
	newInner := strings.Join(append(parts, subs...), "\n\n")

	var all []string
	all = append(all, comments...)
	foreignLines = trimBlankEnds(foreignLines)
	if len(comments) > 0 && len(foreignLines) > 0 {
		sep := ""
		if eol == "\r\n" {
			sep = "\r"
		}
		all = append(all, sep)
	}
	all = append(all, foreignLines...)
	return newInner, joinMoved(all), nil
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
// file's dominant line ending.
func upsertBlock(path, begin, end, body string) error {
	_, err := upsertBlockReport(path, begin, end, body)
	return err
}

// utf8BOM may start a file written by a Windows editor. It is not whitespace
// to TOML scanning, so it is set aside while editing and put back on write.
const utf8BOM = "\ufeff"

// upsertBlockReport is upsertBlock that also reports whether an existing
// unmanaged glim table was adopted into the managed block.
func upsertBlockReport(path, begin, end, body string) (adopted bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	content := string(existing)
	bom := ""
	if strings.HasPrefix(content, utf8BOM) {
		bom, content = utf8BOM, strings.TrimPrefix(content, utf8BOM)
	}
	write := func(s string) error { return os.WriteFile(path, []byte(bom+s), 0o644) }
	eol := detectEOL(content)
	conv := func(s string) string { return strings.ReplaceAll(s, "\n", eol) }
	body = strings.ReplaceAll(body, "\r\n", "\n")

	if sp, ok := findBlock(content, begin, end); ok {
		inner, foreign, err := splitBlock(path, begin, end, content[sp.innerStart:sp.innerEnd], body, false, eol)
		if err != nil {
			return false, err
		}
		block := begin + eol + conv(inner) + eol + end
		if foreign != "" {
			// The text after the end marker supplies the final terminator.
			block += eol + eol + foreign
		}
		if begin == blockBeginTOML {
			if err := checkNoUnmanagedGlim(path, content[:sp.start]+"\n"+content[sp.end:]); err != nil {
				return false, err
			}
		}
		content = content[:sp.start] + block + content[sp.end:]
		return false, write(content)
	}
	if begin == blockBeginTOML {
		adoptedContent, changed, err := adoptUnmanagedGlim(path, content, begin, end, body, eol)
		if err != nil {
			return false, err
		}
		if changed {
			return true, write(adoptedContent)
		}
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += eol
	}
	if content != "" {
		content += eol
	}
	content += begin + eol + conv(body) + eol + end + eol
	return false, write(content)
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
	content := strings.TrimPrefix(string(data), utf8BOM)
	bom := string(data)[:len(data)-len(content)]
	sp, ok := findBlock(content, begin, end)
	if !ok {
		return false, nil
	}
	eol := detectEOL(content)
	_, foreign, err := splitBlock(path, begin, end, content[sp.innerStart:sp.innerEnd], "", true, eol)
	if err != nil {
		return false, err
	}
	before := strings.TrimRight(content[:sp.start], "\r\n")
	rest := strings.TrimLeft(content[sp.end:], "\r\n")
	var parts []string
	for _, p := range []string{before, foreign, rest} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	out := strings.Join(parts, eol+eol)
	if rest == "" && out != "" {
		out += eol
	}
	if out != "" {
		out = bom + out
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
