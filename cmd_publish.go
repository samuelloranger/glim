package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mdp/qrterminal/v3"
	"github.com/samuelloranger/glim/internal/config"
	"github.com/samuelloranger/glim/internal/mcpserver"
	"github.com/samuelloranger/glim/internal/store"
	"golang.org/x/term"
)

func cmdPublish(args []string) error {
	entry, flagArgs := splitEntry(args)
	cfg := config.Load()
	fs, o := newPublishFlags(cfg.TTLDuration())
	title, project, ttl, local, qr, name := o.title, o.project, o.ttl, o.local, o.qr, o.name
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if err := store.ValidateTTL(*ttl); err != nil {
		return err
	}
	if entry == "" || fs.NArg() != 0 {
		return fmt.Errorf("usage: glim <entry.html|dir|-> [--format html|md|txt|json] [--title T] [--project P] [--ttl 6h] [--name SLUG] [--password] [--no-header] [--local] [--qr] [--json]")
	}
	if *o.format != "" && entry != "-" {
		return fmt.Errorf("--format only applies when reading the entry from stdin (glim -)")
	}
	if entry == "-" && *o.password {
		return fmt.Errorf("--password cannot be combined with reading the entry from stdin")
	}
	if entry == "-" {
		stored := ""
		if *name != "" {
			if m, err := store.New(cfg.Root, cfg.BaseURL()).Get(*name); err == nil {
				stored = m.Title
			}
		}
		if term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Fprintln(os.Stderr, "reading the page from stdin; end with Ctrl-D")
		}
		staged, t, cleanup, err := stageStdin(os.Stdin, *o.format, *title, stored)
		defer cleanup()
		if err != nil {
			return err
		}
		entry, *title = staged, t
	}
	passwordHash := ""
	if *o.password {
		h, err := promptNewPassword(stdin)
		if err != nil {
			return err
		}
		passwordHash = h
	}
	base := cfg.BaseURL()
	if *local {
		b, err := ensureLocalServer(cfg)
		if err != nil {
			return err
		}
		base = b
	}
	s := store.New(cfg.Root, base)
	s.OnRemove = forgetViews
	res, err := s.PublishFull(entry, *title, *project, os.Getenv("GLIM_SESSION_ID"), *ttl, *name, passwordHash, *o.noHeader)
	if err != nil {
		return err
	}
	if *o.json {
		// --json wins over --qr: stdout carries only the JSON object.
		return writeJSON(os.Stdout, mcpserver.NewPresentOutput(res))
	}
	fmt.Println(res.URL)
	fmt.Fprintf(os.Stderr, "expires %s\n", res.Expires.Format(time.RFC1123))
	if *qr {
		writeQR(os.Stderr, res.URL)
	}
	return nil
}

func writeQR(w io.Writer, url string) {
	qrterminal.GenerateHalfBlock(url, qrterminal.L, w)
}

type publishFlags struct {
	title, project, name *string
	format               *string
	ttl                  *time.Duration
	local, qr, password  *bool
	json                 *bool
	noHeader             *bool
}

// newPublishFlags defines every flag of the publish command. It is the single
// source of truth: splitEntry derives which flags take a value from it.
func newPublishFlags(defaultTTL time.Duration) (*flag.FlagSet, publishFlags) {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	var o publishFlags
	o.title = fs.String("title", "", "human title (becomes the readable slug)")
	o.project = fs.String("project", "", "project name")
	o.ttl = fs.Duration("ttl", defaultTTL, "time to live, e.g. 6h, 30m")
	o.local = fs.Bool("local", false, "auto-start the built-in server and use a localhost link")
	o.qr = fs.Bool("qr", false, "also print a scannable QR code of the URL")
	o.password = fs.Bool("password", false, "protect the preview with a password, read from a no-echo prompt (or one line of stdin when piped)")
	o.noHeader = fs.Bool("no-header", false, "hide the owner-only glim bar on this preview (not remembered on republish)")
	o.json = fs.Bool("json", false, "print one JSON object (url, name, expires, locked) instead of the plain URL; suppresses --qr")
	o.name = fs.String("name", "", "reuse this exact slug to update in place at the same URL (created if absent); omit for a fresh random link")
	o.format = fs.String("format", "", "with entry -: how to read stdin, html (default), md, txt or json")
	return fs, o
}

// stageStdin reads the entry from r (capped at store.MaxInlineBytes) into a
// private temp dir, as a file named for format. title falls back to one taken
// from the content, then stored (the title of the preview being republished).
// cleanup is always non-nil and removes the temp dir.
func stageStdin(r io.Reader, format, title, stored string) (entry, newTitle string, cleanup func(), err error) {
	noop := func() {}
	if format, err = store.NormalizeInlineFormat(format); err != nil {
		return "", "", noop, err
	}
	data, err := io.ReadAll(io.LimitReader(r, store.MaxInlineBytes+1))
	if err != nil {
		return "", "", noop, fmt.Errorf("reading stdin: %w", err)
	}
	if len(data) == 0 {
		return "", "", noop, fmt.Errorf("stdin is empty: nothing to publish")
	}
	if title == "" {
		title = store.InlineTitle(string(data), format, stored)
	}
	entry, cleanup, err = store.StageInline(string(data), format, title)
	return entry, title, cleanup, err
}

// valueFlagNames returns the flags of fs that consume a following argument,
// i.e. every flag whose Value is not a bool flag.
func valueFlagNames(fs *flag.FlagSet) map[string]bool {
	m := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) {
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			return
		}
		m[f.Name] = true
	})
	return m
}

// splitEntry separates the entry path from the publish flags so flags may come
// before or after it, as `--flag value` or `--flag=value`.
func splitEntry(args []string) (entry string, flagArgs []string) {
	fs, _ := newPublishFlags(0)
	valueFlags := valueFlagNames(fs)
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && a != "-" {
			flagArgs = append(flagArgs, a)
			name := strings.TrimLeft(a, "-")
			if strings.Contains(name, "=") {
				continue
			}
			if valueFlags[name] && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		if entry == "" {
			entry = a
		} else {
			flagArgs = append(flagArgs, a)
		}
	}
	return entry, flagArgs
}
