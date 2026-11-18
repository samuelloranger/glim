package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/config"
	"github.com/samuelloranger/glim/internal/serve"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "ls", "list":
		mustRun(cmdList(os.Args[2:]))
	case "rm", "remove":
		mustRun(cmdRemove(os.Args[2:]))
	case "gc":
		mustRun(cmdGC())
	case "extend":
		mustRun(cmdExtend(os.Args[2:]))
	case "pin":
		mustRun(cmdPin(os.Args[2:]))
	case "unpin":
		mustRun(cmdUnpin(os.Args[2:]))
	case "lock":
		mustRun(cmdLock(os.Args[2:]))
	case "unlock":
		mustRun(cmdUnlock(os.Args[2:]))
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
	case "uninstall":
		mustRun(cmdUninstall(os.Args[2:]))
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

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func ensureLocalServer(cfg config.Config) (string, error) {
	if st, ok := serve.ReadState(); ok && st.Root == cfg.Root && pidAlive(st.PID) && serve.Probe(st) {
		return st.LoopbackURL(), nil
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
	// Reap the child if it exits, so a long-lived glim mcp that restarts a
	// dying server does not collect zombies.
	go cmd.Wait()
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

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// publicHost is the host[:port] of the configured domain, or "" when none is set.
func publicHost(base string) string {
	if base == "" {
		return ""
	}
	u, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return u.Host
}

// pushSubscriber is the VAPID "sub" contact claim: the public https base when
// there is one, otherwise a placeholder mailto (push services need a valid URI).
func pushSubscriber(base string) string {
	if strings.HasPrefix(base, "https://") {
		return strings.TrimRight(base, "/")
	}
	return "mailto:glim@example.com"
}

func hostFromBase(base string) string {
	h := base
	for _, p := range []string{"https://", "http://"} {
		h = strings.TrimPrefix(h, p)
	}
	return h
}

// openExistingDB opens the dashboard database only if it already exists, so
// CLI commands never create it as a side effect. It returns nil otherwise.
func openExistingDB() *auth.DB {
	if _, err := os.Stat(config.DBPath()); err != nil {
		return nil
	}
	db, err := auth.Open(config.DBPath())
	if err != nil {
		return nil
	}
	return db
}

// viewStatsNow reads per-preview view stats (nil without a dashboard DB).
func viewStatsNow() map[string]auth.ViewStat {
	db := openExistingDB()
	if db == nil {
		return nil
	}
	defer db.Close()
	stats, _ := db.ViewStats(context.Background())
	return stats
}

// dbComments reaches the comments in the dashboard database, opening it for
// each call (like viewStatsNow) so a long-running glim mcp never holds it, and
// never creates it.
type dbComments struct{}

func (dbComments) with(f func(*auth.DB) error) error {
	db := openExistingDB()
	if db == nil {
		return fmt.Errorf("comments are stored in the dashboard database, which does not exist yet: run glim serve and create the dashboard account first")
	}
	defer db.Close()
	return f(db)
}

func (c dbComments) Comments(ctx context.Context, preview string, includeResolved bool) (out []auth.Comment, err error) {
	err = c.with(func(db *auth.DB) error {
		out, err = db.Comments(ctx, preview, includeResolved)
		return err
	})
	return out, err
}

func (c dbComments) ResolveComment(ctx context.Context, preview string, id int64, resolved bool, reply *string) (out auth.Comment, err error) {
	err = c.with(func(db *auth.DB) error {
		out, err = db.ResolveComment(ctx, preview, id, resolved, reply)
		return err
	})
	return out, err
}

func (c dbComments) OpenCommentCounts(ctx context.Context) (out map[string]int, err error) {
	err = c.with(func(db *auth.DB) error {
		out, err = db.OpenCommentCounts(ctx)
		return err
	})
	return out, err
}

// forgetViews drops a removed or expired preview's view stats and comments,
// if a DB exists.
func forgetViews(name string) {
	if db := openExistingDB(); db != nil {
		defer db.Close()
		_ = db.DeleteViews(context.Background(), name)
		_ = db.DeleteComments(context.Background(), name)
	}
}

// agoShort renders how long ago something happened, e.g. "just now" or "5m ago".
func agoShort(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	return short(d) + " ago"
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
  glim <entry.html|dir> [--title T] [--project P] [--ttl 6h] [--local] [--qr] [--json]
                                              publish, print the URL (or JSON)
  glim - [--format html|md|txt|json] ...      same, reading the entry from stdin
  glim serve [--port N] [--root DIR]          run the preview server
  glim config [--domain URL --port N ...]     show or set config (and Listen: --llm-url, --tts-url, ...)
  glim caddy                                  print the reverse-proxy vhost
  glim ls [--project P] [--json] | rm <name>... | gc
                                              manage previews
  glim extend <name> <ttl> | pin <name> | unpin <name>
                                              change a preview's lifetime
  glim <entry> --password | lock <name> | unlock <name>
                                              password-protect a preview
  glim open <name> | status                   open a link / show instance status
  glim mcp                                    run as MCP server
  glim install [--skill] <claude|codex|cursor>  wire into an agent (--skill: SKILL.md only)
  glim install service                        run the server in the background (systemd user unit / launchd agent)
  glim uninstall <claude|codex|cursor|service>  undo install
  glim user ls | passwd <email> | rm <email>  manage dashboard accounts
  glim version

config: ~/.glim/config.json; GLIM_* keys also read from ~/.glim/env (env: GLIM_DOMAIN, GLIM_PORT, GLIM_ROOT, GLIM_TTL, GLIM_LIVE_RELOAD, GLIM_SELF_FETCH, GLIM_SESSION_ID)
`)
}

func mustRun(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "glim:", err)
		os.Exit(1)
	}
}
