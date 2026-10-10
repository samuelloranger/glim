package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MaxInlineBytes caps inline content, the same cap as files converted to HTML.
const MaxInlineBytes = maxConvertBytes

// InlineFormats lists the formats accepted for inline content.
var InlineFormats = []string{"html", "md", "txt", "json"}

var (
	titleTagRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	mdHeadRe   = regexp.MustCompile(`(?m)^#{1,6}[ \t]+(.+?)[ \t#]*$`)
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

// InlineTitle picks a default title for content: the first <title> for html,
// the first heading for md, else "preview". When reusing an existing name it
// returns "" instead of "preview", so a republish keeps the stored title.
func InlineTitle(content, format string, reusing bool) string {
	t := ""
	switch format {
	case "html":
		if m := titleTagRe.FindStringSubmatch(content); m != nil {
			t = m[1]
		}
	case "md":
		if m := mdHeadRe.FindStringSubmatch(content); m != nil {
			t = m[1]
		}
	}
	t = strings.Join(strings.Fields(t), " ")
	if t == "" {
		if reusing {
			return ""
		}
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
