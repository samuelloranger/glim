// Package web serves the dashboard's single-page app, built from web/ and
// embedded at compile time.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var dist embed.FS

const CSP = "default-src 'self'; frame-src 'self'; img-src 'self' data:; font-src 'self'; " +
	"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return handler(sub)
}

// handler serves index.html at "/" and the build output under "/_glim/".
func handler(files fs.FS) http.Handler {
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		panic("web: dist/index.html missing; run `bun run build` in web/")
	}
	static := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", CSP)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/" {
			h.Set("Content-Type", "text/html; charset=utf-8")
			h.Set("Cache-Control", "no-cache")
			_, _ = w.Write(index)
			return
		}
		rel, ok := strings.CutPrefix(r.URL.Path, "/_glim/")
		if !ok || rel == "" || rel == "index.html" {
			http.NotFound(w, r)
			return
		}
		if info, err := fs.Stat(files, rel); err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(rel, "assets/") {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", "no-cache")
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + rel
		static.ServeHTTP(w, r2)
	})
}
