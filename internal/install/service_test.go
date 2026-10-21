package install

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/serve"
)

const (
	mainPIDCmd = "systemctl --user show -p MainPID --value glim.service"
	envCmd     = "systemctl --user show-environment"
	lingerCmd  = "loginctl show-user tester -p Linger --value"
	printCmd   = "launchctl print gui/501/glim"
)

// sysFake stands in for the host: commands, serve.json and the probe.
type sysFake struct {
	calls     []string
	outputs   map[string]string // command -> stdout; absent means the command fails
	runErr    map[string]error  // command -> error from Run
	state     *serve.State
	alive     bool
	reachable bool
	sleeps    int
	accounts  bool
	noLogin   bool // loginctl missing
	// launchd: after bootout, print keeps showing the old job for outLag
	// more calls, then fails. bootstrap loads the job as bootPID, which then
	// owns serve.json; bootPID 0 means the new server never comes up.
	bootPID   int
	outLag    int
	bootedOut bool
}

func (f *sysFake) deps(t *testing.T, osName string) Deps {
	t.Helper()
	f.outputs = map[string]string{}
	f.runErr = map[string]error{}
	f.alive, f.reachable = true, true
	return Deps{
		Home:     t.TempDir(),
		GlimPath: "/opt/glim/bin/glim",
		HasCommand: func(n string) bool {
			return !(n == "loginctl" && f.noLogin)
		},
		Run: func(name string, args ...string) error {
			c := strings.Join(append([]string{name}, args...), " ")
			f.calls = append(f.calls, c)
			if err := f.runErr[c]; err != nil {
				return err
			}
			switch {
			case c == "launchctl bootout gui/501/glim":
				f.bootedOut = true
			case strings.HasPrefix(c, "launchctl bootstrap"):
				f.bootedOut = false
				if f.bootPID == 0 {
					break
				}
				f.outputs[printCmd] = "gui/501/glim = {\n\tpid = " + strconv.Itoa(f.bootPID) + "\n}\n"
				f.state = &serve.State{Port: 8787, Bind: "127.0.0.1", PID: f.bootPID}
			}
			return nil
		},
		Service: ServiceDeps{
			OS: osName, UID: 501, User: "tester", Path: "/usr/local/bin:/usr/bin",
			TempDirs: []string{"/tmp"},
			Output: func(name string, args ...string) (string, error) {
				c := strings.Join(append([]string{name}, args...), " ")
				if c == printCmd && f.bootedOut {
					if f.outLag == 0 {
						delete(f.outputs, printCmd)
					}
					f.outLag--
				}
				out, ok := f.outputs[c]
				if !ok {
					return "", errors.New("command failed: " + c)
				}
				return out, nil
			},
			State: func() (serve.State, bool) {
				if f.state == nil {
					return serve.State{}, false
				}
				return *f.state, true
			},
			PidAlive:    func(int) bool { return f.alive },
			Probe:       func(serve.State) bool { return f.reachable },
			Sleep:       func(time.Duration) { f.sleeps++ },
			Now:         func() time.Time { return time.Unix(1700000000, 0) },
			HasAccounts: func() (bool, error) { return f.accounts, nil },
		},
	}
}

func (f *sysFake) ran(cmd string) bool {
	for _, c := range f.calls {
		if c == cmd {
			return true
		}
	}
	return false
}

func (f *sysFake) idx(cmd string) int {
	for i, c := range f.calls {
		if c == cmd {
			return i
		}
	}
	return -1
}

// linuxUp makes a healthy user systemd whose service runs as pid.
func (f *sysFake) linuxUp(pid int) {
	f.outputs[envCmd] = "PATH=/usr/bin\n"
	f.outputs[mainPIDCmd] = strconv.Itoa(pid) + "\n"
	f.outputs[lingerCmd] = "yes\n"
	f.state = &serve.State{Port: 8787, Bind: "127.0.0.1", PID: pid}
}

