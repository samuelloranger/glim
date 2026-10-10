package install

import (
	"fmt"
	"strings"
)

// glimTableUse describes how a TOML file mentions the glim MCP server table.
type glimTableUse struct {
	headers   []int // line indexes of [mcp_servers.glim...] headers
	array     bool  // an [[array of tables]] header names glim
	dotted    bool  // glim is defined by dotted keys or an inline table
	duplicate bool  // the same glim table header appears twice
}

func (u glimTableUse) any() bool { return len(u.headers) > 0 || u.array || u.dotted }

// scanGlimUse looks for definitions of [mcp_servers.glim] (or its sub-tables)
// in lines. It is a line scan, so a header-looking line inside a multi-line
// string is a false positive; callers refuse such files rather than guess.
func scanGlimUse(lines []string) glimTableUse {
	var u glimTableUse
	var cur []string
	seen := map[string]bool{}
	for i, l := range lines {
		l = strings.TrimSuffix(l, "\r")
		if segs, ok := parseTOMLHeader(l); ok {
			cur = segs
			if isGlimSegs(segs) {
				if strings.HasPrefix(strings.TrimSpace(l), "[[") {
					u.array = true
					continue
				}
				u.headers = append(u.headers, i)
				k := strings.Join(segs, "\x00")
				if seen[k] {
					u.duplicate = true
				}
				seen[k] = true
			}
			continue
		}
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		segs, rest, ok := parseKeySegs(t)
		if !ok || !strings.HasPrefix(rest, "=") {
			continue
		}
		full := append(append([]string{}, cur...), segs...)
		if !isGlimSegs(cur) && isGlimSegs(full) {
			u.dotted = true
		} else if len(full) == 1 && full[0] == "mcp_servers" && strings.Contains(rest, "glim") {
			u.dotted = true // mcp_servers = { glim = { ... } }
		}
	}
	return u
}

// hasMultilineConstruct reports whether the file contains a multi-line string
// or a value that opens an array or inline table without closing it on its
// line, inside which a header-looking line cannot be told from content.
func hasMultilineConstruct(content string, lines []string) bool {
	if strings.Contains(content, `'''`) || strings.Contains(content, `"""`) {
		return true
	}
	for _, l := range lines {
		if _, ok := parseTOMLHeader(l); ok {
			continue
		}
		if _, v, ok := valueOf(l); ok {
			if d, _ := scanValue(v); d > 0 {
				return true
			}
		}
	}
	return false
}

func unmanagedGlimError(path, why string) error {
	return fmt.Errorf("%s already defines [mcp_servers.glim] outside glim's markers (%s), so glim cannot "+
		"add its own table without duplicating it, which makes Codex reject the whole file; "+
		"remove or rewrite that table by hand (or run `codex mcp remove glim`), then re-run", path, why)
}

// checkNoUnmanagedGlim refuses when content defines the glim table at all.
// It is used for files that already have a managed block, where an extra
// table outside the markers would be a duplicate.
func checkNoUnmanagedGlim(path, content string) error {
	if content == "" {
		return nil
	}
	if scanGlimUse(splitLines(content)).any() {
		return unmanagedGlimError(path, "it is defined both inside and outside the markers")
	}
	return nil
}

// adoptUnmanagedGlim handles a TOML file that has no glim markers but already
// defines [mcp_servers.glim] (for example from `codex mcp add glim` or by
// hand). Appending a managed block would define the table twice, which Codex
// rejects, so the existing glim tables are moved into the managed block in
// place of the first one; keys glim writes are replaced and everything else in
// them (extra keys, a user-added .env sub-table) is kept, as inside a block.
//
// It returns changed=false when the file does not define glim at all. It
// refuses, leaving the file untouched, when the definition cannot be moved
// safely: dotted keys or an inline table, an array of tables, a repeated
// header, or any multi-line string or value that could hide a header.
func adoptUnmanagedGlim(path, content, begin, end, body, eol string) (string, bool, error) {
	if content == "" {
		return "", false, nil
	}
	lines := strings.Split(content, "\n")
	trailingNL := lines[len(lines)-1] == ""
	if trailingNL {
		lines = lines[:len(lines)-1]
	}
	u := scanGlimUse(lines)
	if !u.any() {
		return "", false, nil
	}
	switch {
	case u.dotted:
		return "", false, unmanagedGlimError(path, "defined with dotted keys or an inline table")
	case u.array:
		return "", false, unmanagedGlimError(path, "defined as an array of tables")
	case u.duplicate:
		return "", false, unmanagedGlimError(path, "the same table appears more than once")
	case hasMultilineConstruct(content, lines):
		return "", false, unmanagedGlimError(path, "the file has multi-line strings or values, so table boundaries cannot be told apart safely")
	}

	isHeader := func(i int) bool { _, ok := parseTOMLHeader(lines[i]); return ok }
	// Each glim table spans its header to the next header, minus trailing
	// blank and comment lines, which stay where they are.
	type span struct{ from, to int }
	var spans []span
	for _, h := range u.headers {
		j := h + 1
		for j < len(lines) && !isHeader(j) {
			j++
		}
		to := j
		for to > h+1 && (isBlank(lines[to-1]) || strings.HasPrefix(strings.TrimSpace(lines[to-1]), "#")) {
			to--
		}
		spans = append(spans, span{h, to})
	}
	var inner []string
	for i, sp := range spans {
		if i > 0 {
			inner = append(inner, "")
		}
		inner = append(inner, stripCR(lines[sp.from:sp.to])...)
	}
	newInner, foreign, err := splitTOMLBlock(path, begin, end, strings.Join(inner, "\n"), body, false, eol)
	if err != nil {
		return "", false, err
	}
	if foreign != "" {
		return "", false, unmanagedGlimError(path, "unexpected content between its tables")
	}
	cr := ""
	if eol == "\r\n" {
		cr = "\r"
	}
	block := []string{begin + cr}
	for _, l := range strings.Split(newInner, "\n") {
		block = append(block, l+cr)
	}
	block = append(block, end+cr)

	var out []string
	for i, si := 0, 0; i < len(lines); {
		if si < len(spans) && i == spans[si].from {
			if si == 0 {
				out = append(out, block...)
			}
			i = spans[si].to
			si++
			if si > 1 {
				// A removed later table must not leave stacked blank lines.
				for i < len(lines) && isBlank(lines[i]) && len(out) > 0 && isBlank(out[len(out)-1]) {
					i++
				}
			}
			continue
		}
		out = append(out, lines[i])
		i++
	}
	// Blank lines that only separated a removed table from the end of the
	// file go with it.
	tailBlank := true
	for _, l := range lines[spans[len(spans)-1].to:] {
		tailBlank = tailBlank && isBlank(l)
	}
	if tailBlank {
		for len(out) > 0 && isBlank(out[len(out)-1]) {
			out = out[:len(out)-1]
		}
	}
	res := strings.Join(out, "\n")
	if trailingNL {
		res += "\n"
	}
	return res, true, nil
}
