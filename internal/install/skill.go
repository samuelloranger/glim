package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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
		ok, err := removeCursorMCP(mcpPath, "glim")
		add(ok, "glim MCP server in "+mcpPath)
		if err != nil {
			return steps, err
		}
		rule := filepath.Join(d.Home, ".cursor", "rules", "glim.mdc")
		ok, err = removeFile(rule)
		add(ok, "steering rule "+rule)
		if err != nil {
			return steps, err
		}
	default:
		return nil, fmt.Errorf("unknown target %q (want claude|codex|cursor)", target)
	}
	dir, _ := skillDir(target, d)
	ok, err := removeFile(filepath.Join(dir, "SKILL.md"))
	if err == nil && ok {
		// Only prune a real, empty directory; never unlink a symlinked skill dir.
		if fi, lerr := os.Lstat(dir); lerr == nil && fi.IsDir() {
			os.Remove(dir) // only succeeds when empty; never touches foreign files
		}
	}
	add(ok, "skill in "+dir)
	return steps, err
}

func removeFile(path string) (bool, error) {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func removeCursorMCP(path, name string) (bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) || (err == nil && len(data) == 0) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	root := map[string]any{}
	if err := json.Unmarshal(data, &root); err != nil {
		return false, fmt.Errorf("existing %s is not valid JSON: %w", path, err)
	}
	servers, _ := root["mcpServers"].(map[string]any)
	if _, ok := servers[name]; !ok {
		return false, nil
	}
	delete(servers, name)
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, append(out, '\n'), 0o644)
}