func unitPath(d Deps) string {
	return filepath.Join(d.Home, ".config", "systemd", "user", "glim.service")
}

func plistPath(d Deps) string {
	return filepath.Join(d.Home, "Library", "LaunchAgents", "glim.plist")
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasSvcStep(steps []string, sub string) bool {
	for _, s := range steps {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestInstallServiceLinuxFresh(t *testing.T) {
	f := &sysFake{accounts: false}
	d := f.deps(t, "linux")
	f.linuxUp(4242)
	d.Service.BinPath = "/home/u/.local/bin/glim"

	steps, err := Install("service", d)
	if err != nil {
		t.Fatalf("err = %v, steps = %v", err, steps)
	}
	unit := mustRead(t, unitPath(d))
	if !strings.HasPrefix(unit, "# glim:managed") {
		t.Errorf("unit lacks the marker:\n%s", unit)
	}
	for _, want := range []string{
		`ExecStart="/home/u/.local/bin/glim" serve`,
		`Environment="PATH=/usr/local/bin:/usr/bin"`,
		"Restart=on-failure", "WantedBy=default.target",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit missing %q:\n%s", want, unit)
		}
	}
	if strings.Contains(unit, "API_KEY") {
		t.Error("unit must hold no secrets")
	}
	// daemon-reload, enable, then restart, in that order.
	a, b, c := f.idx("systemctl --user daemon-reload"), f.idx("systemctl --user enable glim.service"), f.idx("systemctl --user restart glim.service")
	if a < 0 || b < a || c < b {
		t.Errorf("calls out of order: %v", f.calls)
	}
	if !hasSvcStep(steps, "server running at http://127.0.0.1:8787") {
		t.Errorf("steps = %v", steps)
	}
	if !hasSvcStep(steps, "open http://127.0.0.1:8787 and create your account") {
		t.Errorf("first-run account hint missing: %v", steps)
	}
	if hasSvcStep(steps, "warning:") {
		t.Errorf("linger is on, no warning expected: %v", steps)
	}
}

func TestInstallServiceNoAccountHintOnceAccountsExist(t *testing.T) {
	f := &sysFake{accounts: true}
	d := f.deps(t, "linux")
	f.linuxUp(10)
	d.Service.Domain = "https://glim.example.com/"
	steps, err := Install("service", d)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSvcStep(steps, "server running at https://glim.example.com") || hasSvcStep(steps, "create your account") {
		t.Errorf("steps = %v", steps)
	}
}

func TestInstallServiceLinuxReinstallManaged(t *testing.T) {
	f := &sysFake{}
	d := f.deps(t, "linux")
	f.linuxUp(5)
	mustWrite(t, unitPath(d), "# glim:managed — old\n[Service]\nExecStart=/old/glim serve\n")

	steps, err := Install("service", d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(mustRead(t, unitPath(d)), "/old/glim") || !strings.Contains(mustRead(t, unitPath(d)), "/opt/glim/bin/glim") {
		t.Error("managed unit not rewritten with the current binary")
	}
	if _, err := os.Stat(unitPath(d) + ".bak"); err == nil {
		t.Error("a managed file must not be backed up")
	}
	if hasSvcStep(steps, "took over") {
		t.Errorf("steps = %v", steps)
	}
	if !f.ran("systemctl --user restart glim.service") {
		t.Error("re-install must restart so a new binary is picked up")
	}
}

func TestInstallServiceLinuxTakeover(t *testing.T) {
	cases := []struct {
		name, wantBak string
		preBak        bool
	}{
		{"no earlier backup", "glim.service.bak", false},
		{"backup already exists", "glim.service.bak.1700000000", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &sysFake{}
			d := f.deps(t, "linux")
			f.linuxUp(5)
			hand := "[Service]\nExecStart=/home/u/.local/bin/glim serve\nEnvironmentFile=/x\n"
			mustWrite(t, unitPath(d), hand)
			if c.preBak {
				mustWrite(t, unitPath(d)+".bak", "older backup")
			}
			steps, err := Install("service", d)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Dir(unitPath(d))
			if got := mustRead(t, filepath.Join(dir, c.wantBak)); got != hand {
				t.Errorf("backup %s = %q", c.wantBak, got)
			}
			if c.preBak && mustRead(t, unitPath(d)+".bak") != "older backup" {
				t.Error("existing .bak was overwritten")
			}
			if !strings.HasPrefix(mustRead(t, unitPath(d)), "# glim:managed") {
				t.Error("unit not replaced")
			}
			if !hasSvcStep(steps, "took over existing glim.service (backup: "+c.wantBak+")") {
				t.Errorf("steps = %v", steps)
			}
		})
	}
}

func TestInstallServiceLinuxLingerWarning(t *testing.T) {
	f := &sysFake{}
	d := f.deps(t, "linux")
	f.linuxUp(5)
	f.outputs[lingerCmd] = "no\n"
	steps, err := Install("service", d)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSvcStep(steps, "warning: the server stops when you log out; to keep it running: loginctl enable-linger tester") {
		t.Errorf("steps = %v", steps)
	}
	if f.ran("loginctl enable-linger tester") {
		t.Error("linger must be reported, never enabled")
	}

	// loginctl missing: skip silently.
	f2 := &sysFake{noLogin: true}
	d2 := f2.deps(t, "linux")
	f2.linuxUp(5)
	f2.outputs[lingerCmd] = "no\n"
	steps, err = Install("service", d2)
	if err != nil || hasSvcStep(steps, "warning:") {
		t.Errorf("err = %v, steps = %v", err, steps)
	}
}

func TestInstallServiceLinuxNoUserSession(t *testing.T) {
	f := &sysFake{}
	d := f.deps(t, "linux") // show-environment fails
	_, err := Install("service", d)
	if err == nil || !strings.Contains(err.Error(), "no systemd user session") {
		t.Fatalf("err = %v", err)
	}
	if _, serr := os.Stat(unitPath(d)); serr == nil {
		t.Error("nothing should be written without a user session")
	}
	if len(f.calls) != 0 {
		t.Errorf("no commands should run: %v", f.calls)
	}
}

func TestInstallServiceRefusesTemporaryBuild(t *testing.T) {
	for _, bin := range []string{"/tmp/go-build123/b001/exe/glim", "/tmp/glim", "/home/u/.cache/go-build/x/glim"} {
		f := &sysFake{}
		d := f.deps(t, "linux")
		f.linuxUp(5)
		d.Service.BinPath = bin
		_, err := Install("service", d)
		if err == nil || !strings.Contains(err.Error(), "temporary build; install the binary first") {
			t.Errorf("%s: err = %v", bin, err)
		}
		if _, serr := os.Stat(unitPath(d)); serr == nil {
			t.Errorf("%s: unit written", bin)
		}
	}
}

func TestInstallServiceAlreadyRunningElsewhere(t *testing.T) {
	cases := []struct {
		name    string
		osName  string
		svcPID  string // MainPID/pid output; "" means the lookup fails
		running bool   // the server in serve.json is alive and answering
		wantErr string
	}{
		{"linux: a stray server", "linux", "0", true, "a glim server is already running (pid 777); stop it first"},
		{"linux: another pid is the service", "linux", "900", true, "pid 777"},
		{"linux: running service is this service", "linux", "777", true, ""},
		{"linux: lookup fails, fail open", "linux", "", true, ""},
		{"linux: stale serve.json", "linux", "0", false, ""},
		{"darwin: a stray server", "darwin", "pid=900", true, "pid 777"},
		{"darwin: running service is this service", "darwin", "pid=777", true, ""},
		{"darwin: not loaded, fail open", "darwin", "", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &sysFake{}
			d := f.deps(t, c.osName)
			f.state = &serve.State{Port: 8787, PID: 777}
			f.alive, f.reachable = c.running, c.running
			if c.osName == "linux" {
				f.outputs[envCmd] = ""
				f.outputs[lingerCmd] = "yes"
				if c.svcPID != "" {
					f.outputs[mainPIDCmd] = c.svcPID
				}
			} else if c.svcPID != "" {
				f.outputs[printCmd] = "gui/501/glim = {\n\tstate = running\n\tpid = " + strings.TrimPrefix(c.svcPID, "pid=") + "\n}\n"
				f.bootPID = 778
			}
			if c.wantErr != "" {
				_, err := Install("service", d)
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want %q", err, c.wantErr)
				}
				if f.ran("systemctl --user restart glim.service") || f.ran("launchctl bootstrap gui/501 "+plistPath(d)) {
					t.Error("refused install must not start anything")
				}
				return
			}
			// Allowed: the service is the running server, or the lookup failed
			// (fail open) and the health check, not the pre-check, judges it.
			steps, err := Install("service", d)
			if err != nil && strings.Contains(err.Error(), "already running") {
				t.Fatalf("unexpected refusal: %v (%v)", err, steps)
			}
			if c.svcPID == "pid=777" || c.svcPID == "777" {
				if err != nil {
					t.Fatalf("running service should be restarted: %v", err)
				}
			}
		})
	}
}

