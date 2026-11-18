package main

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/samuelloranger/glim/internal/config"
	"github.com/samuelloranger/glim/internal/mcpserver"
	"github.com/samuelloranger/glim/internal/store"
)

func cmdList(args []string) error {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print a JSON array of previews")
	project := fs.String("project", "", "only previews with this project")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: glim ls [--project P] [--json]")
	}
	cfg := config.Load()
	s := store.New(cfg.Root, cfg.BaseURL())
	if *asJSON {
		out, err := mcpserver.ListPreviews(s, *project, viewStatsNow())
		if err != nil {
			return err
		}
		return writeJSON(os.Stdout, out.Previews)
	}
	all, err := s.List()
	if err != nil {
		return err
	}
	list := all[:0:0]
	for _, m := range all {
		if *project == "" || m.Project == *project {
			list = append(list, m)
		}
	}
	if len(list) == 0 {
		fmt.Println("no live previews")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTITLE\tAGE\tEXPIRES IN\tVIEWS\tLAST SEEN")
	now := time.Now()
	stats := viewStatsNow()
	for _, m := range list {
		expires := short(m.Expires.Sub(now))
		if m.Pinned {
			expires = "pinned"
		}
		views, seen := 0, "-"
		if v := stats[m.Name]; v.Count > 0 {
			views, seen = int(v.Count), agoShort(now.Sub(v.LastSeen))
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
			m.Name, truncate(m.Title, 30),
			short(now.Sub(m.Created)), expires, views, seen)
	}
	return w.Flush()
}

func cmdRemove(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: glim rm <name>...")
	}
	cfg := config.Load()
	s := store.New(cfg.Root, cfg.BaseURL())
	s.OnRemove = forgetViews
	for _, name := range args {
		if err := s.Remove(name); err != nil {
			return err
		}
		fmt.Println("removed", name)
	}
	return nil
}

func cmdGC() error {
	cfg := config.Load()
	s := store.New(cfg.Root, cfg.BaseURL())
	s.OnRemove = forgetViews
	n, err := s.GC()
	if err != nil {
		return err
	}
	fmt.Printf("pruned %d expired preview(s)\n", n)
	return nil
}
