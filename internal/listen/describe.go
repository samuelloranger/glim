package listen

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Script is what the model returns: a short written summary and the text to
// read aloud, in the page's language.
type Script struct {
	Language  string `json:"language"`
	Summary   string `json:"summary"`
	Narration string `json:"narration"`
	// Truncated is set when the reply hit the output limit, so the end of
	// the narration is missing.
	Truncated bool `json:"-"`
}

// DefaultMaxTokens is the output limit asked for when none is configured;
// most providers allow at least this much.
const DefaultMaxTokens = 4096

// wordBudget is the narration length that fits in maxTokens, leaving room
// for the summary and the JSON around it (about 1.5 tokens per word).
func wordBudget(maxTokens int) int {
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	return max(150, min(1500, (maxTokens-600)*2/3))
}

// Endpoint is an OpenAI-compatible API: URL is its base (ending in /v1 or
// the provider's equivalent) and Key, when set, is sent as a bearer token.
type Endpoint struct {
	URL   string
	Model string
	Key   string
}

const systemPrompt = `You make the audio version of a web page: the page itself, read aloud for someone who listens instead of reading, the way an audiobook reads a book. You also write a short summary of it.

You get the page's text and pictures of it: screenshots from top to bottom, then close-ups of its largest pictures. Take the exact words from the text, and what the text leaves out from the pictures.

Reply with one JSON object:
{"language": "<BCP 47 code of the page's main language, like en or fr-CA>",
 "summary": "<3 to 6 sentences: what the page is and its main points>",
 "narration": "<the page, read aloud>"}

The narration:
- is the page's own content, in its own words, order and language, spoken directly as the author would say it: "Revenue rose twelve percent".
- reads each heading as a short spoken line that leads into the text under it, and keeps every point, fact, number, step and conclusion.
- is plain spoken sentences: a list becomes sentences, a table becomes its important rows said in words, and symbols, units, abbreviations and links are said the way a person reads them aloud.
- gives each picture, chart, diagram or interface mockup a sentence or two, at the place it appears, for what it adds to the text: what a chart measures, its trend and its key values; what a mockup or photo shows.
- covers the page's body, from its first heading to its last word, as a condensed reading: about a third of the page's length and at most %d words. Keep the page's voice, its order, its headings and every key point, number and conclusion; tighten examples, repetition and detail.`

// Describe asks the model at e for the summary and narration of p, within
// maxTokens of output (0 selects DefaultMaxTokens). Providers differ in what
// they accept, so on a 400 or 422 it retries: with max_tokens instead of
// max_completion_tokens, then without JSON mode, without a token limit, and
// last without the images, so a text-only model still works.
func Describe(ctx context.Context, client *http.Client, e Endpoint, p Page, instructions string, maxTokens int) (Script, error) {
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	attempts := []attempt{
		{json: true, images: true, limit: "max_completion_tokens"},
		{json: true, images: true, limit: "max_tokens"},
		{json: false, images: true, limit: "max_tokens"},
		{json: false, images: true},
	}
	if len(p.Images) > 0 {
		attempts = append(attempts, attempt{})
	}
	var lastErr error
	for _, a := range attempts {
		content, finish, err := chat(ctx, client, e, request(e.Model, p, instructions, maxTokens, a))
		if err == nil {
			if finish == "length" {
				return partialScript(content)
			}
			return parseScript(content)
		}
		lastErr = err
		var he *httpError
		// Only a rejected request shape is worth another shape; anything else
		// (auth, not found, too large, rate limit, outage) fails the same way.
		if !errors.As(err, &he) || (he.status != 400 && he.status != 422) {
			return Script{}, err
		}
	}
	return Script{}, lastErr
}

// attempt is one shape of the request: JSON mode, the images, and which
// field (if any) carries the output limit.
type attempt struct {
	json, images bool
	limit        string
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
	Model               string        `json:"model"`
	Messages            []chatMessage `json:"messages"`
	Temperature         float64       `json:"temperature"`
	MaxCompletionTokens int           `json:"max_completion_tokens,omitempty"`
	MaxTokens           int           `json:"max_tokens,omitempty"`
	ResponseFormat      *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
}

