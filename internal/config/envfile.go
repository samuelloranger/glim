package config

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// EnvPath is the glim-owned env file: KEY=VALUE lines for GLIM_* settings,
// chiefly the Listen API keys, so they reach `glim serve` however it is
// started. glim reads it and never writes it.
func EnvPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".glim", "env")
}

// envLayer answers GLIM_* lookups: the real process environment first, then
// the parsed env file. It is a lookup layer only; nothing is ever passed to
// os.Setenv, so the file's keys do not leak into child processes.
type envLayer struct {
	file map[string]string
}

func (e envLayer) get(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return e.file[key]
}

func (e envLayer) first(keys ...string) string {
	for _, k := range keys {
		if v := e.get(k); v != "" {
			return v
		}
	}
	return ""
}

// loadEnvFile parses the env file at path. A missing or unreadable file gives
// an empty layer. The warnings name keys that were ignored (never values) and
// a file readable by other users.
func loadEnvFile(path string) (envLayer, []string) {
	f, err := os.Open(path)
	if err != nil {
		return envLayer{}, nil
	}
	defer f.Close()
	var warnings []string
	if info, err := f.Stat(); err == nil && info.Mode().Perm()&0o077 != 0 {
		warnings = append(warnings, path+" is readable by others; chmod 600 it")
	}
	vals, ignored := parseEnv(f)
	if len(ignored) > 0 {
		warnings = append(warnings, path+": ignoring keys that do not start with GLIM_: "+strings.Join(ignored, ", "))
	}
	return envLayer{file: vals}, warnings
}

// parseEnv reads KEY=VALUE lines. Blank lines and # comments are skipped, an
// `export ` prefix is accepted, and one pair of surrounding single or double
// quotes is stripped from a value. There is no variable expansion. Only GLIM_
// keys are returned; other keys are reported in ignored.
func parseEnv(r io.Reader) (vals map[string]string, ignored []string) {
	vals = map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export"); ok && rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
			line = strings.TrimSpace(rest)
		}
		key, val, ok := strings.Cut(line, "=")
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if !ok || key == "" {
			continue
		}
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		if !strings.HasPrefix(key, "GLIM_") {
			ignored = append(ignored, key)
			continue
		}
		vals[key] = val
	}
	return vals, ignored
}
