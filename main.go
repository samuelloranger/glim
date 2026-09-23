package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/mdp/qrterminal/v3"
	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/caddy"
	"github.com/samuelloranger/glim/internal/config"
	"github.com/samuelloranger/glim/internal/install"
	"github.com/samuelloranger/glim/internal/mcpserver"
	"github.com/samuelloranger/glim/internal/serve"
	"github.com/samuelloranger/glim/internal/store"
	"golang.org/x/term"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "ls", "list":
		mustRun(cmdList())
	case "rm", "remove":
		mustRun(cmdRemove(os.Args[2:]))
	case "gc":
		mustRun(cmdGC())
	case "extend":
		mustRun(cmdExtend(os.Args[2:]))
	case "pin":
		mustRun(cmdPin(os.Args[2:]))
	case "open":
		mustRun(cmdOpen(os.Args[2:]))
	case "status":
		mustRun(cmdStatus())
	case "serve":
		mustRun(cmdServe(os.Args[2:]))
	case "caddy":
		mustRun(cmdCaddy())
	case "config":
		mustRun(cmdConfig(os.Args[2:]))
	case "mcp":
		mustRun(cmdMCP())
	case "install":
		mustRun(cmdInstall(os.Args[2:]))
	case "user", "users":
		mustRun(cmdUser(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println("glim", version)
	case "help", "-h", "--help":
		usage()
	default:
		mustRun(cmdPublish(os.Args[1:]))
	}
}

