package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
)

type PresentInput struct {
	Path     string `json:"path,omitempty" jsonschema:"absolute path (preferred when the content is already a file; exactly one of path or content is required; a leading ~/ is expanded, and a relative path resolves against the directory glim mcp was started in, which may not be your current one) to a self-contained HTML file or directory (with index.html), or a .md/.txt/.log/.json/image file (converted to a styled page)"`
	Content  string `json:"content,omitempty" jsonschema:"the page itself, instead of path: publish text without writing a file first (exactly one of path or content; up to 20 MB). Interpreted per format; html must be self-contained."`
	Format   string `json:"format,omitempty" jsonschema:"how to read content: html (default), md, txt or json; md/txt/json are converted to a styled page. Only valid with content."`
	Title    string `json:"title,omitempty" jsonschema:"human title; becomes the readable link slug"`
	Project  string `json:"project,omitempty" jsonschema:"project name, stored as metadata"`
	TTL      string `json:"ttl,omitempty" jsonschema:"how long the link lives, e.g. 6h or 30m; default 6h"`
	Password string `json:"password,omitempty" jsonschema:"optional: protect the preview with this password (8-72 bytes); visitors must enter it before seeing anything. Omit when republishing with name to keep the existing password."`
	Name     string `json:"name,omitempty" jsonschema:"reuse this exact slug (e.g. one returned by an earlier present) to publish an update in place at the same URL; created if it does not exist. Omit to get a fresh random link. Lowercase letters, digits and single hyphens only."`
}

type PresentOutput struct {
	URL     string `json:"url" jsonschema:"the link to give the user"`
	Locked  bool   `json:"locked,omitempty" jsonschema:"true when the preview is password-protected"`
	Name    string `json:"name" jsonschema:"the preview slug"`
	Expires string `json:"expires" jsonschema:"RFC3339 expiry time"`
}

type ListInput struct {
	Project string `json:"project,omitempty" jsonschema:"optional: only previews with this project"`
}

type PreviewInfo struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Title    string `json:"title,omitempty"`
	Project  string `json:"project,omitempty"`
	Pinned   bool   `json:"pinned" jsonschema:"true when the preview never expires; expires is then omitted"`
	Expires  string `json:"expires,omitempty" jsonschema:"RFC3339 expiry time; omitted for pinned previews"`
	Views    int64  `json:"views" jsonschema:"how many times the link was opened in a browser (bots, the owner and dashboard thumbnails excluded)"`
	Locked   bool   `json:"locked,omitempty" jsonschema:"true when the preview is password-protected"`
	LastSeen string `json:"lastSeen,omitempty" jsonschema:"RFC3339 time of the latest open; omitted if never opened"`
}

type ListOutput struct {
	Previews []PreviewInfo `json:"previews"`
}

type RevokeInput struct {
	Name string `json:"name" jsonschema:"the preview slug to revoke"`
}

type RevokeOutput struct {
	Revoked string `json:"revoked" jsonschema:"the revoked preview slug"`
}

// listPreviews lists live previews; stats (may be nil) supplies view counts.
func listPreviews(s *store.Store, in ListInput, stats map[string]auth.ViewStat) (ListOutput, error) {
	manifests, err := s.List()
	if err != nil {
		return ListOutput{}, err
	}
	out := ListOutput{Previews: []PreviewInfo{}}
	for _, m := range manifests {
		if in.Project != "" && m.Project != in.Project {
			continue
		}
		info := PreviewInfo{
			Name:    m.Name,
			URL:     s.URL(m.Name),
			Title:   m.Title,
			Project: m.Project,
			Pinned:  m.Pinned,
			Locked:  m.Locked(),
		}
		if !m.Pinned {
			info.Expires = m.Expires.Format(time.RFC3339)
		}
		if v := stats[m.Name]; v.Count > 0 {
			info.Views = v.Count
			info.LastSeen = v.LastSeen.Format(time.RFC3339)
		}
		out.Previews = append(out.Previews, info)
	}
	return out, nil
}

// ListPreviews returns the live previews exactly as the MCP list tool does,
// so the CLI's `ls --json` shares its field names and omission rules.
func ListPreviews(s *store.Store, project string, stats map[string]auth.ViewStat) (ListOutput, error) {
	return listPreviews(s, ListInput{Project: project}, stats)
}

