package main

import (
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/samuelloranger/glim/internal/config"
	"github.com/samuelloranger/glim/internal/listen"
)

func cmdConfig(args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	domain := fs.String("domain", "", "public base URL, e.g. https://glim.example.com")
	bind := fs.String("bind", "", "address the server binds, e.g. 127.0.0.1 or 0.0.0.0")
	port := fs.Int("port", 0, "port the server binds")
	root := fs.String("root", "", "directory previews are stored/served from")
	ttl := fs.String("ttl", "", "default time to live, e.g. 6h")
	liveReload := fs.String("live-reload", "", "refresh open tabs when a preview is republished (true or false)")
	header := fs.String("header", "", "show the owner-only glim bar on preview pages (true or false)")
	selfFetch := fs.String("self-fetch", "", "let preview pages fetch their own files (true or false)")
	llmURL := fs.String("llm-url", "", "Listen: base URL of an OpenAI-compatible API that describes pages (key from GLIM_LLM_API_KEY)")
	llmModel := fs.String("llm-model", "", "Listen: vision model that describes pages")
	llmMax := fs.Int("llm-max-tokens", 0, "Listen: the model's output limit, which sets the narration's length (default 4096)")
	ttsURL := fs.String("tts-url", "", "Listen: base URL of an OpenAI-compatible speech API (key from GLIM_TTS_API_KEY)")
	ttsModel := fs.String("tts-model", "", "Listen: speech model")
	ttsVoice := fs.String("tts-voice", "", "Listen: default voice")
	ttsVoices := fs.String("tts-voices", "", "Listen: voice per language, e.g. fr=voice1,en=voice2")
	chrome := fs.String("chrome", "", "browser for Listen and dashboard screenshots (path or name), or off")
	instructions := fs.String("listen-instructions", "", "Listen: extra instructions for the narration")
	if err := fs.Parse(args); err != nil {
		return err
	}
	changed := false
	fs.Visit(func(f *flag.Flag) { changed = true })
	if !changed {
		fmt.Printf("path:   %s\n", config.Path())
		fmt.Printf("domain: %s\n", cfg.Domain)
		fmt.Printf("base:   %s\n", cfg.BaseURL())
		fmt.Printf("bind:   %s\n", cfg.Bind)
		fmt.Printf("port:   %d\n", cfg.Port)
		fmt.Printf("root:   %s\n", cfg.Root)
		fmt.Printf("ttl:    %s\n", cfg.TTL)
		fmt.Printf("live_reload: %t\n", cfg.LiveReloadEnabled())
		fmt.Printf("self_fetch:  %t\n", cfg.SelfFetchEnabled())
		fmt.Printf("header:      %t\n", cfg.HeaderEnabled())
		printListen(cfg.Listen)
		for _, w := range cfg.Warnings {
			fmt.Println("warning:", w)
		}
		return nil
	}
	if *domain != "" {
		cfg.Domain = *domain
	}
	if *bind != "" {
		cfg.Bind = *bind
	}
	if *port != 0 {
		cfg.Port = *port
	}
	if *root != "" {
		cfg.Root = *root
	}
	if *ttl != "" {
		cfg.TTL = *ttl
	}
	if *liveReload != "" {
		b, err := strconv.ParseBool(*liveReload)
		if err != nil {
			return fmt.Errorf("--live-reload: want true or false, got %q", *liveReload)
		}
		cfg.LiveReload = &b
	}
	if *selfFetch != "" {
		b, err := strconv.ParseBool(*selfFetch)
		if err != nil {
			return fmt.Errorf("--self-fetch: want true or false, got %q", *selfFetch)
		}
		cfg.SelfFetch = &b
	}
	if *header != "" {
		b, err := strconv.ParseBool(*header)
		if err != nil {
			return fmt.Errorf("--header: want true or false, got %q", *header)
		}
		cfg.Header = &b
	}
	for flag, dst := range map[*string]*string{
		llmURL: &cfg.Listen.LLMURL, llmModel: &cfg.Listen.LLMModel,
		ttsURL: &cfg.Listen.TTSURL, ttsModel: &cfg.Listen.TTSModel, ttsVoice: &cfg.Listen.TTSVoice,
		chrome: &cfg.Listen.Chrome, instructions: &cfg.Listen.Instructions,
	} {
		if *flag != "" {
			*dst = strings.TrimSpace(*flag)
		}
	}
	if *llmMax > 0 {
		cfg.Listen.LLMMaxTokens = *llmMax
	}
	if *ttsVoices != "" {
		voices, err := parseVoices(*ttsVoices)
		if err != nil {
			return err
		}
		cfg.Listen.TTSVoices = voices
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Println("saved", config.Path())
	return nil
}

func printListen(l config.Listen) {
	if !l.Enabled() {
		fmt.Println("listen:      off (set --llm-url and --llm-model)")
		return
	}
	set := func(s string) string {
		if s == "" {
			return "unset"
		}
		return "set"
	}
	fmt.Printf("listen:      on\n")
	fmt.Printf("  llm:       %s at %s (GLIM_LLM_API_KEY %s)\n", l.LLMModel, listen.Redact(l.LLMURL), set(l.LLMKey))
	maxTok := l.LLMMaxTokens
	if maxTok <= 0 {
		maxTok = listen.DefaultMaxTokens
	}
	fmt.Printf("  output:    up to %d tokens\n", maxTok)
	if l.TTSURL == "" {
		fmt.Println("  tts:       none, clients use the device's voice")
	} else {
		fmt.Printf("  tts:       %s at %s (GLIM_TTS_API_KEY %s)\n", orDefault(l.TTSModel, "default model"), listen.Redact(l.TTSURL), set(l.TTSKey))
		fmt.Printf("  voice:     %s\n", orDefault(l.TTSVoice, "provider default"))
		langs := make([]string, 0, len(l.TTSVoices))
		for k := range l.TTSVoices {
			langs = append(langs, k)
		}
		sort.Strings(langs)
		for _, k := range langs {
			fmt.Printf("  voice %s:  %s\n", k, l.TTSVoices[k])
		}
	}
	chrome := listen.DescribeChrome(listen.FindChrome(l.Chrome))
	if chrome == "" {
		chrome = "none (pages are read from their HTML)"
	}
	fmt.Printf("  browser:   %s\n", chrome)
}

// parseVoices reads "fr=voice1,en=voice2"; "-" clears the map.
func parseVoices(s string) (map[string]string, error) {
	if strings.TrimSpace(s) == "-" {
		return nil, nil
	}
	out := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		lang, voice, ok := strings.Cut(strings.TrimSpace(pair), "=")
		lang, voice = strings.ToLower(strings.TrimSpace(lang)), strings.TrimSpace(voice)
		if !ok || lang == "" || voice == "" {
			return nil, fmt.Errorf("--tts-voices: want lang=voice pairs like fr=voice1,en=voice2, got %q", pair)
		}
		out[lang] = voice
	}
	return out, nil
}