func request(model string, p Page, instructions string, maxTokens int, a attempt) chatRequest {
	sys := fmt.Sprintf(systemPrompt, wordBudget(maxTokens))
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
	if a.images {
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
	if a.json {
		r.ResponseFormat = &struct {
			Type string `json:"type"`
		}{"json_object"}
	}
	switch a.limit {
	case "max_completion_tokens":
		r.MaxCompletionTokens = maxTokens
	case "max_tokens":
		r.MaxTokens = maxTokens
	}
	return r
}

type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

// chatLog, when set, receives each reply's finish reason and token counts.
var chatLog func(format string, args ...any)

func chat(ctx context.Context, client *http.Client, e Endpoint, body chatRequest) (content, finish string, err error) {
	var out struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := postJSON(ctx, client, e, "/chat/completions", body, &out); err != nil {
		return "", "", err
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", "", errors.New("The model returned an empty answer")
	}
	c := out.Choices[0]
	if chatLog != nil {
		chatLog("listen: model finished with %q, %d chars, usage %s", c.FinishReason, len(c.Message.Content), out.Usage)
	}
	return c.Message.Content, c.FinishReason, nil
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
// it, raw line breaks inside its strings, and the fields split across an
// array of objects; a reply that isn't JSON at all is used as the narration.
func parseScript(content string) (Script, error) {
	if sc, ok := jsonScript(content); ok {
		return sc, nil
	}
	s := strings.Trim(strings.TrimSpace(content), "`")
	if s == "" {
		return Script{}, errors.New("The model returned an empty answer")
	}
	return Script{Summary: firstSentences(s, 3), Narration: s}, nil
}

// jsonScript finds and decodes the model's JSON object (or array of
// objects) inside content.
func jsonScript(content string) (Script, bool) {
	s := strings.TrimSpace(content)
	var cands []string
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		cands = append(cands, s[i:j+1])
	}
	if i, j := strings.Index(s, "["), strings.LastIndex(s, "]"); i >= 0 && j > i {
		cands = append(cands, s[i:j+1])
	}
	for _, c := range cands {
		for _, b := range [][]byte{[]byte(c), escapeControls([]byte(c))} {
			if sc, ok := decodeScript(b); ok {
				return sc, true
			}
		}
	}
	return Script{}, false
}

// decodeScript reads one object, or an array of objects whose fields it
// merges (the first non-empty value of each wins).
func decodeScript(b []byte) (Script, bool) {
	var sc Script
	if json.Unmarshal(b, &sc) != nil {
		var parts []Script
		if json.Unmarshal(b, &parts) != nil {
			return Script{}, false
		}
		for _, p := range parts {
			sc.Language = cmp.Or(sc.Language, p.Language)
			sc.Summary = cmp.Or(sc.Summary, p.Summary)
			sc.Narration = cmp.Or(sc.Narration, p.Narration)
		}
	}
	sc.Narration = strings.TrimSpace(sc.Narration)
	sc.Summary = strings.TrimSpace(sc.Summary)
	sc.Language = strings.TrimSpace(sc.Language)
	if sc.Narration == "" {
		return Script{}, false
	}
	if sc.Summary == "" {
		sc.Summary = firstSentences(sc.Narration, 3)
	}
	return sc, true
}

// partialScript salvages a reply cut off by the output limit: the complete
// fields it has, and the narration up to where it stopped, ended at its last
// full sentence. It never uses the raw reply as the narration.
func partialScript(content string) (Script, error) {
	if full, ok := jsonScript(content); ok {
		full.Truncated = true
		return full, nil
	}
	sc := Script{Truncated: true}
	sc.Language, _ = jsonField(content, "language")
	sc.Summary, _ = jsonField(content, "summary")
	sc.Narration, _ = jsonField(content, "narration")
	if cut := lastSentenceEnd(sc.Narration); cut > 0 {
		sc.Narration = sc.Narration[:cut]
	}
	if sc.Narration == "" {
		return Script{}, errors.New("The model's reply hit its output limit before the narration. Raise llm_max_tokens")
	}
	if sc.Summary == "" {
		sc.Summary = firstSentences(sc.Narration, 3)
	}
	return sc, nil
}

// jsonField reads the string value of key from possibly unfinished JSON,
// skipping matches of the key's text that aren't a key; ok is false when the
// value is cut off before its closing quote.
func jsonField(s, key string) (string, bool) {
	for from := 0; ; {
		i := strings.Index(s[from:], `"`+key+`"`)
		if i < 0 {
			return "", false
		}
		i += from
		from = i + 1
		t := strings.TrimLeft(s[i+len(key)+2:], " \t\r\n")
		if !strings.HasPrefix(t, ":") {
			continue
		}
		t = strings.TrimLeft(t[1:], " \t\r\n")
		if !strings.HasPrefix(t, `"`) {
			continue
		}
		return stringValue(t[1:])
	}
}

// partialEscape matches an escape sequence cut off at the end of a string: a
// lone backslash, or \u with fewer than four hex digits.
var partialEscape = regexp.MustCompile(`(?:^|[^\\])(?:\\\\)*(\\(?:u[0-9a-fA-F]{0,3})?)$`)

// stringValue decodes a JSON string body (after its opening quote) up to its
// closing quote or, when cut off, to the end.
func stringValue(rest string) (string, bool) {
	end, closed := len(rest), false
	for k := 0; k < len(rest); k++ {
		if rest[k] == '\\' {
			k++
			continue
		}
		if rest[k] == '"' {
			end, closed = k, true
			break
		}
	}
	raw := rest[:end]
	if !closed {
		if m := partialEscape.FindStringSubmatchIndex(raw); m != nil {
			raw = raw[:m[2]]
		}
	}
	var v string
	if json.Unmarshal(escapeControls([]byte(`"`+raw+`"`)), &v) != nil {
		return "", false // never speak undecoded escapes
	}
	return strings.TrimSpace(v), closed
}

func lastSentenceEnd(s string) int {
	return strings.LastIndexAny(s, ".!?") + 1
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
