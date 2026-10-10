package mcpserver

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
)

const (
	// maxSourceBytes caps the source text get returns.
	maxSourceBytes = 200 << 10
	// maxFileList caps the file paths listed for a directory publish.
	maxFileList = 200
)

type GetInput struct {
	Name          string `json:"name" jsonschema:"the preview slug to inspect"`
	IncludeSource bool   `json:"include_source,omitempty" jsonschema:"also return the published source text: the original file for a converted Markdown/text/JSON file, else index.html (capped at 200 KB). Images are never returned, only their name and size."`
}

type GetOutput struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Title    string `json:"title,omitempty"`
	Project  string `json:"project,omitempty"`
	Pinned   bool   `json:"pinned" jsonschema:"true when the preview never expires; expires is then omitted"`
	Expires  string `json:"expires,omitempty" jsonschema:"RFC3339 expiry time; omitted for pinned previews"`
	Locked   bool   `json:"locked,omitempty" jsonschema:"true when the preview is password-protected"`
	Views    int64  `json:"views" jsonschema:"how many times the link was opened in a browser"`
	LastSeen string `json:"lastSeen,omitempty" jsonschema:"RFC3339 time of the latest open; omitted if never opened"`
	Created  string `json:"created" jsonschema:"RFC3339 time of the latest publish"`

	SourceFile     string   `json:"sourceFile,omitempty" jsonschema:"with include_source: the file the source was read from, relative to the preview"`
	SourceSize     int64    `json:"sourceSize,omitempty" jsonschema:"with include_source: size in bytes of that file"`
	Source         string   `json:"source,omitempty" jsonschema:"with include_source: the published source text; omitted for binary files"`
	Truncated      bool     `json:"truncated,omitempty" jsonschema:"true when source was cut at 200 KB"`
	Binary         bool     `json:"binary,omitempty" jsonschema:"true when the source is binary (an image): only its name and size are returned"`
	Files          []string `json:"files,omitempty" jsonschema:"with include_source on a directory publish: the other file paths, relative, at most 200"`
	FilesTruncated bool     `json:"filesTruncated,omitempty" jsonschema:"true when more than 200 files exist and files is cut"`
}

// getPreview describes one live preview. stats (may be nil) supplies views.
func getPreview(s *store.Store, in GetInput, stats map[string]auth.ViewStat) (GetOutput, error) {
	m, ok := s.Live(in.Name)
	if !ok {
		return GetOutput{}, fmt.Errorf("no such preview: %s", in.Name)
	}
	out := GetOutput{
		Name:    m.Name,
		URL:     s.URL(m.Name),
		Title:   m.Title,
		Project: m.Project,
		Pinned:  m.Pinned,
		Locked:  m.Locked(),
		Created: m.Created.Format(time.RFC3339),
	}
	if !m.Pinned {
		out.Expires = m.Expires.Format(time.RFC3339)
	}
	if v := stats[m.Name]; v.Count > 0 {
		out.Views = v.Count
		out.LastSeen = v.LastSeen.Format(time.RFC3339)
	}
	if !in.IncludeSource {
		return out, nil
	}
	dir := s.Dir(m.Name)
	file := "index.html"
	if m.Source != "" && filepath.Base(m.Source) == m.Source {
		file = m.Source
	} else {
		out.Files, out.FilesTruncated = listFiles(dir)
	}
	out.SourceFile = file
	info, err := os.Lstat(filepath.Join(dir, file))
	if err != nil || !info.Mode().IsRegular() {
		return out, fmt.Errorf("cannot read the source of %s: %s is missing", m.Name, file)
	}
	out.SourceSize = info.Size()
	if isBinaryName(file) {
		out.Binary = true
		return out, nil
	}
	f, err := os.Open(filepath.Join(dir, file))
	if err != nil {
		return out, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxSourceBytes+1))
	if err != nil {
		return out, err
	}
	if len(data) > maxSourceBytes {
		data = data[:maxSourceBytes]
		out.Truncated = true
		// Do not end on half a rune.
		for len(data) > 0 && !utf8.Valid(data) {
			data = data[:len(data)-1]
		}
	}
	out.Source = string(data)
	return out, nil
}

func isBinaryName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".svg":
		return true
	}
	return false
}

// listFiles returns the regular files of a preview other than index.html and
// the manifest, as sorted slash-separated relative paths, capped at maxFileList.
func listFiles(dir string) (files []string, truncated bool) {
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil || rel == "index.html" || rel == store.ManifestFile || strings.HasPrefix(rel, store.ManifestFile+".tmp-") {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	if len(files) > maxFileList {
		files, truncated = files[:maxFileList], true
	}
	return files, truncated
}

// presentSummary is the second line of present's text: the slug and expiry,
// for clients that read only text.
func presentSummary(res store.PublishResult) string {
	if res.Pinned {
		return "name: " + res.Name + " · pinned"
	}
	return "name: " + res.Name + " · expires: " + res.Expires.Format(time.RFC3339)
}

func getText(o GetOutput) string {
	text := o.Name + " — " + o.URL
	if o.Pinned {
		text += "\npinned"
	} else {
		text += "\nexpires: " + o.Expires
	}
	if o.Locked {
		text += " · locked"
	}
	text += fmt.Sprintf(" · opened %d×", o.Views)
	if o.SourceFile != "" {
		text += "\nsource: " + o.SourceFile
		switch {
		case o.Binary:
			text += fmt.Sprintf(" (binary, %d bytes)", o.SourceSize)
		case o.Truncated:
			text += " (truncated)\n\n" + o.Source
		default:
			text += "\n\n" + o.Source
		}
		if len(o.Files) > 0 {
			text += "\n\nother files:\n" + strings.Join(o.Files, "\n")
			if o.FilesTruncated {
				text += "\n…"
			}
		}
	}
	return text
}

func registerGet(server *mcp.Server, s *store.Store, mu *sync.Mutex, views func() map[string]auth.ViewStat) {
	get := func(_ context.Context, _ *mcp.CallToolRequest, in GetInput) (*mcp.CallToolResult, GetOutput, error) {
		var stats map[string]auth.ViewStat
		if views != nil {
			stats = views()
		}
		mu.Lock()
		out, err := getPreview(s, in, stats)
		mu.Unlock()
		if err != nil {
			return nil, GetOutput{}, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: getText(out)}}}, out, nil
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get",
		Description: "Inspect one live preview by its slug: title, project, link, expiry, lock state, views and times. With include_source, also read back what was published (the original Markdown/text for a converted file, else index.html; capped at 200 KB), plus the other file paths of a directory publish.",
		// Pure read: no mutation, idempotent, closed domain.
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, get)
}
