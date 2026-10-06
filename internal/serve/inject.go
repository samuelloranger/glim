package serve

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
)

// maxInjectSize caps how large an HTML file is read into memory for injection.
// Larger files stream unchanged.
const maxInjectSize = 5 << 20

var (
	reBodyClose = regexp.MustCompile(`(?i)</body\s*>`)
	reHTMLOpen  = regexp.MustCompile(`(?i)<html(\s[^>]*)?>`)
	reMetaTag   = regexp.MustCompile(`(?is)<meta\s[^>]*>`)
	reMetaKey   = regexp.MustCompile(`(?is)\b(?:property|name)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>"']+))`)
)

// injectHTML inserts head just before the first real </head> (falling back to right
// after <html>, then after the doctype) and body just before the last
// </body> (falling back to the end). Empty snippets are skipped. Tag matching
// is case-insensitive.
func injectHTML(doc []byte, head, body string) []byte {
	if head != "" {
		doc = insertAt(doc, head, headPos(doc))
	}
	if body != "" {
		pos := len(doc)
		if locs := reBodyClose.FindAllIndex(doc, -1); len(locs) > 0 {
			pos = locs[len(locs)-1][0]
		}
		doc = insertAt(doc, body, pos)
	}
	return doc
}

// headPos returns where head tags go: before the first real </head> (ignoring
// comments and script/style/textarea/title raw text), else right after the
// <html> start tag, else after any BOM, whitespace, comments and doctype.
func headPos(doc []byte) int {
	htmlEnd := -1
	i := 0
	for i < len(doc) {
		j := bytes.IndexByte(doc[i:], '<')
		if j < 0 {
			break
		}
		i += j
		rest := doc[i:]
		if bytes.HasPrefix(rest, []byte("<!--")) {
			i += skipComment(rest)
			continue
		}
		if isTagAt(rest, "</head") {
			return i
		}
		if htmlEnd < 0 {
			if loc := reHTMLOpen.FindIndex(rest); loc != nil && loc[0] == 0 {
				htmlEnd = i + loc[1]
			}
		}
		if name := rawTextTag(rest); name != "" {
			i += skipRawText(rest, name)
			continue
		}
		i++
	}
	if htmlEnd >= 0 {
		return htmlEnd
	}
	return preludeEnd(doc)
}

// skipComment returns the length of the comment at the start of rest.
func skipComment(rest []byte) int {
	if k := bytes.Index(rest[4:], []byte("-->")); k >= 0 {
		return 4 + k + 3
	}
	return len(rest)
}

// isTagAt reports whether rest starts with prefix (case-insensitive) followed
// by whitespace, '>' or '/'.
func isTagAt(rest []byte, prefix string) bool {
	n := len(prefix)
	if len(rest) <= n || !strings.EqualFold(string(rest[:n]), prefix) {
		return false
	}
	switch rest[n] {
	case ' ', '\t', '\n', '\r', '\f', '>', '/':
		return true
	}
	return false
}

var rawTextTags = []string{"script", "style", "textarea", "title"}

// rawTextTag returns the element name when rest starts a raw-text element.
func rawTextTag(rest []byte) string {
	for _, n := range rawTextTags {
		if isTagAt(rest, "<"+n) {
			return n
		}
	}
	return ""
}

// skipRawText returns how far to advance past the start tag and content of a
// raw-text element, stopping at its closing tag.
func skipRawText(rest []byte, name string) int {
	close := "</" + name
	for k := 1; k < len(rest); {
		j := bytes.IndexByte(rest[k:], '<')
		if j < 0 {
			break
		}
		k += j
		if isTagAt(rest[k:], close) {
			return k
		}
		k++
	}
	return len(rest)
}

// preludeEnd returns the offset after a leading BOM, whitespace, comments and
// doctype, so a head snippet never lands in front of the doctype.
func preludeEnd(doc []byte) int {
	i := 0
	if bytes.HasPrefix(doc, []byte("\xef\xbb\xbf")) {
		i = 3
	}
	for i < len(doc) {
		switch {
		case doc[i] == ' ' || doc[i] == '\t' || doc[i] == '\n' || doc[i] == '\r' || doc[i] == '\f':
			i++
		case bytes.HasPrefix(doc[i:], []byte("<!--")):
			i += skipComment(doc[i:])
		case len(doc[i:]) >= 9 && strings.EqualFold(string(doc[i:i+9]), "<!doctype"):
			k := bytes.IndexByte(doc[i:], '>')
			if k < 0 {
				return len(doc)
			}
			i += k + 1
		default:
			return i
		}
	}
	return i
}

func insertAt(doc []byte, s string, pos int) []byte {
	out := make([]byte, 0, len(doc)+len(s))
	out = append(out, doc[:pos]...)
	out = append(out, s...)
	return append(out, doc[pos:]...)
}

// declaredMeta returns the lowercased property/name keys of every <meta> tag
// in doc, so injectors can avoid overriding what the page already declares.
func declaredMeta(doc []byte) map[string]bool {
	keys := map[string]bool{}
	for _, tag := range reMetaTag.FindAll(doc, -1) {
		for _, m := range reMetaKey.FindAllSubmatch(tag, -1) {
			for _, g := range m[1:] {
				if len(g) > 0 {
					keys[strings.ToLower(string(g))] = true
				}
			}
		}
	}
	return keys
}

// filterMeta drops entries whose key the page already declares.
func filterMeta(tags [][2]string, have map[string]bool) [][2]string {
	var out [][2]string
	for _, t := range tags {
		if !have[strings.ToLower(t[0])] {
			out = append(out, t)
		}
	}
	return out
}

// injector builds the head and body snippets to insert for a given document.
type injector func(doc []byte) (head, body string)

// serveHTMLInjected serves the named file under root-relative request r. It
// returns false when the request is not an injectable HTML file (not found, a
// directory without index.html, non-HTML, or over maxInjectSize), in which
// case nothing has been written and the caller should fall through to the
// normal file server.
func serveHTMLInjected(w http.ResponseWriter, r *http.Request, fsys http.FileSystem, inj injector) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	name := r.URL.Path
	if strings.HasSuffix(name, "/index.html") {
		return false // the file server redirects this to the directory
	}
	if strings.HasSuffix(name, "/") {
		name += "index.html"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".html") {
		return false
	}
	f, err := fsys.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() || st.Size() > maxInjectSize {
		return false
	}
	buf := make([]byte, 0, st.Size())
	tmp := make([]byte, 32*1024)
	for {
		n, rerr := f.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if rerr != nil {
			break
		}
		if len(buf) > maxInjectSize {
			return false
		}
	}
	head, body := inj(buf)
	out := injectHTML(buf, head, body)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "", st.ModTime(), bytes.NewReader(out))
	return true
}
