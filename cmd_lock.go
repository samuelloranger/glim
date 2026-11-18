package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/config"
	"github.com/samuelloranger/glim/internal/store"
	"golang.org/x/term"
)

func cmdLock(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: glim lock <name>")
	}
	cfg := config.Load()
	return runLock(store.New(cfg.Root, cfg.BaseURL()), args[0], stdin, os.Stdout)
}

func cmdUnlock(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: glim unlock <name>")
	}
	cfg := config.Load()
	return runUnlock(store.New(cfg.Root, cfg.BaseURL()), args[0], os.Stdout)
}

// runLock sets (or replaces) the password of an existing preview.
func runLock(s *store.Store, name string, in *bufio.Reader, out io.Writer) error {
	if _, err := s.Get(name); err != nil {
		return err
	}
	hash, err := promptNewPassword(in)
	if err != nil {
		return err
	}
	if err := s.SetPasswordHash(name, hash); err != nil {
		return err
	}
	fmt.Fprintf(out, "locked %s (visitors need the password)\n", name)
	return nil
}

// runUnlock removes a preview's password.
func runUnlock(s *store.Store, name string, out io.Writer) error {
	if _, err := s.Get(name); err != nil {
		return err
	}
	if err := s.SetPasswordHash(name, ""); err != nil {
		return err
	}
	fmt.Fprintf(out, "unlocked %s (no password required)\n", name)
	return nil
}

// promptNewPassword reads a preview password (twice on a terminal, to catch
// typos), validates it and returns its bcrypt hash. The password is only ever
// read from the prompt or stdin, never from a flag or the environment.
func promptNewPassword(in *bufio.Reader) (string, error) {
	p1, err := readPassword(in, "Preview password: ")
	if err != nil {
		return "", err
	}
	if in == stdin && term.IsTerminal(int(os.Stdin.Fd())) {
		p2, err := readPassword(in, "Repeat password: ")
		if err != nil {
			return "", err
		}
		if p1 != p2 {
			return "", fmt.Errorf("passwords do not match")
		}
	}
	h, err := auth.HashPassword(p1)
	if err != nil {
		return "", passwordError(err)
	}
	return h, nil
}
