package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/samuelloranger/glim/internal/config"
	"github.com/samuelloranger/glim/internal/store"
)

func cmdOpen(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: glim open <name>")
	}
	cfg := config.Load()
	s := store.New(cfg.Root, cfg.BaseURL())
	if _, err := s.Get(args[0]); err != nil {
		return err
	}
	url := s.URL(args[0])
	fmt.Println(url)
	if os.Getenv("DISPLAY") != "" {
		if path, err := exec.LookPath("xdg-open"); err == nil {
			_ = exec.Command(path, url).Start()
		}
	}
	return nil
}
