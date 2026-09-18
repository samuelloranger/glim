package install

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const SteeringRule = "To show the user any HTML or visual preview, call the glim `present` MCP tool " +
	"and give the user the returned link. Do not use Claude artifacts or other built-in preview mechanisms."

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
	body := "---\ndescription: Prefer glim for HTML previews\nalwaysApply: true\n---\n\n" + SteeringRule + "\n"
	if err := os.MkdirAll(filepath.Dir(rule), 0o755); err != nil {
		return steps, err
	}
	if err := os.WriteFile(rule, []byte(body), 0o644); err != nil {
		return steps, err
	}
	steps = append(steps, "wrote steering rule to "+rule)
	return steps, nil
}

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
	if b := strings.Index(content, begin); b != -1 {
		if e := strings.Index(content, end); e != -1 && e > b {
			content = content[:b] + block + content[e+len(end):]
			return os.WriteFile(path, []byte(content), 0o644)
		}
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
