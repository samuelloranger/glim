package listen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Script is what the model returns: a short written summary and the text to
// read aloud, in the page's language.
type Script struct {
	Language  string `json:"language"`
	Summary   string `json:"summary"`
	Narration string `json:"narration"`
}

// Endpoint is an OpenAI-compatible API: URL is its base (ending in /v1 or
// the provider's equivalent) and Key, when set, is sent as a bearer token.
type Endpoint struct {
	URL   string
	Model string
	Key   string
}

const systemPrompt = `You turn a web page into an audio version for someone who cannot see the screen, and write a short summary of it.

You get the page's text and pictures of it: screenshots from top to bottom, then close-ups of its largest pictures. Use both: the text for exact wording and numbers, the pictures for everything visual.

Reply with one JSON object and nothing else:
{"language": "<BCP 47 code of the page's main language, like en or fr-CA>",
 "summary": "<3 to 6 sentences: what the page is and its main points>",
 "narration": "<the script read aloud>"}

The narration:
- is in the page's language, written to be heard: plain spoken sentences, no markdown, no lists with symbols, no emoji, no URLs, no code. Say symbols, units and abbreviations the way a person reads them aloud.
- opens with one sentence saying what the page is.
- then walks through the page in reading order and covers all of it: every substantive point, fact, number, step and conclusion, not just the gist.
- describes the visual side too: the layout and design (style, colours, typography, mood), and what each picture, chart, diagram, table, screenshot or interface mockup shows. For a chart, say what is measured, the trend and the notable values; for a table, read the key rows and summarize the rest. Never just say "an image".
- skips navigation chrome, repeated boilerplate and anything the page only contains for machines.
- is as long as the content needs, and no longer.`

// Describe asks the model at e for the summary and narration of p. Some
// providers reject JSON mode or images; on a 4xx it retries without JSON
// mode, then without the images, so a text-only model still works.
func Describe(ctx context.Context, client *http.Client, e Endpoint, p Page, instructions string) (Script, error) {
	type attempt struct{ json, images bool }
	attempts := []attempt{{true, true}, {false, true}}
	if len(p.Images) > 0 {
		attempts = append(attempts, attempt{false, false})
	}
	var lastErr error
	for _, a := range attempts {
		content, err := chat(ctx, client, e, request(e.Model, p, instructions, a.json, a.images))
		if err == nil {
			return parseScript(content)
		}
		lastErr = err
		var he *httpError
		if !errors.As(err, &he) || he.status < 400 || he.status >= 500 || he.status == 401 || he.status == 403 || he.status == 429 {
			return Script{}, err
		}
	}
	return Script{}, lastErr
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
}

func request(model string, p Page, instructions string, jsonMode, images bool) chatRequest {
	sys := systemPrompt
	if s := strings.TrimSpace(instructions); s != "" {
		sys += "\n\nAlso: " + s
	}
	var intro strings.Builder
	if p.Title != "" {
		fmt.Fprintf(&intro, "Page title: %s\n\n", p.Title)
	}
	if !p.Rendered {
		intro.WriteString("(The text was read from the HTML source without running its scripts.)\n\n")
	}
	if p.Text != "" {
		intro.WriteString("Page text:\n" + p.Text)
	} else {
		intro.WriteString("The page has no readable text; describe it from the pictures.")
	}
	parts := []contentPart{{Type: "text", Text: intro.String()}}
	if images {
		for _, img := range p.Images {
			parts = append(parts,
				contentPart{Type: "text", Text: img.Label + ":"},
				contentPart{Type: "image_url", ImageURL: &imageURL{URL: img.DataURL()}})
		}
	} else if len(p.Images) > 0 {
		parts = append(parts, contentPart{Type: "text", Text: "(The page's pictures could not be sent; describe what the text tells you about them.)"})
	}
	r := chatRequest{
		Model:       model,
		Temperature: 0.3,
		Messages: []chatMessage{
			{Role: "system", Content: sys},
			{Role: "user", Content: parts},
		},
	}
	if jsonMode {
		r.ResponseFormat = &struct {
			Type string `json:"type"`
		}{"json_object"}
	}
	return r
}

