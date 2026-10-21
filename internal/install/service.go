package install

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/samuelloranger/glim/internal/serve"
)

// serviceMarker marks a unit or plist as written by glim. A file without it
// is someone else's and is backed up before glim replaces it.
const serviceMarker = `glim:managed — written by "glim install service"; re-running it rewrites this file`

const (
	unitName    = "glim.service"
	plistLabel  = "glim"
	healthTries = 100 // x healthEvery = 10 s
	healthEvery = 100 * time.Millisecond
)

// ServiceDeps are the seams `install service` needs beyond Deps: the host OS,
// the commands it queries, and the probe used to prove the server came up.
// Zero values select the real thing where one exists.
type ServiceDeps struct {
	OS       string // "linux" or "darwin"; empty means runtime.GOOS
	UID      int
	User     string
	BinPath  string // absolute, symlinks kept; empty means Deps.GlimPath
	Path     string // the installer's $PATH, copied into the service
	TempDirs []string
	// Output runs a command and returns its stdout.
	Output func(name string, args ...string) (string, error)
	// State reads serve.json; the server writes it once it is listening.
	State    func() (serve.State, bool)
	PidAlive func(pid int) bool
	Probe    func(serve.State) bool
	Sleep    func(time.Duration)
	Now      func() time.Time
	// Domain is the configured public URL, if any.
	Domain string
	// HasAccounts reports whether a dashboard account exists yet.
	HasAccounts func() (bool, error)
}

