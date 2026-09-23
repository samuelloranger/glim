package config

import (
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
}

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
		}
	}
	if v := firstEnv("GLIM_DOMAIN", "GLIM_BASE_URL"); v != "" {
		c.Domain = v
	}
	if v := os.Getenv("GLIM_BIND"); v != "" {
		c.Bind = v
	}
	if v := os.Getenv("GLIM_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Port = p
		}
	}
	if v := os.Getenv("GLIM_ROOT"); v != "" {
		c.Root = v
	}
	if v := os.Getenv("GLIM_TTL"); v != "" {
		c.TTL = v
	}
	return c
}

func (c Config) BaseURL() string {
	if c.Domain != "" {
		return strings.TrimRight(c.Domain, "/")
	}
	return fmt.Sprintf("http://127.0.0.1:%d", c.Port)
}

func (c Config) TTLDuration() time.Duration {
	if d, err := time.ParseDuration(c.TTL); err == nil {
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

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func DBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".glim", "glim.db")
}
