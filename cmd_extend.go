package main

import (
	"fmt"
	"time"

	"github.com/samuelloranger/glim/internal/config"
	"github.com/samuelloranger/glim/internal/store"
)

func cmdExtend(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: glim extend <name> <ttl>")
	}
	ttl, err := store.ParseTTL(args[1])
	if err != nil {
		return err
	}
	cfg := config.Load()
	s := store.New(cfg.Root, cfg.BaseURL())
	if err := s.Extend(args[0], ttl); err != nil {
		return err
	}
	m, err := s.Get(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("%s now expires %s\n", args[0], m.Expires.Format(time.RFC1123))
	return nil
}

func cmdPin(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: glim pin <name>")
	}
	cfg := config.Load()
	s := store.New(cfg.Root, cfg.BaseURL())
	if err := s.Pin(args[0]); err != nil {
		return err
	}
	fmt.Printf("pinned %s (never expires)\n", args[0])
	return nil
}

func cmdUnpin(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: glim unpin <name>")
	}
	cfg := config.Load()
	s := store.New(cfg.Root, cfg.BaseURL())
	if err := s.Unpin(args[0], cfg.TTLDuration()); err != nil {
		return err
	}
	m, err := s.Get(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("unpinned %s, now expires %s\n", args[0], m.Expires.Format(time.RFC1123))
	return nil
}
