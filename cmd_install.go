package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/config"
	"github.com/samuelloranger/glim/internal/install"
	"github.com/samuelloranger/glim/internal/serve"
)

func cmdInstall(args []string) error {
	skill := false
	var targets []string
	for _, a := range args {
		if a == "--skill" {
			skill = true
		} else {
			targets = append(targets, a)
		}
	}
	if len(targets) != 1 {
		return fmt.Errorf("usage: glim install [--skill] <claude|codex|cursor|service>")
	}
	d, err := installDeps()
	if err != nil {
		return err
	}
	var steps []string
	if skill {
		steps, err = install.InstallSkill(targets[0], d)
	} else {
		steps, err = install.Install(targets[0], d)
	}
	printSteps(steps)
	return err
}

func cmdUninstall(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: glim uninstall <claude|codex|cursor|service>")
	}
	d, err := installDeps()
	if err != nil {
		return err
	}
	steps, err := install.Uninstall(args[0], d)
	printSteps(steps)
	return err
}

// printSteps prints one line per step: ✓ for what was done, ! for a
// "warning: " or "note: " the user should read.
func printSteps(steps []string) {
	for _, s := range steps {
		if rest, ok := strings.CutPrefix(s, "warning: "); ok {
			fmt.Println("!", rest)
		} else if rest, ok := strings.CutPrefix(s, "note: "); ok {
			fmt.Println("!", rest)
		} else {
			fmt.Println("✓", s)
		}
	}
}

// serviceBinPath is the absolute path to run as the service. On Linux
// os.Executable resolves symlinks, so a stable symlink the user ran glim
// through (a Homebrew bin entry, ~/.local/bin/glim) is recovered from argv[0]
// when it points at the same binary; it then survives upgrades.
func serviceBinPath(self string) string {
	arg := os.Args[0]
	if !strings.Contains(arg, "/") {
		p, err := exec.LookPath(arg)
		if err != nil {
			return self
		}
		arg = p
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return self
	}
	a, errA := filepath.EvalSymlinks(abs)
	b, errB := filepath.EvalSymlinks(self)
	if errA != nil || errB != nil || a != b {
		return self
	}
	return abs
}

// currentUser is the login name for loginctl: $USER, else the account
// database, since $USER is unset in some non-login sessions.
func currentUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

func installDeps() (install.Deps, error) {
	self, err := os.Executable()
	if err != nil {
		return install.Deps{}, err
	}
	return install.Deps{
		Home:       homeDir(),
		GlimPath:   self,
		HasCommand: func(name string) bool { _, e := exec.LookPath(name); return e == nil },
		Run:        install.DefaultRun,
		Service: install.ServiceDeps{
			OS:       runtime.GOOS,
			UID:      os.Getuid(),
			User:     currentUser(),
			BinPath:  serviceBinPath(self),
			Path:     os.Getenv("PATH"),
			TempDirs: []string{os.TempDir()},
			Output: func(name string, args ...string) (string, error) {
				out, err := exec.Command(name, args...).Output()
				return string(out), err
			},
			State:    serve.ReadState,
			PidAlive: pidAlive,
			Probe:    serve.Probe,
			Domain:   config.Load().Domain,
			HasAccounts: func() (bool, error) {
				db, err := auth.Open(config.DBPath())
				if err != nil {
					return false, err
				}
				defer db.Close()
				n, err := db.CountUsers(context.Background())
				return n > 0, err
			},
		},
	}, nil
}