func TestInstallServiceHealthCheck(t *testing.T) {
	t.Run("ignores a stale serve.json from an older run", func(t *testing.T) {
		f := &sysFake{}
		d := f.deps(t, "linux")
		f.linuxUp(500)
		f.state = &serve.State{Port: 8787, Bind: "127.0.0.1", PID: 111} // old pid
		f.alive = false                                                 // so the pre-check does not refuse
		steps, err := Install("service", d)
		if err == nil {
			t.Fatalf("stale file passed the health check: %v", steps)
		}
		if f.sleeps != healthTries {
			t.Errorf("polled %d times, want %d", f.sleeps, healthTries)
		}
		msg := err.Error()
		if !strings.Contains(msg, "journalctl --user -u glim -n 50") || !strings.Contains(msg, "left installed") {
			t.Errorf("error = %q", msg)
		}
		// No rollback: unit stays, nothing disabled or removed.
		if _, serr := os.Stat(unitPath(d)); serr != nil {
			t.Error("a failed health check must leave the unit in place")
		}
		for _, c := range f.calls {
			if strings.Contains(c, "disable") || strings.Contains(c, "stop") {
				t.Errorf("rolled back with %q", c)
			}
		}
	})

	t.Run("passes once serve.json names the service pid", func(t *testing.T) {
		f := &sysFake{}
		d := f.deps(t, "linux")
		f.linuxUp(500)
		f.state = &serve.State{Port: 8787, PID: 111}
		f.alive = false
		polls := 0
		d.Service.Sleep = func(time.Duration) {
			polls++
			if polls == 3 {
				f.state = &serve.State{Port: 9001, Bind: "0.0.0.0", PID: 500}
			}
		}
		steps, err := Install("service", d)
		if err != nil {
			t.Fatal(err)
		}
		if polls != 3 || !hasSvcStep(steps, "server running at http://127.0.0.1:9001") {
			t.Errorf("polls = %d, steps = %v", polls, steps)
		}
	})

	t.Run("fails when the port never answers", func(t *testing.T) {
		f := &sysFake{}
		d := f.deps(t, "linux")
		f.linuxUp(500)
		d.Service.Probe = func(serve.State) bool { return false }
		if _, err := Install("service", d); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("service pid 0 never matches", func(t *testing.T) {
		f := &sysFake{}
		d := f.deps(t, "linux")
		f.linuxUp(0)
		f.state = &serve.State{Port: 8787, PID: 0}
		f.alive = false
		if _, err := Install("service", d); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestInstallServiceCommandFailure(t *testing.T) {
	f := &sysFake{}
	d := f.deps(t, "linux")
	f.linuxUp(5)
	f.runErr["systemctl --user enable glim.service"] = errors.New("boom")
	_, err := Install("service", d)
	if err == nil || !strings.Contains(err.Error(), "enable failed") {
		t.Fatalf("err = %v", err)
	}
	if f.ran("systemctl --user restart glim.service") {
		t.Error("restart ran after a failed enable")
	}
}

func TestInstallServiceDarwinFresh(t *testing.T) {
	f := &sysFake{}
	d := f.deps(t, "darwin")
	d.Service.BinPath = "/opt/homebrew/bin/glim"
	f.state = &serve.State{Port: 8787, Bind: "127.0.0.1", PID: 321}
	// Not loaded yet: print fails until bootstrap has run, then reports the pid.
	d.Run = func(name string, args ...string) error {
		c := strings.Join(append([]string{name}, args...), " ")
		f.calls = append(f.calls, c)
		if strings.HasPrefix(c, "launchctl bootstrap") {
			f.outputs[printCmd] = "gui/501/glim = {\n\tactive count = 1\n\tpid = 321\n}\n"
		}
		return nil
	}
	steps, err := Install("service", d)
	if err != nil {
		t.Fatalf("err = %v, steps = %v", err, steps)
	}
	plist := mustRead(t, plistPath(d))
	for _, want := range []string{
		"<?xml", "<!-- glim:managed", "<string>glim</string>",
		"<string>/opt/homebrew/bin/glim</string>", "<string>serve</string>",
		"<key>RunAtLoad</key>\n\t<true/>", "<key>SuccessfulExit</key>\n\t\t<false/>",
		filepath.Join(d.Home, "Library", "Logs", "glim", "serve.log"),
		"<key>PATH</key>\n\t\t<string>/usr/local/bin:/usr/bin</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist missing %q:\n%s", want, plist)
		}
	}
	if strings.Contains(plist, "API_KEY") || strings.Count(plist, "<key>") != 9 {
		t.Errorf("plist should carry only PATH in its environment:\n%s", plist)
	}
	if strings.Index(plist, "<?xml") > strings.Index(plist, "glim:managed") {
		t.Error("marker must follow the xml declaration")
	}
	if _, err := os.Stat(filepath.Join(d.Home, "Library", "Logs", "glim")); err != nil {
		t.Error("log dir not created")
	}
	if f.ran("launchctl bootout gui/501/glim") {
		t.Error("nothing was loaded, so nothing to boot out")
	}
	if !f.ran("launchctl bootstrap gui/501 " + plistPath(d)) {
		t.Errorf("calls = %v", f.calls)
	}
	if !hasSvcStep(steps, "note: a LaunchAgent only runs while you are logged in") || !hasSvcStep(steps, "server running at") {
		t.Errorf("steps = %v", steps)
	}
}

func TestInstallServiceDarwinReinstallBootsOutFirst(t *testing.T) {
	f := &sysFake{}
	d := f.deps(t, "darwin")
	f.outputs[printCmd] = "gui/501/glim = {\n\tpid = 321\n}\n"
	f.state = &serve.State{Port: 8787, PID: 321}
	f.bootPID = 322
	mustWrite(t, plistPath(d), "<?xml version=\"1.0\"?>\n<!-- glim:managed -->\n<plist/>\n")
	if _, err := Install("service", d); err != nil {
		t.Fatal(err)
	}
	out, boot := f.idx("launchctl bootout gui/501/glim"), f.idx("launchctl bootstrap gui/501 "+plistPath(d))
	if out < 0 || boot < out {
		t.Errorf("want bootout then bootstrap: %v", f.calls)
	}
	if _, err := os.Stat(plistPath(d) + ".bak"); err == nil {
		t.Error("a managed plist must not be backed up")
	}
}

// bootout returns before the old job is gone: bootstrap must wait for it, and
// the outgoing server, still answering on its port, must not pass the health
// check for the new one.
func TestInstallServiceDarwinReloadWaitsForOldJob(t *testing.T) {
	t.Run("waits for bootout to finish", func(t *testing.T) {
		f := &sysFake{}
		d := f.deps(t, "darwin")
		f.outputs[printCmd] = "pid = 321\n"
		f.state = &serve.State{Port: 8787, PID: 321}
		f.bootPID, f.outLag = 322, 3
		if _, err := Install("service", d); err != nil {
			t.Fatal(err)
		}
		if f.outLag >= 0 {
			t.Errorf("bootstrap ran while the old job was still loaded (lag left %d)", f.outLag)
		}
	})
	t.Run("old server does not pass", func(t *testing.T) {
		f := &sysFake{}
		d := f.deps(t, "darwin")
		f.outputs[printCmd] = "pid = 321\n"
		f.state = &serve.State{Port: 8787, PID: 321}
		// bootstrap reloads the job but it still reports the old pid, which
		// still owns serve.json and answers.
		d.Run = func(name string, args ...string) error {
			c := strings.Join(append([]string{name}, args...), " ")
			f.calls = append(f.calls, c)
			if strings.HasPrefix(c, "launchctl bootstrap") {
				f.outputs[printCmd] = "pid = 321\n"
			}
			return nil
		}
		if _, err := Install("service", d); err == nil || !strings.Contains(err.Error(), "did not answer") {
			t.Fatalf("err = %v, want health-check failure", err)
		}
	})
}

func TestInstallServiceDarwinTakeoverWithExistingBackup(t *testing.T) {
	f := &sysFake{}
	d := f.deps(t, "darwin")
	f.outputs[printCmd] = "pid = 9\n"
	f.state = &serve.State{Port: 8787, PID: 9}
	f.bootPID = 10
	mustWrite(t, plistPath(d), "<plist>hand written</plist>")
	mustWrite(t, plistPath(d)+".bak", "older")
	steps, err := Install("service", d)
	if err != nil {
		t.Fatal(err)
	}
	if mustRead(t, plistPath(d)+".bak.1700000000") != "<plist>hand written</plist>" || mustRead(t, plistPath(d)+".bak") != "older" {
		t.Error("backups wrong")
	}
	if !hasSvcStep(steps, "took over existing glim.plist (backup: glim.plist.bak.1700000000)") {
		t.Errorf("steps = %v", steps)
	}
}

func TestInstallServiceDarwinHealthFailurePointsAtLog(t *testing.T) {
	f := &sysFake{}
	d := f.deps(t, "darwin")
	f.outputs[printCmd] = "pid = 9\n"
	f.state = &serve.State{Port: 8787, PID: 1} // stale
	f.alive = false
	_, err := Install("service", d)
	if err == nil || !strings.Contains(err.Error(), filepath.Join("Logs", "glim", "serve.log")) {
		t.Fatalf("err = %v", err)
	}
	if _, serr := os.Stat(plistPath(d)); serr != nil {
		t.Error("plist must stay installed")
	}
}

func TestUninstallService(t *testing.T) {
	t.Run("linux managed", func(t *testing.T) {
		f := &sysFake{}
		d := f.deps(t, "linux")
		mustWrite(t, unitPath(d), "# glim:managed — x\n")
		mustWrite(t, filepath.Join(unitPath(d)+".d", "env.conf"), "[Service]\nEnvironmentFile=/x\n")
		steps, err := Uninstall("service", d)
		if err != nil {
			t.Fatal(err)
		}
		if _, serr := os.Stat(unitPath(d)); serr == nil {
			t.Error("unit not removed")
		}
		if !f.ran("systemctl --user disable --now glim.service") || !f.ran("systemctl --user daemon-reload") {
			t.Errorf("calls = %v", f.calls)
		}
		if _, serr := os.Stat(filepath.Join(unitPath(d)+".d", "env.conf")); serr != nil {
			t.Error("drop-ins must be left alone")
		}
		if !hasSvcStep(steps, "left the drop-ins in") {
			t.Errorf("steps = %v", steps)
		}
	})
	t.Run("linux missing", func(t *testing.T) {
		f := &sysFake{}
		d := f.deps(t, "linux")
		steps, err := Uninstall("service", d)
		if err != nil || len(steps) != 1 || steps[0] != "no glim service installed" {
			t.Fatalf("steps = %v, err = %v", steps, err)
		}
		if len(f.calls) != 0 {
			t.Errorf("calls = %v", f.calls)
		}
	})
	t.Run("linux unmanaged", func(t *testing.T) {
		f := &sysFake{}
		d := f.deps(t, "linux")
		mustWrite(t, unitPath(d), "[Service]\nExecStart=/x\n")
		_, err := Uninstall("service", d)
		if err == nil || !strings.Contains(err.Error(), "glim.service was not written by glim; remove it yourself") {
			t.Fatalf("err = %v", err)
		}
		if _, serr := os.Stat(unitPath(d)); serr != nil || len(f.calls) != 0 {
			t.Error("a foreign unit must be left untouched and unstopped")
		}
	})
	t.Run("darwin managed", func(t *testing.T) {
		f := &sysFake{}
		d := f.deps(t, "darwin")
		f.runErr["launchctl bootout gui/501/glim"] = errors.New("not loaded") // ignored
		mustWrite(t, plistPath(d), "<?xml?>\n<!-- glim:managed -->\n")
		logDir := filepath.Join(d.Home, "Library", "Logs", "glim")
		mustWrite(t, filepath.Join(logDir, "serve.log"), "x")
		if _, err := Uninstall("service", d); err != nil {
			t.Fatal(err)
		}
		if _, serr := os.Stat(plistPath(d)); serr == nil {
			t.Error("plist not removed")
		}
		if _, serr := os.Stat(filepath.Join(logDir, "serve.log")); serr != nil {
			t.Error("log dir must be left")
		}
	})
	t.Run("darwin missing and unmanaged", func(t *testing.T) {
		f := &sysFake{}
		d := f.deps(t, "darwin")
		if steps, err := Uninstall("service", d); err != nil || steps[0] != "no glim service installed" {
			t.Fatalf("steps = %v err = %v", steps, err)
		}
		mustWrite(t, plistPath(d), "<plist/>")
		if _, err := Uninstall("service", d); err == nil || !strings.Contains(err.Error(), "glim.plist was not written by glim") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestServiceTargetErrors(t *testing.T) {
	f := &sysFake{}
	d := f.deps(t, "linux")
	if _, err := InstallSkill("service", d); err == nil || !strings.Contains(err.Error(), "--skill") {
		t.Errorf("--skill service: err = %v", err)
	}
	d.Service.OS = "windows"
	if _, err := Install("service", d); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("unsupported OS: err = %v", err)
	}
}

func TestUnitQuoting(t *testing.T) {
	u := unitFile(`/opt/my glim/100%/glim$x`, `/a b:/c"d`)
	for _, want := range []string{`ExecStart="/opt/my glim/100%%/glim$x" serve`, `Environment="PATH=/a b:/c\"d"`} {
		if !strings.Contains(u, want) {
			t.Errorf("missing %q in\n%s", want, u)
		}
	}
}

func TestPlistEscapesXML(t *testing.T) {
	p := plistFile("/opt/a&b/glim", "/x:<y>", "/log")
	if !strings.Contains(p, "/opt/a&amp;b/glim") || !strings.Contains(p, "/x:&lt;y&gt;") {
		t.Errorf("not escaped:\n%s", p)
	}
}
