package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samuelloranger/glim/internal/store"
)

type PresentInput struct {
	Path    string `json:"path" jsonschema:"path to a self-contained HTML file or directory (with index.html)"`
	Title   string `json:"title,omitempty" jsonschema:"human title; becomes the readable link slug"`
	Project string `json:"project,omitempty" jsonschema:"project name, stored as metadata"`
	TTL     string `json:"ttl,omitempty" jsonschema:"how long the link lives, e.g. 6h or 30m; default 6h"`
	Name    string `json:"name,omitempty" jsonschema:"reuse this exact slug (e.g. one returned by an earlier present) to publish an update in place at the same URL; created if it does not exist. Omit to get a fresh random link. Lowercase letters, digits and single hyphens only."`
}

type PresentOutput struct {
	URL     string `json:"url" jsonschema:"the link to give the user"`
	Name    string `json:"name" jsonschema:"the preview slug"`
	Expires string `json:"expires" jsonschema:"RFC3339 expiry time"`
}

type ListInput struct {
	Project string `json:"project,omitempty" jsonschema:"optional: only previews with this project"`
}

type PreviewInfo struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Title   string `json:"title,omitempty"`
	Project string `json:"project,omitempty"`
	Expires string `json:"expires" jsonschema:"RFC3339 expiry time"`
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

func listPreviews(s *store.Store, in ListInput) (ListOutput, error) {
	manifests, err := s.List()
	if err != nil {
		return ListOutput{}, err
	}
	out := ListOutput{Previews: []PreviewInfo{}}
	for _, m := range manifests {
		if in.Project != "" && m.Project != in.Project {
			continue
		}
		out.Previews = append(out.Previews, PreviewInfo{
			Name:    m.Name,
			URL:     s.URL(m.Name),
			Title:   m.Title,
			Project: m.Project,
			Expires: m.Expires.Format(time.RFC3339),
		})
	}
	return out, nil
}

func revokePreview(s *store.Store, in RevokeInput) (RevokeOutput, error) {
	if err := s.Remove(in.Name); err != nil {
		return RevokeOutput{}, err
	}
	return RevokeOutput{Revoked: in.Name}, nil
}

type PinInput struct {
	Name string `json:"name" jsonschema:"the preview slug to pin so it never expires"`
}

type PinOutput struct {
	Pinned string `json:"pinned" jsonschema:"the pinned preview slug"`
}

func pinPreview(s *store.Store, in PinInput) (PinOutput, error) {
	if err := s.Pin(in.Name); err != nil {
		return PinOutput{}, err
	}
	return PinOutput{Pinned: in.Name}, nil
}

type ExtendInput struct {
	Name string `json:"name" jsonschema:"the preview slug to extend"`
	TTL  string `json:"ttl" jsonschema:"new lifetime from now, e.g. 6h or 30m"`
}

type ExtendOutput struct {
	Name    string `json:"name"`
	Expires string `json:"expires" jsonschema:"RFC3339 expiry time"`
}

func extendPreview(s *store.Store, in ExtendInput) (ExtendOutput, error) {
	d, err := time.ParseDuration(in.TTL)
	if err != nil {
		return ExtendOutput{}, fmt.Errorf("bad ttl %q: %w", in.TTL, err)
	}
	if err := s.Extend(in.Name, d); err != nil {
		return ExtendOutput{}, err
	}
	m, err := s.Get(in.Name)
	if err != nil {
		return ExtendOutput{}, err
	}
	return ExtendOutput{Name: in.Name, Expires: m.Expires.Format(time.RFC3339)}, nil
}

// boolPtr returns a pointer to b, so that DestructiveHint/OpenWorldHint (which
// are *bool in the SDK) serialize an explicit true/false rather than being
// omitted. Note: ReadOnlyHint and IdempotentHint are plain bool with
// `omitempty`, so a false value for those is dropped from the wire regardless.
func boolPtr(b bool) *bool { return &b }

func Run(ctx context.Context, s *store.Store, defaultTTL time.Duration, version string) error {
	server := mcp.NewServer(&mcp.Implementation{Name: "glim", Version: version}, nil)

	present := func(_ context.Context, _ *mcp.CallToolRequest, in PresentInput) (*mcp.CallToolResult, PresentOutput, error) {
		ttl := defaultTTL
		if in.TTL != "" {
			d, err := time.ParseDuration(in.TTL)
			if err != nil {
				return nil, PresentOutput{}, fmt.Errorf("bad ttl %q: %w", in.TTL, err)
			}
			ttl = d
		}
		res, err := s.Publish(in.Path, in.Title, in.Project, "", ttl, in.Name)
		if err != nil {
			return nil, PresentOutput{}, err
		}
		out := PresentOutput{URL: res.URL, Name: res.Name, Expires: res.Expires.Format(time.RFC3339)}
		result := &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Preview published. Give the user this link: " + res.URL}},
		}
		return result, out, nil
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "present",
		Description: "Publish a self-contained HTML file or directory and return a short-lived link to show the user. Use this to show any visual/HTML preview instead of other preview mechanisms.",
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
		out, err := listPreviews(s, in)
		if err != nil {
			return nil, ListOutput{}, err
		}
		text := fmt.Sprintf("%d live preview(s).", len(out.Previews))
		for _, p := range out.Previews {
			text += "\n" + p.Name + " — " + p.URL
		}
		result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
		return result, out, nil
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list",
		Description: "List the currently live previews this glim instance is serving (name, link, expiry), optionally filtered by project.",
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
		out, err := pinPreview(s, in)
		if err != nil {
			return nil, PinOutput{}, err
		}
		result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Pinned preview " + out.Pinned + " (never expires)"}}}
		return result, out, nil
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pin",
		Description: "Pin a live preview by its slug so it never expires until explicitly revoked.",
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
		result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Preview " + out.Name + " now expires " + out.Expires}}}
		return result, out, nil
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "extend",
		Description: "Extend a live preview's lifetime, setting a new TTL measured from now.",
		// Modifies TTL, additive (not destructive); NOT idempotent (each call
		// re-bases expiry on now, yielding a different expiry); closed domain.
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, extend)

	return server.Run(ctx, &mcp.StdioTransport{})
}
