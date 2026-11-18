package main

import (
	"fmt"
	"time"

	"github.com/samuelloranger/glim/internal/config"
	"github.com/samuelloranger/glim/internal/serve"
	"github.com/samuelloranger/glim/internal/store"
)

func cmdStatus() error {
	cfg := config.Load()
	s := store.New(cfg.Root, cfg.BaseURL())
	list, err := s.List()
	if err != nil {
		return err
	}
	bytes, err := s.DiskUsage()
	if err != nil {
		return err
	}
	pinned := 0
	var next time.Time
	for _, m := range list {
		if m.Pinned {
			pinned++
			continue
		}
		if next.IsZero() || m.Expires.Before(next) {
			next = m.Expires
		}
	}
	fmt.Printf("root:     %s\n", cfg.Root)
	state, haveState := serve.ReadState()
	running := haveState && pidAlive(state.PID) && serve.Probe(state)
	fmt.Printf("server:   %s\n", serverLine(state, running))
	if running {
		if note := serve.HTTPSNote(cfg.Domain); note != "" {
			fmt.Printf("note:     %s\n", note)
		}
	}
	fmt.Printf("live:    %d preview(s) (%d pinned)\n", len(list), pinned)
	fmt.Printf("disk:     %s\n", humanBytes(bytes))
	if next.IsZero() {
		fmt.Println("next gc:  none pending")
	} else {
		fmt.Printf("next gc:  %s (in %s)\n", next.Format(time.RFC1123), short(time.Until(next)))
	}
	return nil
}

// serverLine describes the running server from its serve.json state.
func serverLine(st serve.State, running bool) string {
	if !running {
		return "not running"
	}
	return fmt.Sprintf("running on port %d (pid %d)", st.Port, st.PID)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
