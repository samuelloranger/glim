package store

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/samuelloranger/glim/internal/render"
)

// MaxInlineBytes caps inline content, the same cap as files converted to HTML.
const MaxInlineBytes = maxConvertBytes

// InlineFormats lists the formats accepted for inline content.
var InlineFormats = []string{"html", "md", "txt", "json"}

var (
	titleTagRe = regexp.MustCompile(`(?is)<title(?:\s[^>]*)?>(.*?)</title>`)
)

// NormalizeInlineFormat returns the canonical format for f, defaulting an empty
// value to html, or an error naming the accepted formats.
func NormalizeInlineFormat(f string) (string, error) {
	f = strings.ToLower(strings.TrimSpace(f))
	if f == "" {
		return "html", nil
	}
	for _, ok := range InlineFormats {
		if f == ok {
			return f, nil
		}
	}
	return "", fmt.Errorf("invalid format %q: use one of %s", f, strings.Join(InlineFormats, ", "))
}

// InlineTitle picks a title for inline content: the page's own <title> (html)
// or first level-1 heading (md); else stored, the title of the preview being
// republished; else "preview".
func InlineTitle(content, format, stored string) string {
	t := ""
	switch format {
	case "html":
		if m := titleTagRe.FindStringSubmatch(content); m != nil {
			t = html.UnescapeString(m[1])
		}
	case "md":
		t = render.MarkdownTitle([]byte(content))
	}
	t = strings.Join(strings.Fields(t), " ")
	if t == "" {
		t = stored
	}
	if t == "" {
		return "preview"
	}
	if r := []rune(t); len(r) > 80 {
		t = string(r[:80])
	}
	return t
}

// StageInline writes content to a private temporary directory as index.html
// (html) or <slug-of-title or "content">.<ext>, ready to publish like a file.
// The returned cleanup removes the directory and is safe to call on any path.
func StageInline(content, format, title string) (entry string, cleanup func(), err error) {
	noop := func() {}
	if format, err = NormalizeInlineFormat(format); err != nil {
		return "", noop, err
	}
	if int64(len(content)) > MaxInlineBytes {
		return "", noop, fmt.Errorf("content is larger than %d MB", MaxInlineBytes>>20)
	}
	dir, err := os.MkdirTemp("", "glim-inline-")
	if err != nil {
		return "", noop, err
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	file := "index.html"
	if format != "html" {
		base := Slugify(title)
		if base == "" {
			base = "content"
		}
		file = base + "." + format
	}
	entry = filepath.Join(dir, file)
	if err := os.WriteFile(entry, []byte(content), 0o600); err != nil {
		cleanup()
		return "", noop, err
	}
	return entry, cleanup, nil
}
