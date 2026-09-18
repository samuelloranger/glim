package serve

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func Handler(root string) http.Handler {
	fs := http.FileServer(noListFS{http.Dir(root)})
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.NotFound(w, r)
			return
		}
		fs.ServeHTTP(w, r)
	})
	return mux
}

func Serve(root, bind string, port int) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if bind == "" {
		bind = "127.0.0.1"
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", bind, port))
	if err != nil {
		return err
	}
	actual := ln.Addr().(*net.TCPAddr).Port
	_ = WriteState(State{Port: actual, PID: os.Getpid(), Root: root})
	fmt.Printf("glim serving %s on %s:%d\n", root, bind, actual)
	return http.Serve(ln, Handler(root))
}

func BaseURL(port int) string { return fmt.Sprintf("http://127.0.0.1:%d", port) }

type State struct {
	Port int    `json:"port"`
	PID  int    `json:"pid"`
	Root string `json:"root"`
}

func StatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".glim", "serve.json")
}

func WriteState(s State) error {
	p := StatePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func ReadState() (State, bool) {
	var s State
	data, err := os.ReadFile(StatePath())
	if err != nil {
		return s, false
	}
	if json.Unmarshal(data, &s) != nil {
		return s, false
	}
	return s, true
}

func FreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

type noListFS struct{ fs http.FileSystem }

func (n noListFS) Open(name string) (http.File, error) {
	f, err := n.fs.Open(name)
	if err != nil {
		return nil, err
	}
	s, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if s.IsDir() {
		index := strings.TrimSuffix(name, "/") + "/index.html"
		if idx, ierr := n.fs.Open(index); ierr != nil {
			f.Close()
			return nil, os.ErrNotExist
		} else {
			idx.Close()
		}
		return noReaddirFile{f}, nil
	}
	return f, nil
}

type noReaddirFile struct{ http.File }

func (noReaddirFile) Readdir(int) ([]os.FileInfo, error) { return nil, nil }
