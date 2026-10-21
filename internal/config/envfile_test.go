package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeEnvFile(t *testing.T, home, body string, mode os.FileMode) {
	t.Helper()
	dir := filepath.Join(home, ".glim")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "env")
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
}

func TestParseEnv(t *testing.T) {
	body := strings.Join([]string{
		"# a comment",
		"",
		"GLIM_LLM_API_KEY=plain",
		`GLIM_TTS_API_KEY="double quoted"`,
		"GLIM_LLM_MODEL='single quoted'",
		"export GLIM_TTS_MODEL=exported",
		"  GLIM_TTS_VOICE = spaced  ",
		"GLIM_CHROME=$HOME/chrome",
		"GLIM_TTL=\"unbalanced",
		"no equals sign",
		"PATH=/usr/bin",
		"OTHER=x",
	}, "\n")
	vals, ignored := parseEnv(strings.NewReader(body))
	want := map[string]string{
		"GLIM_LLM_API_KEY": "plain",
		"GLIM_TTS_API_KEY": "double quoted",
		"GLIM_LLM_MODEL":   "single quoted",
		"GLIM_TTS_MODEL":   "exported",
		"GLIM_TTS_VOICE":   "spaced",
		"GLIM_CHROME":      "$HOME/chrome", // no expansion
		"GLIM_TTL":         "\"unbalanced",
	}
	if len(vals) != len(want) {
		t.Fatalf("vals = %v", vals)
	}
	for k, v := range want {
		if vals[k] != v {
			t.Errorf("%s = %q, want %q", k, vals[k], v)
		}
	}
	if strings.Join(ignored, ",") != "PATH,OTHER" {
		t.Errorf("ignored = %v", ignored)
	}
}

func TestEnvFileLayeringAndPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, k := range []string{"GLIM_LLM_API_KEY", "GLIM_TTS_API_KEY", "GLIM_DOMAIN", "GLIM_BASE_URL", "GLIM_PORT"} {
		t.Setenv(k, "")
	}
	if err := os.MkdirAll(filepath.Join(home, ".glim"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), []byte(`{"domain":"https://from-config.example.com","port":9000}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeEnvFile(t, home, "GLIM_LLM_API_KEY=file-llm\nGLIM_TTS_API_KEY=file-tts\nGLIM_DOMAIN=https://from-file.example.com\n", 0o600)
	t.Setenv("GLIM_TTS_API_KEY", "process-tts")

	c := Load()
	if c.Listen.LLMKey != "file-llm" {
		t.Errorf("file key = %q, want it from the env file", c.Listen.LLMKey)
	}
	if c.Listen.TTSKey != "process-tts" {
		t.Errorf("process env should beat the file, got %q", c.Listen.TTSKey)
	}
	if c.Domain != "https://from-file.example.com" {
		t.Errorf("env file should beat config.json, got %q", c.Domain)
	}
	if c.Port != 9000 {
		t.Errorf("config.json value lost: port %d", c.Port)
	}
	if len(c.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", c.Warnings)
	}
	// A lookup layer only: nothing reaches the process environment.
	if v := os.Getenv("GLIM_LLM_API_KEY"); v != "" {
		t.Errorf("env file leaked into the process environment: %q", v)
	}
}

func TestEnvFileWarnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GLIM_TTL", "")

	writeEnvFile(t, home, "GLIM_TTL=1h\nSECRET_TOKEN=abc\n", 0o644)
	c := Load()
	if c.TTL != "1h" {
		t.Errorf("a readable file is still loaded, ttl = %q", c.TTL)
	}
	joined := strings.Join(c.Warnings, "\n")
	if !strings.Contains(joined, "readable by others; chmod 600 it") {
		t.Errorf("missing permission warning: %v", c.Warnings)
	}
	if !strings.Contains(joined, "SECRET_TOKEN") || strings.Contains(joined, "abc") {
		t.Errorf("ignored key should be named, never its value: %v", c.Warnings)
	}

	writeEnvFile(t, home, "GLIM_TTL=2h\n", 0o600)
	if w := Load().Warnings; len(w) != 0 {
		t.Errorf("0600 file should not warn: %v", w)
	}
}

func TestNoEnvFileIsFine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if w := Load().Warnings; len(w) != 0 {
		t.Errorf("warnings = %v", w)
	}
}