func cmdPublish(args []string) error {
	entry, flagArgs := splitEntry(args)
	cfg := config.Load()
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	title := fs.String("title", "", "human title (becomes the readable slug)")
	project := fs.String("project", "", "project name")
	ttl := fs.Duration("ttl", cfg.TTLDuration(), "time to live, e.g. 6h, 30m")
	local := fs.Bool("local", false, "auto-start the built-in server and use a localhost link")
	qr := fs.Bool("qr", false, "also print a scannable QR code of the URL")
	name := fs.String("name", "", "reuse this exact slug to update in place at the same URL (created if absent); omit for a fresh random link")
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if entry == "" || fs.NArg() != 0 {
		return fmt.Errorf("usage: glim <entry.html|dir> [--title T] [--project P] [--ttl 6h] [--name SLUG] [--local]")
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
	res, err := s.Publish(entry, *title, *project, os.Getenv("GLIM_SESSION_ID"), *ttl, *name)
	if err != nil {
		return err
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

func splitEntry(args []string) (entry string, flagArgs []string) {
	valueFlags := map[string]bool{"title": true, "project": true, "ttl": true}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
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

func cmdList() error {
	cfg := config.Load()
	s := store.New(cfg.Root, cfg.BaseURL())
	list, err := s.List()
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Println("no live previews")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTITLE\tAGE\tEXPIRES IN")
	now := time.Now()
	for _, m := range list {
		expires := short(m.Expires.Sub(now))
		if m.Pinned {
			expires = "pinned"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			m.Name, truncate(m.Title, 30),
			short(now.Sub(m.Created)), expires)
	}
	return w.Flush()
}

func cmdRemove(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: glim rm <name>...")
	}
	cfg := config.Load()
	s := store.New(cfg.Root, cfg.BaseURL())
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
	n, err := s.GC()
	if err != nil {
		return err
	}
	fmt.Printf("pruned %d expired preview(s)\n", n)
	return nil
}

func cmdExtend(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: glim extend <name> <ttl>")
	}
	ttl, err := time.ParseDuration(args[1])
	if err != nil {
		return fmt.Errorf("bad ttl %q: %w", args[1], err)
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
	fmt.Printf("live:     %d preview(s) (%d pinned)\n", len(list), pinned)
	fmt.Printf("disk:     %s\n", humanBytes(bytes))
	if next.IsZero() {
		fmt.Println("next gc:  none pending")
	} else {
		fmt.Printf("next gc:  %s (in %s)\n", next.Format(time.RFC1123), short(time.Until(next)))
	}
	if code, ok := auth.ReadSetupCode(config.SetupCodePath()); ok {
		fmt.Printf("setup:    open %s/ and enter code %s\n", cfg.BaseURL(), auth.FormatSetupCode(code))
	}
	return nil
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

func cmdMCP() error {
	cfg := config.Load()
	s := store.New(cfg.Root, cfg.BaseURL())
	return mcpserver.Run(context.Background(), s, cfg.TTLDuration(), version)
}

func cmdServe(args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	bind := fs.String("bind", cfg.Bind, "address to bind (0.0.0.0 to accept a reverse proxy)")
	port := fs.Int("port", cfg.Port, "port to bind (0 = pick a free one)")
	root := fs.String("root", cfg.Root, "directory of previews to serve")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return serve.Serve(*root, *bind, *port)
}

func cmdCaddy() error {
	cfg := config.Load()
	if cfg.Domain == "" {
		return fmt.Errorf("set a domain first: glim config --domain https://glim.example.com")
	}
	upstream := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	fmt.Print(caddy.Snippet(hostFromBase(cfg.BaseURL()), upstream))
	return nil
}

func cmdConfig(args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	domain := fs.String("domain", "", "public base URL, e.g. https://glim.example.com")
	bind := fs.String("bind", "", "address the server binds, e.g. 127.0.0.1 or 0.0.0.0")
	port := fs.Int("port", 0, "port the server binds")
	root := fs.String("root", "", "directory previews are stored/served from")
	ttl := fs.String("ttl", "", "default time to live, e.g. 6h")
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
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Println("saved", config.Path())
	return nil
}

func ensureLocalServer(cfg config.Config) (string, error) {
	if st, ok := serve.ReadState(); ok && st.Root == cfg.Root && pidAlive(st.PID) && portOpen(st.Port) {
		return serve.BaseURL(st.Port), nil
	}
	port, err := serve.FreePort()
	if err != nil {
		return "", err
	}
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	logf, _ := os.OpenFile(filepath.Join(homeDir(), ".glim", "serve.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	cmd := exec.Command(self, "serve", "--root", cfg.Root, "--bind", "127.0.0.1", "--port", strconv.Itoa(port))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if logf != nil {
		cmd.Stdout, cmd.Stderr = logf, logf
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	for i := 0; i < 40; i++ {
		if portOpen(port) {
			return serve.BaseURL(port), nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return "", fmt.Errorf("local server did not start on port %d (see ~/.glim/serve.log)", port)
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func portOpen(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func cmdInstall(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: glim install <claude|codex|cursor>")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	steps, err := install.Install(args[0], install.Deps{
		Home:       homeDir(),
		GlimPath:   self,
		HasCommand: func(name string) bool { _, e := exec.LookPath(name); return e == nil },
		Run:        install.DefaultRun,
	})
	for _, s := range steps {
		fmt.Println("✓", s)
	}
	return err
}

var stdin = bufio.NewReader(os.Stdin)

func cmdUser(args []string) error {
	db, err := auth.Open(config.DBPath())
	if err != nil {
		return err
	}
	defer db.Close()
	return runUser(args, db, stdin, os.Stdout)
}

func runUser(args []string, db *auth.DB, in *bufio.Reader, out io.Writer) error {
	ctx := context.Background()
	usage := fmt.Errorf("usage: glim user ls | passwd <name> | rm <name>")
	if len(args) == 0 {
		return usage
	}
	switch args[0] {
	case "ls", "list":
		users, err := db.ListUsers(ctx)
		if err != nil {
			return err
		}
		if len(users) == 0 {
			fmt.Fprintln(out, "no users (open the dashboard with the setup code from `glim serve`)")
			return nil
		}
		w := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "USERNAME\tCREATED")
		for _, u := range users {
			fmt.Fprintf(w, "%s\t%s\n", u.Username, u.CreatedAt.Local().Format(time.RFC1123))
		}
		return w.Flush()
	case "passwd":
		if len(args) != 2 {
			return usage
		}
		if _, err := db.GetUser(ctx, args[1]); err != nil {
			return fmt.Errorf("no such user: %s", args[1])
		}
		p1, err := readPassword(in, "New password: ")
		if err != nil {
			return err
		}
		p2, err := readPassword(in, "Repeat password: ")
		if err != nil {
			return err
		}
		if p1 != p2 {
			return fmt.Errorf("passwords do not match")
		}
		if err := db.SetPassword(ctx, args[1], p1); err != nil {
			return passwordError(err)
		}
		fmt.Fprintf(out, "password updated for %s (signed out everywhere)\n", args[1])
		return nil
	case "rm", "remove":
		if len(args) != 2 {
			return usage
		}
		if err := db.DeleteUser(ctx, args[1]); err != nil {
			return fmt.Errorf("no such user: %s", args[1])
		}
		fmt.Fprintln(out, "removed", args[1])
		return nil
	default:
		return usage
	}
}

func passwordError(err error) error {
	switch err {
	case auth.ErrPasswordTooShort:
		return fmt.Errorf("password must be at least %d characters", auth.MinPasswordChars)
	case auth.ErrPasswordTooLong:
		return fmt.Errorf("password must be at most %d bytes", auth.MaxPasswordBytes)
	}
	return err
}

// readPassword prompts on stderr; it hides input on a terminal and reads a
// plain line otherwise (scripts, tests).
func readPassword(in *bufio.Reader, prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if in == stdin && term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return string(b), err
	}
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func hostFromBase(base string) string {
	h := base
	for _, p := range []string{"https://", "http://"} {
		h = strings.TrimPrefix(h, p)
	}
	return h
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func short(d time.Duration) string {
	d = d.Round(time.Minute)
	if d < 0 {
		return "expired"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func usage() {
	fmt.Fprint(os.Stderr, `glim — publish HTML previews and serve them behind a reverse proxy

usage:
  glim <entry.html|dir> [--title T] [--project P] [--ttl 6h] [--local] [--qr]
                                              publish, print the URL
  glim serve [--port N] [--root DIR]          run the preview server
  glim config [--domain URL --port N ...]     show or set config
  glim caddy                                  print the reverse-proxy vhost
  glim ls | rm <name>... | gc                 manage previews
  glim extend <name> <ttl> | pin <name>       change a preview's lifetime
  glim open <name> | status                   open a link / show instance status
  glim mcp                                    run as MCP server
  glim install <claude|codex|cursor>          wire into an agent
  glim user ls | passwd <name> | rm <name>    manage dashboard accounts
  glim version

config: ~/.glim/config.json (env: GLIM_DOMAIN, GLIM_PORT, GLIM_ROOT, GLIM_TTL, GLIM_SESSION_ID)
`)
}

func mustRun(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "glim:", err)
		os.Exit(1)
	}
}
