// Package qrcode renders a link as a QR code in SVG, for scanning a preview's
// link from a screen with a phone.
package qrcode

import (
	"fmt"
	"strings"

	"rsc.io/qr"
)

// quiet is the blank margin, in modules, that scanners need around the code.
const quiet = 4

// Code is a QR code as one SVG path: Size is the side of the square, quiet
// zone included, in modules, and Path draws every dark module in that space.
type Code struct {
	Size int    `json:"size"`
	Path string `json:"path"`
}

// Encode encodes text at the medium error-correction level.
func Encode(text string) (Code, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return Code{}, err
	}
	var b strings.Builder
	for y := 0; y < c.Size; y++ {
		for x := 0; x < c.Size; {
			if !c.Black(x, y) {
				x++
				continue
			}
			run := 1
			for x+run < c.Size && c.Black(x+run, y) {
				run++
			}
			fmt.Fprintf(&b, "M%d %dh%dv1h-%dz", x+quiet, y+quiet, run, run)
			x += run
		}
	}
	return Code{Size: c.Size + 2*quiet, Path: b.String()}, nil
}

// SVG returns a standalone SVG document of the code, dark on white whatever
// the page's colour scheme, since scanners need that contrast.
func (c Code) SVG() string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`+
		`<rect width="%d" height="%d" fill="#fff"/><path fill="#000" d="%s"/></svg>`,
		c.Size, c.Size, c.Size, c.Size, c.Path)
}
