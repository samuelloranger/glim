package config

import (
	"github.com/samuelloranger/glim/internal/store"

	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultPort = 8787
	DefaultTTL  = "6h"
)

type Config struct {
	Domain string `json:"domain,omitempty"`
	Bind   string `json:"bind,omitempty"`
	Port   int    `json:"port,omitempty"`
	Root   string `json:"root,omitempty"`
	TTL    string `json:"ttl,omitempty"`
	// LiveReload makes open preview tabs refresh when the preview is
	// republished under the same name. Nil means the default (on).
	LiveReload *bool `json:"live_reload,omitempty"`
	// SelfFetch lets a preview page fetch its own files by answering
	// Origin: null with CORS headers. Nil means the default (on).
	SelfFetch *bool `json:"self_fetch,omitempty"`
	// Header shows the owner-only glim bar on preview pages opened by the
	// signed-in dashboard owner. Nil means the default (on).
	Header *bool `json:"header,omitempty"`
	// Listen configures the audio version of a preview: a vision model
	// describes the page and a speech endpoint reads the description aloud.
	Listen Listen `json:"listen,omitzero"`
	// Warnings are problems found while loading, such as an env file readable
	// by other users. Never saved.
	Warnings []string `json:"-"`
}

// Listen points at OpenAI-compatible endpoints, so any provider (hosted or
// local) works. API keys are never stored here: they come from the
// GLIM_LLM_API_KEY and GLIM_TTS_API_KEY environment variables.
type Listen struct {
	// LLMURL is the base of an OpenAI-compatible API, e.g.
	// https://api.example.com/v1; glim calls LLMURL/chat/completions.
	LLMURL   string `json:"llm_url,omitempty"`
	LLMModel string `json:"llm_model,omitempty"`
	// LLMMaxTokens caps the model's output, and so the narration's length;
	// 0 selects listen.DefaultMaxTokens. Keep it within the model's limit.
	LLMMaxTokens int `json:"llm_max_tokens,omitempty"`
	// LLMKey is filled from GLIM_LLM_API_KEY and never saved.
	LLMKey string `json:"-"`
	// TTSURL is the base of an OpenAI-compatible speech API; glim calls
	// TTSURL/audio/speech. Unset means no audio: the dashboard and the bar
	// offer the device's own voice instead.
	TTSURL   string `json:"tts_url,omitempty"`
	TTSModel string `json:"tts_model,omitempty"`
	// TTSVoice is the default voice; TTSVoices maps a language subtag (the
	// part of the page's language before any "-", such as "fr") to a voice.
	TTSVoice  string            `json:"tts_voice,omitempty"`
	TTSVoices map[string]string `json:"tts_voices,omitempty"`
	// TTSKey is filled from GLIM_TTS_API_KEY and never saved.
	TTSKey string `json:"-"`
	// Chrome is the browser used to screenshot the page, for Listen and for
	// the dashboard's thumbnails; empty finds one on PATH, "off" never uses
	// one (Listen reads text and image files only, and the dashboard frames
	// the previews).
	Chrome string `json:"chrome,omitempty"`
	// Instructions are appended to the prompt, e.g. "Keep it under 5 minutes."
	Instructions string `json:"instructions,omitempty"`
}

// Enabled reports whether a model is configured to describe pages.
func (l Listen) Enabled() bool { return l.LLMURL != "" && l.LLMModel != "" }

// LiveReloadEnabled reports whether live reload is on (the default).
func (c Config) LiveReloadEnabled() bool { return c.LiveReload == nil || *c.LiveReload }

// SelfFetchEnabled reports whether previews may fetch their own files (the default).
func (c Config) SelfFetchEnabled() bool { return c.SelfFetch == nil || *c.SelfFetch }

// HeaderEnabled reports whether the owner-only preview bar is on (the default).
func (c Config) HeaderEnabled() bool { return c.Header == nil || *c.Header }

func Path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".glim", "config.json")
}

func defaults() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Bind: "127.0.0.1",
		Port: DefaultPort,
		Root: filepath.Join(home, ".glim", "pub"),
		TTL:  DefaultTTL,
	}
}

func Load() Config {
	c := defaults()
	env, warnings := loadEnvFile(EnvPath())
	c.Warnings = warnings
	if data, err := os.ReadFile(Path()); err == nil {
		var f Config
		if json.Unmarshal(data, &f) == nil {
			if f.Domain != "" {
				c.Domain = f.Domain
			}
			if f.Bind != "" {
				c.Bind = f.Bind
			}
			if f.Port != 0 {
				c.Port = f.Port
			}
			if f.Root != "" {
				c.Root = f.Root
			}
			if f.TTL != "" {
				c.TTL = f.TTL
			}
			if f.LiveReload != nil {
				c.LiveReload = f.LiveReload
			}
			if f.SelfFetch != nil {
				c.SelfFetch = f.SelfFetch
			}
			if f.Header != nil {
				c.Header = f.Header
			}
			c.Listen = f.Listen
		}
	}
	if v := env.first("GLIM_DOMAIN", "GLIM_BASE_URL"); v != "" {
		c.Domain = v
	}
	if v := env.get("GLIM_BIND"); v != "" {
		c.Bind = v
	}
	if v := env.get("GLIM_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Port = p
		}
	}
	if v := env.get("GLIM_ROOT"); v != "" {
		c.Root = v
	}
	if v := env.get("GLIM_TTL"); v != "" {
		c.TTL = v
	}
	if v := env.get("GLIM_LIVE_RELOAD"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.LiveReload = &b
		}
	}
	if v := env.get("GLIM_SELF_FETCH"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.SelfFetch = &b
		}
	}
	if v := env.get("GLIM_HEADER"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.Header = &b
		}
	}
	listenEnv(env, &c.Listen)
	return c
}

// listenEnv applies the GLIM_LLM_* / GLIM_TTS_* / GLIM_CHROME overrides.
func listenEnv(env envLayer, l *Listen) {
	for name, dst := range map[string]*string{
		"GLIM_LLM_URL":             &l.LLMURL,
		"GLIM_LLM_MODEL":           &l.LLMModel,
		"GLIM_LLM_API_KEY":         &l.LLMKey,
		"GLIM_TTS_URL":             &l.TTSURL,
		"GLIM_TTS_MODEL":           &l.TTSModel,
		"GLIM_TTS_VOICE":           &l.TTSVoice,
		"GLIM_TTS_API_KEY":         &l.TTSKey,
		"GLIM_CHROME":              &l.Chrome,
		"GLIM_LISTEN_INSTRUCTIONS": &l.Instructions,
	} {
		if v := env.get(name); v != "" {
			*dst = v
		}
	}
	if v := env.get("GLIM_LLM_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			l.LLMMaxTokens = n
		}
	}
}

func (c Config) BaseURL() string {
	if c.Domain != "" {
		return strings.TrimRight(c.Domain, "/")
	}
	return fmt.Sprintf("http://127.0.0.1:%d", c.Port)
}

func (c Config) TTLDuration() time.Duration {
	if d, err := store.ParseTTL(c.TTL); err == nil {
		return d
	}
	return 6 * time.Hour
}

func (c Config) Save() error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(data, '\n'), 0o644)
}

func DBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".glim", "glim.db")
}