// NewPresentOutput converts a publish result to the shape the MCP present tool
// returns, so the CLI's `--json` output matches it.
func NewPresentOutput(res store.PublishResult) PresentOutput {
	return PresentOutput{URL: res.URL, Name: res.Name, Expires: res.Expires.Format(time.RFC3339), Locked: res.Locked}
}

func revokePreview(s *store.Store, in RevokeInput) (RevokeOutput, error) {
	if err := s.Remove(in.Name); err != nil {
		return RevokeOutput{}, err
	}
	return RevokeOutput{Revoked: in.Name}, nil
}

type PinInput struct {
	Name   string `json:"name" jsonschema:"the preview slug to pin so it never expires"`
	Pinned *bool  `json:"pinned,omitempty" jsonschema:"default true. Pass false to unpin: the preview expires again after the default lifetime"`
}

type PinOutput struct {
	Pinned   string `json:"pinned,omitempty" jsonschema:"the pinned preview slug; omitted when unpinning"`
	Unpinned string `json:"unpinned,omitempty" jsonschema:"the unpinned preview slug; omitted when pinning"`
	Expires  string `json:"expires,omitempty" jsonschema:"RFC3339 expiry time after unpinning"`
}

// pinPreview pins the preview, or unpins it when in.Pinned is false, giving it
// defaultTTL of life from now.
func pinPreview(s *store.Store, defaultTTL time.Duration, in PinInput) (PinOutput, error) {
	if in.Pinned != nil && !*in.Pinned {
		if err := s.Unpin(in.Name, defaultTTL); err != nil {
			return PinOutput{}, err
		}
		m, err := s.Get(in.Name)
		if err != nil {
			return PinOutput{}, err
		}
		return PinOutput{Unpinned: in.Name, Expires: m.Expires.Format(time.RFC3339)}, nil
	}
	if err := s.Pin(in.Name); err != nil {
		return PinOutput{}, err
	}
	return PinOutput{Pinned: in.Name}, nil
}

// resolvePath expands a leading ~/. A relative path still resolves against
// this server's working directory, which most clients set to the project, but
// that need not be the agent's current directory, so relative reports it.
func resolvePath(p string) (path string, relative bool, err error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false, fmt.Errorf("cannot expand %s: %w", p, err)
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~")), false, nil
	}
	return p, !filepath.IsAbs(p), nil
}

