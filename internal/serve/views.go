package serve

import (
	"net/http"
	"strings"

	"github.com/samuelloranger/glim/internal/auth"
)

// BotAgents are lower-case substrings of User-Agents that never count as a
// view: crawlers, link unfurlers and scripted HTTP clients.
var BotAgents = []string{
	"bot", "crawler", "spider", "slurp", "facebookexternalhit", "embedly",
	"preview", "whatsapp", "telegram", "discord", "slack", "curl", "wget",
	"python-requests", "go-http-client",
}

// IsBotAgent reports whether ua looks like a bot or unfurler.
func IsBotAgent(ua string) bool {
	ua = strings.ToLower(ua)
	for _, m := range BotAgents {
		if strings.Contains(ua, m) {
			return true
		}
	}
	return false
}

// CountsAsView reports whether r is a real page open: a GET that is a
// top-level navigation (Sec-Fetch-Dest: document), which excludes iframes
// (the dashboard thumbnails) and sub-resources. Without Sec-Fetch-Dest it
// falls back to a GET whose Accept asks for text/html. Bots never count.
func CountsAsView(r *http.Request) bool {
	if r.Method != http.MethodGet || IsBotAgent(r.UserAgent()) {
		return false
	}
	if dest := r.Header.Get("Sec-Fetch-Dest"); dest != "" {
		return strings.EqualFold(dest, "document")
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html")
}

// Views records page opens, skipping the dashboard owner.
type Views struct {
	// Record stores one view of the named preview; it must not block.
	Record func(name string)
	// IsOwner validates a glim_owner cookie value; nil means nobody is owner.
	IsOwner func(token string) bool
}

func (v *Views) track(r *http.Request, name string) {
	if v == nil || v.Record == nil || !CountsAsView(r) {
		return
	}
	if v.IsOwner != nil {
		if c, err := r.Cookie(auth.OwnerCookie); err == nil && v.IsOwner(c.Value) {
			return
		}
	}
	v.Record(name)
}
