package main

import (
	"context"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/samuelloranger/glim/internal/api"
	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/config"
	"github.com/samuelloranger/glim/internal/listen"
	"github.com/samuelloranger/glim/internal/serve"
	"github.com/samuelloranger/glim/internal/store"
	"github.com/samuelloranger/glim/internal/thumb"
	"github.com/samuelloranger/glim/internal/web"
)

func cmdServe(args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	bind := fs.String("bind", cfg.Bind, "address to bind (0.0.0.0 to accept a reverse proxy)")
	port := fs.Int("port", cfg.Port, "port to bind (0 = pick a free one)")
	root := fs.String("root", cfg.Root, "directory of previews to serve")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx := context.Background()
	base := ""
	if cfg.Domain != "" {
		base = cfg.BaseURL()
	}
	st := store.New(*root, base)

	db, err := auth.Open(config.DBPath())
	if err != nil {
		return err
	}
	defer db.Close()
	users, err := db.CountUsers(ctx)
	if err != nil {
		return err
	}
	for _, w := range cfg.Warnings {
		log.Printf("warning: %s", w)
	}
	shownBase := base
	if shownBase == "" {
		shownBase = serve.BaseURL(*port)
	}
	if note := serve.HTTPSNote(base); note != "" {
		log.Printf("note: %s", note)
	}
	if users == 0 {
		log.Printf("no account yet — open %s/ to create the first one", shownBase)
	}

	st.OnRemove = func(name string) {
		if err := db.DeleteViews(ctx, name); err != nil {
			log.Printf("views: forget %s: %v", name, err)
		}
		if err := db.DeleteComments(ctx, name); err != nil {
			log.Printf("comments: forget %s: %v", name, err)
		}
	}
	hub := api.NewHub(st, db, 2*time.Second, log.Printf)
	if _, _, err := db.VAPIDKeys(ctx); err != nil {
		return err
	}
	notifier := api.NewNotifier(db, &api.WebPushSender{DB: db, Subscriber: pushSubscriber(base)}, log.Printf)
	go notifier.Run(ctx)
	hub.SetNotifier(notifier)
	go hub.Run(ctx)
	recorder := db.NewViewRecorder(hub.Poke)
	defer recorder.Close()
	lst := newListen(cfg.Listen, st)
	thumbs := newThumbs(cfg.Listen.Chrome, st)
	defer thumbs.Close()
	apiSrv := api.New(api.Deps{
		Store: st, Auth: db, Limiter: auth.NewLimiter(nil), Hub: hub, Listen: lst, Thumbs: thumbs,
		SecureCookies: strings.HasPrefix(base, "https://"),
		PublicHost:    publicHost(base),
		Logf:          log.Printf,
		DefaultTTL:    cfg.TTLDuration(),
	})
	return serve.Serve(ctx, serve.Options{Bind: *bind, Port: *port, Store: st, API: apiSrv, Web: web.Handler(),
		LiveReload:  cfg.LiveReloadEnabled(),
		NoSelfFetch: !cfg.SelfFetchEnabled(),
		NoHeader:    !cfg.HeaderEnabled(),
		Listen:      lst.Enabled(),
		Views:       &serve.Views{Record: recorder.Record, IsOwner: db.IsOwnerToken, BarToken: db.BarToken},
		Unlock: &serve.Unlock{Token: db.UnlockToken, Limiter: auth.NewLimiter(nil),
			Secure: strings.HasPrefix(base, "https://")}})
}

// newListen builds the audio-version service from the config and logs what
// it will use (never the keys).
func newListen(l config.Listen, st *store.Store) *listen.Service {
	set := listen.Settings{
		LLM:          listen.Endpoint{URL: l.LLMURL, Model: l.LLMModel, Key: l.LLMKey},
		TTS:          listen.Endpoint{URL: l.TTSURL, Model: l.TTSModel, Key: l.TTSKey},
		Voice:        l.TTSVoice,
		Voices:       l.TTSVoices,
		Instructions: l.Instructions,
		MaxTokens:    l.LLMMaxTokens,
	}
	if l.Enabled() {
		set.Chrome = listen.FindChrome(l.Chrome)
		log.Printf("listen: describing previews with %s at %s", l.LLMModel, listen.Redact(l.LLMURL))
		if l.TTSURL != "" {
			log.Printf("listen: speech from %s at %s", orDefault(l.TTSModel, "the default model"), listen.Redact(l.TTSURL))
		} else {
			log.Printf("listen: no tts_url, so clients read the narration with the device's voice")
		}
		if set.Chrome != "" {
			log.Printf("listen: screenshots with %s", listen.DescribeChrome(set.Chrome))
		} else {
			log.Printf("listen: no browser, so pages are read from their HTML and image files")
		}
	}
	return listen.New(st, set, log.Printf)
}

// newThumbs builds the card screenshot service. It shares Listen's browser
// setting, so "off" turns both off.
func newThumbs(chrome string, st *store.Store) *thumb.Service {
	path := listen.FindChrome(chrome)
	if path != "" {
		log.Printf("thumbnails: screenshots with %s", listen.DescribeChrome(path))
	} else {
		log.Printf("thumbnails: no browser, so the dashboard frames previews instead")
	}
	return thumb.New(st, path, log.Printf)
}