// presentPreview publishes in, first asking ensureBase (may be nil) for a base
// URL that serves. If that fails the preview is still published and warning
// says the returned link will not load yet.
func presentPreview(s *store.Store, mu *sync.Mutex, defaultTTL time.Duration, ensureBase func() (string, error), in PresentInput) (res store.PublishResult, warning string, err error) {
	// Starting the server can take a while; do it outside mu so list is not
	// held up behind it.
	base := ""
	if ensureBase != nil {
		var berr error
		if base, berr = ensureBase(); berr != nil {
			warning = fmt.Sprintf("Warning: could not start the local preview server (%v); this link will not load until `glim serve` is running.", berr)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if base != "" {
		s.BaseURL = base
	}
	res, err = publishPreview(s, defaultTTL, in)
	return res, warning, err
}

// presentSource resolves what in publishes: its path, or its inline content
// staged into a private temp directory that cleanup (always non-nil) removes.
// title is in.Title, or for content a default taken from the content.
func presentSource(s *store.Store, in PresentInput) (path string, relative bool, title string, cleanup func(), err error) {
	cleanup = func() {}
	title = in.Title
	switch {
	case in.Path != "" && in.Content != "":
		return "", false, "", cleanup, fmt.Errorf("pass either path or content, not both")
	case in.Path == "" && in.Content == "":
		if in.Format != "" {
			return "", false, "", cleanup, fmt.Errorf("format needs content: pass the page text in content")
		}
		return "", false, "", cleanup, fmt.Errorf("pass path (a file or directory to publish) or content (the page itself)")
	case in.Content == "":
		if in.Format != "" {
			return "", false, "", cleanup, fmt.Errorf("format only applies to content, not path")
		}
		path, relative, err = resolvePath(in.Path)
		return path, relative, title, cleanup, err
	}
	format, err := store.NormalizeInlineFormat(in.Format)
	if err != nil {
		return "", false, "", cleanup, err
	}
	if title == "" {
		stored := ""
		if in.Name != "" {
			if m, err := s.Get(in.Name); err == nil {
				stored = m.Title
			}
		}
		title = store.InlineTitle(in.Content, format, stored)
	}
	path, cleanup, err = store.StageInline(in.Content, format, title)
	return path, false, title, cleanup, err
}

// publishPreview validates the ttl, whether explicit or the resolved default
// (rejecting zero or negative lifetimes), and publishes the preview.
func publishPreview(s *store.Store, defaultTTL time.Duration, in PresentInput) (store.PublishResult, error) {
	path, relative, title, cleanup, err := presentSource(s, in)
	defer cleanup()
	if err != nil {
		return store.PublishResult{}, err
	}
	ttl := defaultTTL
	if in.TTL != "" {
		d, err := store.ParseTTL(in.TTL)
		if err != nil {
			return store.PublishResult{}, err
		}
		ttl = d
	} else if err := store.ValidateTTL(ttl); err != nil {
		return store.PublishResult{}, fmt.Errorf("default ttl: %w", err)
	}
	hash := ""
	if in.Password != "" {
		h, err := auth.HashPassword(in.Password)
		if err != nil {
			return store.PublishResult{}, passwordErr(err)
		}
		hash = h
	}
	res, err := s.PublishLocked(path, title, in.Project, "", ttl, in.Name, hash)
	if err != nil && relative {
		cwd, _ := os.Getwd()
		err = fmt.Errorf("%w (relative path resolved against %s, where glim mcp runs; pass an absolute path)", err, cwd)
	}
	return res, err
}

func passwordErr(err error) error {
	switch err {
	case auth.ErrPasswordTooShort:
		return fmt.Errorf("password must be at least %d characters", auth.MinPasswordChars)
	case auth.ErrPasswordTooLong:
		return fmt.Errorf("password must be at most %d bytes", auth.MaxPasswordBytes)
	}
	return err
}

type ExtendInput struct {
	Name string `json:"name" jsonschema:"the preview slug to extend"`
	TTL  string `json:"ttl" jsonschema:"new lifetime from now, e.g. 6h or 30m"`
}

type ExtendOutput struct {
	Name     string `json:"name"`
	Expires  string `json:"expires" jsonschema:"RFC3339 expiry time"`
	Unpinned bool   `json:"unpinned,omitempty" jsonschema:"true when the preview was pinned and extending it unpinned it"`
}

func extendPreview(s *store.Store, in ExtendInput) (ExtendOutput, error) {
	d, err := store.ParseTTL(in.TTL)
	if err != nil {
		return ExtendOutput{}, err
	}
	wasPinned := false
	if m, err := s.Get(in.Name); err == nil {
		wasPinned = m.Pinned
	}
	if err := s.Extend(in.Name, d); err != nil {
		return ExtendOutput{}, err
	}
	m, err := s.Get(in.Name)
	if err != nil {
		return ExtendOutput{}, err
	}
	return ExtendOutput{Name: in.Name, Expires: m.Expires.Format(time.RFC3339), Unpinned: wasPinned && !m.Pinned}, nil
}

// boolPtr returns a pointer to b, so that DestructiveHint/OpenWorldHint (which
// are *bool in the SDK) serialize an explicit true/false rather than being
// omitted. Note: ReadOnlyHint and IdempotentHint are plain bool with
// `omitempty`, so a false value for those is dropped from the wire regardless.
func boolPtr(b bool) *bool { return &b }

// selfFetchClause is the part of present's sandbox sentence about a page
// fetching its own files, which depends on the server's self-fetch setting.
func selfFetchClause(selfFetch bool) string {
	if selfFetch {
		return "fetch/XHR of the page's own files works (not for password-protected previews: inline the data there)"
	}
	return "fetch/XHR of the page's own files fails (inline the data)"
}

// Run serves the MCP tools over stdio. views (may be nil) returns the current
// per-preview view stats for the list tool. ensureBase (may be nil) is called
// before each present to make sure the links it returns load, returning the
// base URL to use; when it fails the preview is still published, with a
// warning in the result. selfFetch says whether the server lets preview pages
// fetch their own files, which present's description reports.
func Run(ctx context.Context, s *store.Store, defaultTTL time.Duration, version string, views func() map[string]auth.ViewStat, ensureBase func() (string, error), selfFetch bool) error {
	server := mcp.NewServer(&mcp.Implementation{Name: "glim", Version: version}, nil)

	// mu guards s.BaseURL, which a present may update while list reads it.
	var mu sync.Mutex

	present := func(_ context.Context, _ *mcp.CallToolRequest, in PresentInput) (*mcp.CallToolResult, PresentOutput, error) {
		res, warning, err := presentPreview(s, &mu, defaultTTL, ensureBase, in)
		if err != nil {
			return nil, PresentOutput{}, err
		}
		out := NewPresentOutput(res)
		text := "Preview published. Give the user this link: " + res.URL + "\n" + presentSummary(res)
		if warning != "" {
			text += "\n" + warning
		}
		result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
		return result, out, nil
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "present",
		Description: "Publish a self-contained HTML file or directory, or a Markdown, text, JSON or image file (converted to a styled page), and return a short-lived link to show the user, optionally behind a password. Pass `name` to update an existing preview in place (omit `password` to keep its current one); open tabs usually refresh by themselves. Pass exactly one of `path` (an existing file or directory) or `content` (the page text itself, no file needed; `format` html by default, or md, txt, json). Pages run in a sandbox: localStorage, sessionStorage, cookies, IndexedDB and service workers throw (use try/catch), " + selfFetchClause(selfFetch) + ", and CDN scripts, styles and images work. Use this to show any visual/HTML preview instead of other preview mechanisms.",
		// Creates a new preview each call: writes state, additive (not
		// destructive), non-idempotent, closed domain (own local store/server).
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, present)

	list := func(_ context.Context, _ *mcp.CallToolRequest, in ListInput) (*mcp.CallToolResult, ListOutput, error) {
		var stats map[string]auth.ViewStat
		if views != nil {
			stats = views()
		}
		mu.Lock()
		out, err := listPreviews(s, in, stats)
		mu.Unlock()
		if err != nil {
			return nil, ListOutput{}, err
		}
		text := fmt.Sprintf("%d live preview(s).", len(out.Previews))
		for _, p := range out.Previews {
			text += "\n" + p.Name + " — " + p.URL
			if p.Pinned {
				text += " (pinned)"
			} else {
				text += " (expires " + p.Expires + ")"
			}
			if p.Views > 0 {
				text += fmt.Sprintf(" (opened %d×, last %s)", p.Views, p.LastSeen)
			} else {
				text += " (not opened yet)"
			}
		}
		result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
		return result, out, nil
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list",
		Description: "List the currently live previews this glim instance is serving (name, link, expiry, view count and last-opened time), optionally filtered by project.",
		// Pure read: no mutation, idempotent, closed domain.
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, list)

	revoke := func(_ context.Context, _ *mcp.CallToolRequest, in RevokeInput) (*mcp.CallToolResult, RevokeOutput, error) {
		out, err := revokePreview(s, in)
		if err != nil {
			return nil, RevokeOutput{}, err
		}
		result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Revoked preview " + out.Revoked}}}
		return result, out, nil
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "revoke",
		Description: "Revoke (delete) a live preview by its slug so its link stops working immediately.",
		// Deletes a preview: destructive; idempotent (re-revoking lands the same
		// gone state); closed domain.
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, revoke)

	pin := func(_ context.Context, _ *mcp.CallToolRequest, in PinInput) (*mcp.CallToolResult, PinOutput, error) {
		out, err := pinPreview(s, defaultTTL, in)
		if err != nil {
			return nil, PinOutput{}, err
		}
		text := "Pinned preview " + out.Pinned + " (never expires)"
		if out.Unpinned != "" {
			text = "Unpinned preview " + out.Unpinned + "; it now expires " + out.Expires
		}
		result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
		return result, out, nil
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pin",
		Description: "Pin a live preview by its slug so it never expires until explicitly revoked. Pass pinned=false to unpin it, so it expires again after the default lifetime.",
		// Modifies TTL, additive (not destructive); idempotent (re-pinning lands
		// the same never-expires state); closed domain.
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, pin)

	extend := func(_ context.Context, _ *mcp.CallToolRequest, in ExtendInput) (*mcp.CallToolResult, ExtendOutput, error) {
		out, err := extendPreview(s, in)
		if err != nil {
			return nil, ExtendOutput{}, err
		}
		text := "Preview " + out.Name + " now expires " + out.Expires
		if out.Unpinned {
			text += " (it was pinned; extending unpinned it)"
		}
		result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
		return result, out, nil
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "extend",
		Description: "Extend a live preview's lifetime, setting a new TTL measured from now. On a pinned preview this unpins it: the chosen TTL replaces never-expires.",
		// Modifies TTL, additive (not destructive); NOT idempotent (each call
		// re-bases expiry on now, yielding a different expiry); closed domain.
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, extend)

	registerGet(server, s, &mu, views)

	return server.Run(ctx, &mcp.StdioTransport{})
}