func (s ServiceDeps) sleep(d time.Duration) {
	if s.Sleep != nil {
		s.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (s ServiceDeps) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// svc bundles what one install or uninstall run needs.
type svc struct {
	d    Deps
	s    ServiceDeps
	os   string
	bin  string
	path string // service file
}

func newSvc(d Deps) (*svc, error) {
	v := &svc{d: d, s: d.Service, os: d.Service.OS, bin: d.Service.BinPath}
	if v.bin == "" {
		v.bin = d.GlimPath
	}
	switch v.os {
	case "linux":
		v.path = filepath.Join(d.Home, ".config", "systemd", "user", unitName)
	case "darwin":
		v.path = filepath.Join(d.Home, "Library", "LaunchAgents", plistLabel+".plist")
	default:
		return nil, fmt.Errorf("installing a service is not supported on %q (linux and darwin only)", v.os)
	}
	return v, nil
}

func (v *svc) has(name string) bool { return v.d.HasCommand == nil || v.d.HasCommand(name) }

func (v *svc) output(name string, args ...string) (string, error) {
	if v.s.Output == nil {
		return "", errors.New("no command runner")
	}
	return v.s.Output(name, args...)
}

func (v *svc) target() string { return fmt.Sprintf("gui/%d/%s", v.s.UID, plistLabel) }
func (v *svc) domain() string { return fmt.Sprintf("gui/%d", v.s.UID) }

var launchctlPID = regexp.MustCompile(`(?m)^\s*pid = (\d+)\s*$`)

// servicePID is the pid of the running service: 0 when it is not running,
// an error when it cannot be told.
func (v *svc) servicePID() (int, error) {
	if v.os == "linux" {
		out, err := v.output("systemctl", "--user", "show", "-p", "MainPID", "--value", unitName)
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(strings.TrimSpace(out))
	}
	out, err := v.output("launchctl", "print", v.target())
	if err != nil {
		return 0, err
	}
	m := launchctlPID.FindStringSubmatch(out)
	if m == nil {
		return 0, errors.New("launchctl print shows no pid")
	}
	return strconv.Atoi(m[1])
}

func (v *svc) managed(content []byte) bool {
	head := content
	if len(head) > 1024 {
		head = head[:1024]
	}
	return bytes.Contains(head, []byte("glim:managed"))
}

// installService writes the service file, loads it, and checks the server
// answers. A failed health check leaves the service installed.
func installService(d Deps) ([]string, error) {
	v, err := newSvc(d)
	if err != nil {
		return nil, err
	}
	var steps []string
	if v.os == "linux" {
		if !v.has("systemctl") {
			return nil, errNoUserSession
		}
		if _, err := v.output("systemctl", "--user", "show-environment"); err != nil {
			return nil, errNoUserSession
		}
	}
	if isTempBuild(v.bin, v.s.TempDirs) {
		return nil, errors.New("glim is running from a temporary build; install the binary first")
	}
	if err := v.refuseOtherServer(); err != nil {
		return nil, err
	}

	// oldPID is the server being replaced on macOS, which must not pass the
	// health check for the new one. systemctl restart only returns once the
	// new process is up, so Linux needs no such guard.
	oldPID := 0
	var content string
	if v.os == "linux" {
		content = unitFile(v.bin, v.s.Path)
	} else {
		content = plistFile(v.bin, v.s.Path, v.logPath())
	}
	took, err := v.write(content)
	if err != nil {
		return nil, err
	}
	steps = append(steps, "wrote "+v.path)
	if took != "" {
		steps = append(steps, fmt.Sprintf("took over existing %s (backup: %s)", filepath.Base(v.path), took))
		// The old file may carry settings the template lacks, and a re-install
		// rewrites the file, so they belong somewhere glim never touches.
		keep := "re-apply any setting from the backup you still need"
		if v.os == "linux" {
			keep = "settings in the backup are not carried over; put any you still need in a drop-in, " +
				v.path + ".d/override.conf, which re-installs leave alone"
		}
		steps = append(steps, "note: "+keep)
	}

	if v.os == "linux" {
		for _, args := range [][]string{{"daemon-reload"}, {"enable", unitName}, {"restart", unitName}} {
			if err := v.d.Run("systemctl", append([]string{"--user"}, args...)...); err != nil {
				return steps, fmt.Errorf("systemctl --user %s failed: %w", args[0], err)
			}
		}
		steps = append(steps, "enabled and started "+unitName)
		if v.has("loginctl") {
			if out, err := v.output("loginctl", "show-user", v.s.User, "-p", "Linger", "--value"); err == nil && strings.TrimSpace(out) == "no" {
				steps = append(steps, "warning: the server stops when you log out; to keep it running: loginctl enable-linger "+v.s.User)
			}
		}
	} else {
		if _, err := v.output("launchctl", "print", v.target()); err == nil {
			oldPID, _ = v.servicePID()
			if err := v.d.Run("launchctl", "bootout", v.target()); err != nil {
				return steps, fmt.Errorf("launchctl bootout failed: %w", err)
			}
			// bootout can return while the old job is still being torn down,
			// and bootstrap fails until it is gone.
			for i := 0; i < healthTries; i++ {
				if _, err := v.output("launchctl", "print", v.target()); err != nil {
					break
				}
				v.s.sleep(healthEvery)
			}
		}
		if err := v.d.Run("launchctl", "bootstrap", v.domain(), v.path); err != nil {
			return steps, fmt.Errorf("launchctl bootstrap failed: %w", err)
		}
		steps = append(steps, "loaded "+plistLabel+" into launchd")
	}

	st, err := v.waitHealthy(oldPID)
	if err != nil {
		return steps, err
	}
	base := st.LoopbackURL()
	if v.s.Domain != "" {
		base = strings.TrimRight(v.s.Domain, "/")
	}
	steps = append(steps, "server running at "+base)
	if v.s.HasAccounts != nil {
		if has, err := v.s.HasAccounts(); err == nil && !has {
			steps = append(steps, "open "+st.LoopbackURL()+" and create your account before exposing it")
		}
	}
	if v.os == "darwin" {
		steps = append(steps, "note: a LaunchAgent only runs while you are logged in")
	}
	return steps, nil
}

var errNoUserSession = errors.New("no systemd user session; run `glim serve` yourself or under your own supervisor")

func (v *svc) logPath() string {
	return filepath.Join(v.d.Home, "Library", "Logs", "glim", "serve.log")
}

// refuseOtherServer fails when a glim server runs that is not this service.
// An unknown service pid fails open: the health check catches a port clash.
func (v *svc) refuseOtherServer() error {
	if v.s.State == nil || v.s.PidAlive == nil || v.s.Probe == nil {
		return nil
	}
	st, ok := v.s.State()
	if !ok || st.PID <= 0 || !v.s.PidAlive(st.PID) || !v.s.Probe(st) {
		return nil
	}
	pid, err := v.servicePID()
	if err != nil || pid == st.PID {
		return nil
	}
	return fmt.Errorf("a glim server is already running (pid %d); stop it first", st.PID)
}

// write puts content at the service file, first moving a file glim did not
// write aside. It returns the backup's base name when it took one over.
func (v *svc) write(content string) (backup string, err error) {
	if err := os.MkdirAll(filepath.Dir(v.path), 0o755); err != nil {
		return "", err
	}
	if v.os == "darwin" {
		if err := os.MkdirAll(filepath.Dir(v.logPath()), 0o755); err != nil {
			return "", err
		}
	}
	if old, err := os.ReadFile(v.path); err == nil && !v.managed(old) {
		bak := v.path + ".bak"
		if _, err := os.Lstat(bak); err == nil {
			bak += "." + strconv.FormatInt(v.s.now().Unix(), 10)
		}
		if err := os.Rename(v.path, bak); err != nil {
			return "", err
		}
		backup = filepath.Base(bak)
	}
	return backup, os.WriteFile(v.path, []byte(content), 0o644)
}

// waitHealthy polls until serve.json names the service's own pid and its
// address accepts connections, so a stale file from an earlier run cannot pass.
// A service still running as oldPID is the server being replaced, not the new one.
func (v *svc) waitHealthy(oldPID int) (serve.State, error) {
	for i := 0; i < healthTries; i++ {
		if pid, err := v.servicePID(); err == nil && pid > 0 && pid != oldPID {
			if st, ok := v.s.State(); ok && st.PID == pid && v.s.Probe(st) {
				return st, nil
			}
		}
		v.s.sleep(healthEvery)
	}
	where := "journalctl --user -u glim -n 50"
	if v.os == "darwin" {
		where = "the log at " + v.logPath()
	}
	return serve.State{}, fmt.Errorf("the service is installed but the server did not answer within 10s; "+
		"it is left installed so you can read %s and re-run `glim install service`", where)
}

// uninstallService stops and removes a service glim wrote.
func uninstallService(d Deps) ([]string, error) {
	v, err := newSvc(d)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(v.path)
	if errors.Is(err, os.ErrNotExist) {
		return []string{"no glim service installed"}, nil
	}
	if err != nil {
		return nil, err
	}
	if !v.managed(content) {
		return nil, fmt.Errorf("%s was not written by glim; remove it yourself", filepath.Base(v.path))
	}
	var steps []string
	if v.os == "linux" {
		if err := v.d.Run("systemctl", "--user", "disable", "--now", unitName); err != nil {
			return nil, fmt.Errorf("systemctl --user disable failed: %w", err)
		}
	} else {
		// Not loaded is the common case after a logout; nothing to report.
		_ = v.d.Run("launchctl", "bootout", v.target())
	}
	if err := os.Remove(v.path); err != nil {
		return nil, err
	}
	steps = append(steps, "stopped and removed "+v.path)
	if v.os == "linux" {
		if err := v.d.Run("systemctl", "--user", "daemon-reload"); err != nil {
			return steps, fmt.Errorf("systemctl --user daemon-reload failed: %w", err)
		}
		dropIns := v.path + ".d"
		if info, err := os.Stat(dropIns); err == nil && info.IsDir() {
			steps = append(steps, "left the drop-ins in "+dropIns+" in place")
		}
	}
	return steps, nil
}

// isTempBuild reports whether bin sits in a temp dir or a Go build cache,
// which is where `go run` puts its binary.
func isTempBuild(bin string, tempDirs []string) bool {
	for _, t := range tempDirs {
		if t == "" {
			continue
		}
		t = filepath.Clean(t)
		if bin == t || strings.HasPrefix(bin, t+string(filepath.Separator)) {
			return true
		}
	}
	for _, seg := range strings.Split(filepath.ToSlash(bin), "/") {
		if strings.HasPrefix(seg, "go-build") {
			return true
		}
	}
	return false
}

// unitFile is the systemd user unit that runs `<bin> serve`. No flags are
// passed, so config changes apply on restart; secrets stay out of the file.
func unitFile(bin, path string) string {
	var b strings.Builder
	b.WriteString("# " + serviceMarker + "\n")
	// StartLimitIntervalSec=0 keeps retrying forever: a bind address that only
	// appears later (a container bridge, a VPN) would otherwise exhaust the
	// start limit during boot and leave the server down.
	b.WriteString("[Unit]\nDescription=glim preview server\nAfter=network.target\nStartLimitIntervalSec=0\n\n[Service]\n")
	// systemd expands no variables in the executable word, so $ stays as is.
	b.WriteString("ExecStart=" + unitQuote(bin) + " serve\n")
	if path != "" {
		b.WriteString("Environment=" + unitQuote("PATH="+path) + "\n")
	}
	b.WriteString("Restart=on-failure\nRestartSec=2\n\n[Install]\nWantedBy=default.target\n")
	return b.String()
}

// unitQuote quotes s as one systemd word and doubles % so it is not read as a
// specifier.
func unitQuote(s string) string {
	s = strings.ReplaceAll(s, "%", "%%")
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// plistFile is the launchd agent that runs `<bin> serve` while the user is
// logged in and restarts it when it fails.
func plistFile(bin, path, log string) string {
	esc := func(s string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString("<!-- " + serviceMarker + " -->\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	b.WriteString("\t<key>Label</key>\n\t<string>" + plistLabel + "</string>\n")
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n\t\t<string>" + esc(bin) + "</string>\n\t\t<string>serve</string>\n\t</array>\n")
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	b.WriteString("\t<key>KeepAlive</key>\n\t<dict>\n\t\t<key>SuccessfulExit</key>\n\t\t<false/>\n\t</dict>\n")
	b.WriteString("\t<key>StandardOutPath</key>\n\t<string>" + esc(log) + "</string>\n")
	b.WriteString("\t<key>StandardErrorPath</key>\n\t<string>" + esc(log) + "</string>\n")
	if path != "" {
		b.WriteString("\t<key>EnvironmentVariables</key>\n\t<dict>\n\t\t<key>PATH</key>\n\t\t<string>" + esc(path) + "</string>\n\t</dict>\n")
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}