type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

func chat(ctx context.Context, client *http.Client, e Endpoint, body chatRequest) (string, error) {
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := postJSON(ctx, client, e, "/chat/completions", body, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", errors.New("The model returned an empty answer")
	}
	return out.Choices[0].Message.Content, nil
}

// postJSON posts body to e.URL+path and decodes a 2xx JSON reply into out
// (or, when out is a *[]byte, keeps the raw body).
func postJSON(ctx context.Context, client *http.Client, e Endpoint, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(e.URL, "/")+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.Key != "" {
		req.Header.Set("Authorization", "Bearer "+e.Key)
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Can't reach %s: %w", hostOf(e.URL), err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return &httpError{res.StatusCode, fmt.Sprintf("The server at %s answered %d: %s", hostOf(e.URL), res.StatusCode, providerMessage(data))}
	}
	if raw, ok := out.(*[]byte); ok {
		*raw = data
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("The reply from %s can't be read: %w", hostOf(e.URL), err)
	}
	return nil
}

// providerMessage pulls the error text out of an OpenAI-style error body.
func providerMessage(data []byte) string {
	var v struct {
		Error any `json:"error"`
	}
	msg := ""
	if json.Unmarshal(data, &v) == nil {
		switch x := v.Error.(type) {
		case string:
			msg = x
		case map[string]any:
			if m, ok := x["message"].(string); ok {
				msg = m
			}
		}
	}
	if msg == "" {
		msg = strings.TrimSpace(string(data))
	}
	if !utf8.ValidString(msg) {
		msg = "(unreadable error body)"
	}
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	return msg
}

// hostOf names an endpoint in messages by its host alone: never the userinfo,
// path or query, any of which may carry a credential.
func hostOf(u string) string {
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		return p.Host
	}
	return "the provider"
}

// Redact returns u without userinfo or query, for logs.
func Redact(u string) string {
	p, err := url.Parse(u)
	if err != nil || p.Host == "" {
		return hostOf(u)
	}
	p.User, p.RawQuery, p.Fragment = nil, "", ""
	return p.String()
}

// parseScript reads the model's JSON, tolerating code fences or prose around
// it; a reply that isn't JSON at all is used as the narration.
func parseScript(content string) (Script, error) {
	s := strings.TrimSpace(content)
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		var sc Script
		obj := []byte(s[i : j+1])
		err := json.Unmarshal(obj, &sc)
		if err != nil {
			// Models often put raw line breaks inside JSON strings.
			err = json.Unmarshal(escapeControls(obj), &sc)
		}
		if err == nil && strings.TrimSpace(sc.Narration) != "" {
			sc.Narration = strings.TrimSpace(sc.Narration)
			sc.Summary = strings.TrimSpace(sc.Summary)
			sc.Language = strings.TrimSpace(sc.Language)
			if sc.Summary == "" {
				sc.Summary = firstSentences(sc.Narration, 3)
			}
			return sc, nil
		}
	}
	s = strings.Trim(s, "`")
	if s == "" {
		return Script{}, errors.New("The model returned an empty answer")
	}
	return Script{Summary: firstSentences(s, 3), Narration: s}, nil
}

// escapeControls escapes raw control characters inside JSON string literals,
// leaving the rest of the document alone.
func escapeControls(b []byte) []byte {
	out := make([]byte, 0, len(b)+64)
	in, esc := false, false
	for _, c := range b {
		switch {
		case esc:
			esc = false
		case in && c == '\\':
			esc = true
		case c == '"':
			in = !in
		case in && c < 0x20:
			switch c {
			case '\n':
				out = append(out, '\\', 'n')
			case '\r':
				out = append(out, '\\', 'r')
			case '\t':
				out = append(out, '\\', 't')
			default:
				out = append(out, ' ')
			}
			continue
		}
		out = append(out, c)
	}
	return out
}

func firstSentences(s string, n int) string {
	count := 0
	for i, r := range s {
		if r == '.' || r == '!' || r == '?' {
			count++
			if count == n {
				return strings.TrimSpace(s[:i+1])
			}
		}
	}
	return strings.TrimSpace(s)
}
