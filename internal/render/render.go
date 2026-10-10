// Package render converts non-HTML single files into a styled, self-contained
// index.html. Generated pages use inline CSS only and no scripts, so they work
// under the sandbox CSP previews are served with.
package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// Exts lists every extension Convert handles.
var Exts = []string{".md", ".markdown", ".txt", ".log", ".json", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".svg"}

func kind(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return "md"
	case ".txt", ".log":
		return "text"
	case ".json":
		return "json"
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".svg":
		return "image"
	}
	return ""
}

// Supported reports whether Convert can render the file name.
func Supported(name string) bool { return kind(name) != "" }

const css = `:root{color-scheme:light dark;--bg:#fff;--fg:#1f2328;--muted:#656d76;--line:#d0d7de;--code:#f6f8fa;--link:#0969da}
@media (prefers-color-scheme: dark){:root{--bg:#0d1117;--fg:#e6edf3;--muted:#8b949e;--line:#30363d;--code:#161b22;--link:#58a6ff}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.6 system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif}
main{max-width:46rem;margin:0 auto;padding:2rem 1rem 4rem}
a{color:var(--link)}
h1,h2,h3{line-height:1.25}
h1,h2{border-bottom:1px solid var(--line);padding-bottom:.3em}
pre,code{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.9em}
code{background:var(--code);padding:.15em .35em;border-radius:4px}
pre{background:var(--code);padding:1rem;border-radius:6px;overflow:auto}
pre code{background:none;padding:0}
pre.wrap{white-space:pre-wrap;overflow-wrap:anywhere}
table{border-collapse:collapse;display:block;overflow:auto}
th,td{border:1px solid var(--line);padding:.35em .75em}
blockquote{margin:0;padding:0 1em;color:var(--muted);border-left:.25em solid var(--line)}
img{max-width:100%}
hr{border:0;border-top:1px solid var(--line)}
.raw{margin-top:2rem;font-size:.8rem;color:var(--muted)}
.viewer{display:flex;flex-direction:column;align-items:center;justify-content:center;min-height:100vh;padding:1rem}
.viewer img{max-width:100%;max-height:90vh;object-fit:contain}
`

// relURL escapes a file name for use as a relative URL. PathEscape leaves ":"
// alone, so "a:b.md" would read as a URL with scheme "a:"; escaping it keeps
// the reference a path.
func relURL(name string) string {
	return strings.ReplaceAll(url.PathEscape(name), ":", "%3A")
}

func rawLink(name string) string {
	return fmt.Sprintf(`<p class="raw"><a href="%s">raw</a></p>`, html.EscapeString(relURL(name)))
}

func page(title, body string) []byte {
	return []byte("<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\">" +
		"<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">" +
		"<title>" + html.EscapeString(title) + "</title><style>" + css + "</style></head><body>" + body + "</body></html>\n")
}

// Convert renders the file called name (only its base name is used, for the
// fallback title and the raw link) with the given contents into a
// self-contained HTML page. A non-empty title overrides the derived one. For
// images data is ignored: the caller copies the image next to the page.
func Convert(name string, data []byte, title string) ([]byte, error) {
	name = filepath.Base(name)
	// A UTF-8 byte order mark would otherwise hide the first heading from the
	// Markdown parser and make encoding/json reject the document.
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	explicit := title != ""
	if !explicit {
		title = name
	}
	switch kind(name) {
	case "md":
		md := goldmark.New(goldmark.WithExtensions(extension.GFM)) // raw HTML stays disabled
		if !explicit {
			if h1 := firstH1(md, data); h1 != "" {
				title = h1
			}
		}
		var buf bytes.Buffer
		if err := md.Convert(data, &buf); err != nil {
			return nil, err
		}
		return page(title, "<main>"+buf.String()+rawLink(name)+"</main>"), nil
	case "text":
		return page(title, `<main><pre class="wrap">`+html.EscapeString(string(data))+"</pre>"+rawLink(name)+"</main>"), nil
	case "json":
		var buf bytes.Buffer
		if err := json.Indent(&buf, data, "", "  "); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return page(title, `<main><pre class="wrap">`+html.EscapeString(buf.String())+"</pre>"+rawLink(name)+"</main>"), nil
	case "image":
		src := html.EscapeString(relURL(name))
		return page(title, `<div class="viewer"><img src="`+src+`" alt="`+html.EscapeString(name)+`">`+rawLink(name)+"</div>"), nil
	}
	return nil, fmt.Errorf("unsupported file type %q", filepath.Ext(name))
}

// firstH1 returns the plain text of the first level-1 heading, or "".
// MarkdownTitle returns the text of the first level-1 heading of a Markdown
// document, parsed the way Convert parses it (so a "#" line inside a code
// block is not a heading), or "" when there is none.
func MarkdownTitle(src []byte) string {
	return firstH1(goldmark.New(goldmark.WithExtensions(extension.GFM)), bytes.TrimPrefix(src, []byte("\xef\xbb\xbf")))
}

func firstH1(md goldmark.Markdown, src []byte) string {
	doc := md.Parser().Parse(text.NewReader(src))
	var out string
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if h, ok := n.(*ast.Heading); ok && entering && h.Level == 1 {
			var sb strings.Builder
			_ = ast.Walk(h, func(c ast.Node, e bool) (ast.WalkStatus, error) {
				if t, ok := c.(*ast.Text); ok && e {
					sb.Write(t.Segment.Value(src))
				}
				return ast.WalkContinue, nil
			})
			out = strings.TrimSpace(sb.String())
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return out
}
