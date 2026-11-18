package main

import (
	"fmt"

	"github.com/samuelloranger/glim/internal/caddy"
	"github.com/samuelloranger/glim/internal/config"
)

func cmdCaddy() error {
	cfg := config.Load()
	if cfg.Domain == "" {
		return fmt.Errorf("set a domain first: glim config --domain https://glim.example.com")
	}
	upstream := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	fmt.Print(caddy.Snippet(hostFromBase(cfg.BaseURL()), upstream))
	return nil
}
