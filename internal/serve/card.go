package serve

import (
	"bytes"
	"html"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/samuelloranger/glim/internal/store"
)

const (
	ogImagePath  = "/_glim/og.png"
	cardW, cardH = 1200, 630
)

var (
	ogOnce sync.Once
	ogPNG  []byte
)

// ogImage returns the fixed link-preview card, rendered once: a dark gradient
// with a soft glow and a ring, using a 256-entry palette to stay small.
func ogImage() []byte {
	ogOnce.Do(func() {
		pal := make(color.Palette, 256)
		for i := range pal {
			t := float64(i) / 255
			pal[i] = color.RGBA{
				R: uint8(14 + t*(120-14)),
				G: uint8(18 + t*(224-18)),
				B: uint8(36 + t*(200-36)),
				A: 255,
			}
		}
		img := image.NewPaletted(image.Rect(0, 0, cardW, cardH), pal)
		cx, cy := float64(cardW)/2, float64(cardH)/2
		for y := 0; y < cardH; y++ {
			for x := 0; x < cardW; x++ {
				d := math.Hypot(float64(x)-cx, float64(y)-cy)
				t := 0.10 + 0.10*float64(y)/cardH
				t += 0.25 * math.Exp(-d*d/(2*260*260))
				t += 0.55 * math.Exp(-(d-150)*(d-150)/(2*5*5))
				if t > 1 {
					t = 1
				}
				img.SetColorIndex(x, y, uint8(t*255+0.5))
			}
		}
		var buf bytes.Buffer
		_ = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img)
		ogPNG = buf.Bytes()
	})
	return ogPNG
}

// humanLeft renders a remaining lifetime such as "expires in 3 hours".
func humanLeft(d time.Duration) string {
	switch {
	case d <= 0:
		return "expires soon"
	case d < time.Minute:
		return "expires in under a minute"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minute")
	case d < 48*time.Hour:
		return plural(int(d/time.Hour), "hour")
	default:
		return plural(int(d/(24*time.Hour)), "day")
	}
}

func plural(n int, unit string) string {
	s := "expires in " + strconv.Itoa(n) + " " + unit
	if n != 1 {
		s += "s"
	}
	return s
}

// cardTags builds the Open Graph and Twitter tags for a preview, minus any the
// page already declares.
func cardTags(m store.Manifest, slug, pageURL, imageURL string, now time.Time, have map[string]bool) string {
	title := m.Title
	if title == "" {
		title = slug
	}
	life := "pinned"
	if !m.Pinned {
		life = humanLeft(m.Expires.Sub(now))
	}
	desc := life
	if m.Project != "" {
		desc = m.Project + " · " + life
	}
	tags := filterMeta([][2]string{
		{"og:title", title},
		{"og:description", desc},
		{"og:site_name", "glim"},
		{"og:type", "website"},
		{"og:url", pageURL},
		{"og:image", imageURL},
		{"twitter:card", "summary"},
	}, have)
	var sb strings.Builder
	for _, t := range tags {
		attr := "property"
		if strings.HasPrefix(t[0], "twitter:") {
			attr = "name"
		}
		sb.WriteString("<meta " + attr + `="` + t[0] + `" content="` + html.EscapeString(t[1]) + `">` + "\n")
	}
	return sb.String()
}
