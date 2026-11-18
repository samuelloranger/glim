package main

import (
	"context"

	"github.com/samuelloranger/glim/internal/config"
	"github.com/samuelloranger/glim/internal/mcpserver"
	"github.com/samuelloranger/glim/internal/store"
)

func cmdMCP() error {
	cfg := config.Load()
	s := store.New(cfg.Root, cfg.BaseURL())
	s.OnRemove = forgetViews
	var ensureBase func() (string, error)
	if cfg.Domain == "" {
		// Without a domain the links point at a loopback server that nothing
		// else starts, so bring it up on demand like `glim --local` does.
		ensureBase = func() (string, error) { return ensureLocalServer(cfg) }
	}
	return mcpserver.Run(context.Background(), s, cfg.TTLDuration(), version, viewStatsNow, ensureBase, cfg.SelfFetchEnabled(), dbComments{})
}
