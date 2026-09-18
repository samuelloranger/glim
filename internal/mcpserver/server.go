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
}

type PresentOutput struct {
	URL     string `json:"url" jsonschema:"the link to give the user"`
	Name    string `json:"name" jsonschema:"the preview slug"`
	Expires string `json:"expires" jsonschema:"RFC3339 expiry time"`
}

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
		res, err := s.Publish(in.Path, in.Title, in.Project, "", ttl)
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
	}, present)

	return server.Run(ctx, &mcp.StdioTransport{})
}
