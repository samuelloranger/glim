// Package listen makes the audio version of a preview: it captures the page
// (its text, pictures of it and its images), asks a vision model for a
// summary and a script that describes everything on it, and has a speech
// endpoint read the script. Both endpoints speak the OpenAI API shape, so any
// provider or local server works.
package listen

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// Limits on what is sent to the model.
const (
	// MaxText caps the page text, in bytes.
	MaxText = 60_000
	// MaxImages caps the pictures per request; several providers allow 5.
	MaxImages = 5
	// maxImageBytes skips image files too large to send inline.
	maxImageBytes = 3 << 20
)

// Image is one picture sent to the model, with a label saying what it is.
type Image struct {
	Label string
	MIME  string
	Data  []byte
}

// DataURL is the image as a data: URL, the form vision APIs accept inline.
func (i Image) DataURL() string {
	return "data:" + i.MIME + ";base64," + base64.StdEncoding.EncodeToString(i.Data)
}

// Page is what the model sees of a preview.
type Page struct {
	Title  string
	Text   string
	Images []Image
	// Rendered is true when a browser rendered the page (the text is what a
	// reader sees, and the images include screenshots).
	Rendered bool
}

var imageTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".webp": "image/webp", ".gif": "image/gif",
}

// skipText are elements whose content is never read.
var skipText = map[string]bool{
	"script": true, "style": true, "noscript": true, "template": true, "head": true, "svg": true,
}

// blockTags start a new line in the extracted text.
var blockTags = map[string]bool{
	"p": true, "div": true, "section": true, "article": true, "header": true, "footer": true,
	"main": true, "aside": true, "nav": true, "li": true, "ul": true, "ol": true, "tr": true,
	"table": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"br": true, "hr": true, "pre": true, "blockquote": true, "figure": true, "figcaption": true,
	"dt": true, "dd": true, "summary": true, "details": true,
}

// StaticPage reads dir/index.html without a browser: the text as written in
// the HTML (scripts don't run) and the image files it references inside dir.
func StaticPage(dir string) (Page, error) {
	doc, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		return Page{}, err
	}
	root, err := html.Parse(bytes.NewReader(doc))
	if err != nil {
		return Page{}, err
	}
	var p Page
	var text strings.Builder
	type ref struct {
		src, alt string
	}
	var refs []ref
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if n.FirstChild != nil && p.Title == "" {
					p.Title = strings.TrimSpace(n.FirstChild.Data)
				}
				return
			case "img":
				src, alt := attr(n, "src"), attr(n, "alt")
				refs = append(refs, ref{src, alt})
				if alt != "" {
					text.WriteString(" [image: " + alt + "] ")
				}
			}
			if n.Data != "html" && skipText[n.Data] {
				if n.Data == "head" {
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.ElementNode && c.Data == "title" {
							walk(c)
						}
					}
				}
				return
			}
			if blockTags[n.Data] {
				text.WriteString("\n")
			}
		}
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && blockTags[n.Data] {
			text.WriteString("\n")
		}
	}
	walk(root)
	p.Text = tidy(text.String())

	type found struct {
		img  Image
		size int
	}
	var imgs []found
	seen := map[string]bool{}
	for _, r := range refs {
		if seen[r.src] {
			continue
		}
		seen[r.src] = true
		img, ok := loadImage(dir, r.src)
		if !ok {
			continue
		}
		img.Label = "An image on the page"
		if r.alt != "" {
			img.Label += ", alt text: " + r.alt
		}
		imgs = append(imgs, found{img, len(img.Data)})
	}
	// The largest files are the likeliest to carry content (not icons).
	sort.SliceStable(imgs, func(i, j int) bool { return imgs[i].size > imgs[j].size })
	for i := 0; i < len(imgs) && i < MaxImages; i++ {
		p.Images = append(p.Images, imgs[i].img)
	}
	return p, nil
}

// loadImage resolves src to a raster image inside dir (a relative path or a
// data: URL); anything outside dir, remote, vector or too large is skipped.
func loadImage(dir, src string) (Image, bool) {
	src = strings.TrimSpace(src)
	if rest, ok := strings.CutPrefix(src, "data:"); ok {
		meta, data, ok := strings.Cut(rest, ",")
		mime, isB64 := strings.CutSuffix(meta, ";base64")
		if !ok || !isB64 || !rasterMIME(mime) {
			return Image{}, false
		}
		b, err := base64.StdEncoding.DecodeString(data)
		if err != nil || len(b) > maxImageBytes {
			return Image{}, false
		}
		return Image{MIME: mime, Data: b}, true
	}
	if src == "" || strings.Contains(src, "://") || strings.HasPrefix(src, "//") {
		return Image{}, false
	}
	src, _, _ = strings.Cut(src, "?")
	src, _, _ = strings.Cut(src, "#")
	rel := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(src, "/")))
	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return Image{}, false
	}
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		if strings.HasPrefix(seg, ".") {
			return Image{}, false
		}
	}
	mime := imageTypes[strings.ToLower(filepath.Ext(rel))]
	if mime == "" {
		return Image{}, false
	}
	path := filepath.Join(dir, rel)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxImageBytes {
		return Image{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Image{}, false
	}
	return Image{MIME: mime, Data: b}, true
}

func rasterMIME(m string) bool {
	switch m {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// tidy collapses runs of spaces, drops blank lines beyond one, and caps the
// result at MaxText bytes on a rune boundary.
func tidy(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	blank := false
	for _, l := range lines {
		l = strings.Join(strings.Fields(l), " ")
		if l == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		blank = false
		out = append(out, l)
	}
	return capText(strings.TrimSpace(strings.Join(out, "\n")), MaxText)
}

func capText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8Start(s[cut]) {
		cut--
	}
	return s[:cut] + "\n[…the page continues]"
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
