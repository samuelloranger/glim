package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// legacySkillTexts are earlier SKILL.md bodies glim wrote. They are still
// recognised as glim's own so uninstall can remove them.
var legacySkillTexts = []string{`---
name: glim
description: Show the user an HTML, Markdown, text, JSON or image preview via a short-lived link. Use when a coding task benefits from a visual preview.
---

# glim

Use glim to show the user a preview of something you built.

1. Create a self-contained HTML file or directory (or a .md, .txt, .json or image file).
2. Publish it: ` + "`glim <entry> --title <title>`" + `. ` + "`<entry>`" + ` is the file or directory.
3. Give the user the link glim prints.

To update a preview, republish to the same link with ` + "`--name <slug>`" + `.

Previews expire automatically. Remove one early with ` + "`glim rm <name>`" + `.
`}

// SkillText is the SKILL.md written by `glim install --skill`.
const SkillText = `---
name: glim
description: Show the user an HTML, Markdown, text, JSON or image preview via a short-lived link. Use when a coding task benefits from a visual preview.
---

# glim

Use glim to show the user a preview of something you built.

1. Create a self-contained HTML file or directory (or a .md, .txt, .json or image file).
2. Publish it: ` + "`glim <entry> --title <title>`" + `. ` + "`<entry>`" + ` is the file or directory.
3. Give the user the link glim prints.

To update a preview, republish to the same link with ` + "`--name <slug>`" + `. The link stays the
same, and open tabs usually refresh by themselves (live reload, on by default).

To keep a preview private, add ` + "`--password`" + ` and pipe the password (8 to 72 bytes) on stdin:
` + "`printf '%s\\n' \"$PW\" | glim <entry> --password`" + `. Visitors must enter it first.
Republishing with ` + "`--name`" + ` and no ` + "`--password`" + ` keeps the existing password.

Previews expire automatically. Remove one early with ` + "`glim rm <name>`" + `.
`

// skillDir returns the user-level skills directory for target.
func skillDir(target string, d Deps) (string, error) {
	switch target {
	case "claude":
		return filepath.Join(d.Home, ".claude", "skills", "glim"), nil
	case "codex":
		return filepath.Join(d.Home, ".agents", "skills", "glim"), nil
	case "cursor":
		// Cursor documents ~/.cursor/skills/ as its user-level skills directory.
		return filepath.Join(d.Home, ".cursor", "skills", "glim"), nil
	default:
		return "", fmt.Errorf("unknown target %q (want claude|codex|cursor)", target)
	}
}

// InstallSkill writes a SKILL.md for target instead of registering the MCP
// server and steering rule. Re-running overwrites.
func InstallSkill(target string, d Deps) ([]string, error) {
	dir, err := skillDir(target, d)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(SkillText), 0o644); err != nil {
		return nil, err
	}
	return []string{"wrote skill to " + path}, nil
}

