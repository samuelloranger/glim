package install

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// splitBlock separates what glim wrote between the markers from anything
// else another tool or the user put there. It returns the foreign content
// verbatim (own content removed, no surrounding blank lines) or an error when
// the two cannot be told apart safely. TOML blocks are split structurally by
// table; every other block is matched against the bodies glim has written.
func splitBlock(path, begin, end, inner, body string) (string, error) {
	if begin == blockBeginTOML {
		return splitTOMLBlock(path, begin, end, inner)
	}
	return splitTextBlock(path, begin, end, inner, body)
}

// splitTextBlock treats the current body and the known legacy rules as glim's
// own content. Everything else is foreign. If the block holds text but none of
// those bodies, an older glim rule cannot be told from foreign text, so it
// refuses rather than guess.
func splitTextBlock(path, begin, end, inner, body string) (string, error) {
	if strings.TrimSpace(inner) == "" {
		return "", nil
	}
	const mark = "\x00"
	known := append([]string{body, SteeringRule}, legacySteeringRules...)
	found := false
	for _, k := range known {
		if k != "" && strings.Contains(inner, k) {
			found = true
			inner = strings.ReplaceAll(inner, k, mark)
		}
	}
	if !found {
		return "", fmt.Errorf("%s: the block between %q and %q has content glim does not recognise; "+
			"move or delete those lines by hand (keep anything you want outside the markers), then re-run", path, begin, end)
	}
	var parts []string
	for _, p := range strings.Split(inner, mark) {
		if p = strings.Trim(p, "\r\n"); strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "\n"), nil
}

var tomlHeader = regexp.MustCompile(`^\s*\[\[?[^\[\],]+\]\]?\s*(#.*)?$`)

// isGlimTOMLHeader reports whether a table header line names the
// mcp_servers.glim table or one of its sub-tables.
func isGlimTOMLHeader(line string) bool {
	h := strings.TrimSpace(line)
	if i := strings.Index(h, "]"); i != -1 {
		h = h[:i]
	}
	h = strings.NewReplacer("[", "", "\"", "", "'", "", " ", "", "\t", "").Replace(h)
	return h == "mcp_servers.glim" || strings.HasPrefix(h, "mcp_servers.glim.")
}

// splitTOMLBlock treats the [mcp_servers.glim] table and its sub-tables as
// glim's own content and every other table as foreign, kept byte for byte.
// Full-line comments inside glim's tables are kept as foreign (glim writes
// none). Bare key/value lines before the first table header are refused: once
// moved they would silently attach to a different table.
func splitTOMLBlock(path, begin, end, inner string) (string, error) {
	var tables []string // foreign tables, each starting at its header
	var cur []string
	var comments []string // comments from the preamble and from glim's tables
	curGlim, started := false, false
	flush := func() {
		if started && !curGlim {
			tables = append(tables, strings.Trim(strings.Join(cur, "\n"), "\r\n"))
		}
		cur = nil
	}
	for _, line := range strings.Split(inner, "\n") {
		if tomlHeader.MatchString(line) {
			flush()
			started, curGlim = true, isGlimTOMLHeader(line)
			if !curGlim {
				cur = append(cur, line)
			}
			continue
		}
		trim := strings.TrimSpace(line)
		switch {
		case !started:
			if trim != "" && !strings.HasPrefix(trim, "#") {
				return "", fmt.Errorf("%s: the block between %q and %q has key/value lines before any table header; "+
					"move or delete them by hand (keep anything you want outside the markers), then re-run", path, begin, end)
			}
			if trim != "" {
				comments = append(comments, line)
			}
		case curGlim:
			if strings.HasPrefix(trim, "#") {
				comments = append(comments, line)
			}
		default:
			cur = append(cur, line)
		}
	}
	flush()
	res := strings.Join(comments, "\n")
	if len(tables) > 0 {
		if res != "" {
			res += "\n\n"
		}
		res += strings.Join(tables, "\n\n")
	}
	return res, nil
}

// findBlock locates the managed block, returning the offsets of the begin
// marker, the end marker, and the end of the end marker.
func findBlock(content, begin, end string) (b, e, after int, ok bool) {
	b = strings.Index(content, begin)
	if b == -1 {
		return 0, 0, 0, false
	}
	i := strings.Index(content[b+len(begin):], end)
	if i == -1 {
		return 0, 0, 0, false
	}
	e = b + len(begin) + i
	return b, e, e + len(end), true
}

// upsertBlock writes glim's block between the markers. Anything inside the
// markers that glim did not write (see splitBlock) is moved to just after the
// end marker instead of being overwritten.
func upsertBlock(path, begin, end, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	block := begin + "\n" + body + "\n" + end
	content := string(existing)
	if b, e, after, ok := findBlock(content, begin, end); ok {
		foreign, err := splitBlock(path, begin, end, content[b+len(begin):e], body)
		if err != nil {
			return err
		}
		if foreign != "" {
			block += "\n\n" + foreign
			if !strings.HasPrefix(content[after:], "\n") {
				block += "\n"
			}
		}
		content = content[:b] + block + content[after:]
		return os.WriteFile(path, []byte(content), 0o644)
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if content != "" {
		content += "\n"
	}
	content += block + "\n"
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
	b, e, after, ok := findBlock(content, begin, end)
	if !ok {
		return false, nil
	}
	foreign, err := splitBlock(path, begin, end, content[b+len(begin):e], "")
	if err != nil {
		return false, err
	}
	before := strings.TrimRight(content[:b], "\n")
	rest := strings.TrimLeft(content[after:], "\n")
	var parts []string
	for _, p := range []string{before, foreign, rest} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	out := strings.Join(parts, "\n\n")
	if rest == "" && out != "" {
		out += "\n"
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