// Uninstall removes everything Install and InstallSkill added for target.
func Uninstall(target string, d Deps) ([]string, error) {
	var steps []string
	add := func(removed bool, what string) {
		if removed {
			steps = append(steps, "removed "+what)
		} else {
			steps = append(steps, "nothing to remove: "+what)
		}
	}
	block := func(path, begin, end, what string) error {
		ok, err := removeBlock(path, begin, end)
		add(ok, what+" in "+path)
		return err
	}
	switch target {
	case "claude":
		removed := false
		if d.HasCommand == nil || d.HasCommand("claude") {
			// A failure (e.g. "not found") means there was nothing to remove.
			removed = d.Run("claude", "mcp", "remove", "--scope", "user", "glim") == nil
		}
		add(removed, "glim MCP server registration (claude mcp remove)")
		if err := block(filepath.Join(d.Home, ".claude", "CLAUDE.md"), blockBeginMD, blockEndMD, "steering rule"); err != nil {
			return steps, err
		}
	case "codex":
		if err := block(filepath.Join(d.Home, ".codex", "config.toml"), blockBeginTOML, blockEndTOML, "MCP server block"); err != nil {
			return steps, err
		}
		if err := block(filepath.Join(d.Home, ".codex", "AGENTS.md"), blockBeginMD, blockEndMD, "steering rule"); err != nil {
			return steps, err
		}
	case "cursor":
		mcpPath := filepath.Join(d.Home, ".cursor", "mcp.json")
		ok, foreignMCP, err := removeCursorMCP(mcpPath, "glim")
		if foreignMCP {
			steps = append(steps, "left the glim entry in "+mcpPath+" in place: it is not an entry glim wrote")
		} else {
			add(ok, "glim MCP server in "+mcpPath)
		}
		if err != nil {
			return steps, err
		}
		rule := filepath.Join(d.Home, ".cursor", "rules", "glim.mdc")
		var owned []string
		for _, desc := range append([]string{cursorRuleDescription}, legacyCursorRuleDescriptions...) {
			for _, r := range append([]string{SteeringRule}, legacySteeringRules...) {
				owned = append(owned, cursorRuleBody(desc, r))
			}
		}
		ok, foreignRule, err := removeOwnedFile(rule, owned)
		if foreignRule {
			steps = append(steps, "left "+rule+" in place: it is not a rule glim wrote")
		} else {
			add(ok, "steering rule "+rule)
		}
		if err != nil {
			return steps, err
		}
	default:
		return nil, fmt.Errorf("unknown target %q (want claude|codex|cursor)", target)
	}
	dir, _ := skillDir(target, d)
	skill := filepath.Join(dir, "SKILL.md")
	ok, foreign, err := removeOwnedFile(skill, append([]string{SkillText}, legacySkillTexts...))
	if err == nil && ok {
		// Only prune a real, empty directory; never unlink a symlinked skill dir.
		if fi, lerr := os.Lstat(dir); lerr == nil && fi.IsDir() {
			os.Remove(dir) // only succeeds when empty; never touches foreign files
		}
	}
	if foreign {
		steps = append(steps, "left "+skill+" in place: it is not a skill glim wrote")
	} else {
		add(ok, "skill in "+dir)
	}
	return steps, err
}

// removeOwnedFile deletes path only when its whole content equals one of the
// texts glim has written there (ignoring CRLF line endings). A file with any
// other content is the user's: it is left alone and foreign is true.
func removeOwnedFile(path string, owned []string) (removed, foreign bool, err error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	got := strings.ReplaceAll(string(data), "\r\n", "\n")
	for _, o := range owned {
		if got == o {
			removed, err = removeFile(path)
			return removed, false, err
		}
	}
	return false, true, nil
}

// isGlimCursorEntry reports whether a mcpServers entry has the shape glim
// registers: exactly {command, args: ["mcp"]}.
func isGlimCursorEntry(m map[string]any) bool {
	if len(m) != 2 {
		return false
	}
	if _, ok := m["command"].(string); !ok {
		return false
	}
	args, _ := m["args"].([]any)
	return len(args) == 1 && args[0] == "mcp"
}

func removeFile(path string) (bool, error) {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// removeCursorMCP removes the glim server entry, but only when it has the
// shape glim registers; any other entry named glim is the user's own, is left
// in place and reported through foreign.
func removeCursorMCP(path, name string) (removed, foreign bool, err error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) || (err == nil && len(data) == 0) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	root := map[string]any{}
	if err := json.Unmarshal(data, &root); err != nil {
		return false, false, fmt.Errorf("existing %s is not valid JSON: %w", path, err)
	}
	servers, _ := root["mcpServers"].(map[string]any)
	entry, ok := servers[name]
	if !ok {
		return false, false, nil
	}
	if m, _ := entry.(map[string]any); !isGlimCursorEntry(m) {
		return false, true, nil
	}
	delete(servers, name)
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, false, err
	}
	return true, false, os.WriteFile(path, append(out, '\n'), 0o644)
}
