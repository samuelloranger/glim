package qrcode

import (
	"fmt"
	"strings"
	"testing"

	"rsc.io/qr"
)

// TestPathDrawsExactlyTheDarkModules parses the path back into a grid and
// compares it with the encoder's bitmap, quiet zone included.
func TestPathDrawsExactlyTheDarkModules(t *testing.T) {
	const link = "https://glim.example.com/diff-review-a1b2/"
	got, err := Encode(link)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := qr.Encode(link, qr.M)
	if got.Size != want.Size+2*quiet {
		t.Fatalf("size = %d, want %d", got.Size, want.Size+2*quiet)
	}
	dark := make([][]bool, got.Size)
	for i := range dark {
		dark[i] = make([]bool, got.Size)
	}
	for _, seg := range strings.Split(strings.TrimSuffix(got.Path, "z"), "z") {
		var x, y, run, back int
		if _, err := fmt.Sscanf(seg, "M%d %dh%dv1h-%d", &x, &y, &run, &back); err != nil || run != back {
			t.Fatalf("segment %q: %v", seg, err)
		}
		for i := 0; i < run; i++ {
			if dark[y][x+i] {
				t.Fatalf("module %d,%d drawn twice", x+i, y)
			}
			dark[y][x+i] = true
		}
	}
	for y := 0; y < got.Size; y++ {
		for x := 0; x < got.Size; x++ {
			inner := x >= quiet && y >= quiet && x < quiet+want.Size && y < quiet+want.Size
			if exp := inner && want.Black(x-quiet, y-quiet); dark[y][x] != exp {
				t.Fatalf("module %d,%d dark = %v, want %v", x, y, dark[y][x], exp)
			}
		}
	}
}

func TestSVGIsDarkOnWhite(t *testing.T) {
	c, err := Encode("https://glim.example.com/x-0000/")
	if err != nil {
		t.Fatal(err)
	}
	svg := c.SVG()
	for _, want := range []string{
		fmt.Sprintf(`viewBox="0 0 %d %d"`, c.Size, c.Size),
		`fill="#fff"`, `fill="#000"`, c.Path,
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("svg lacks %q", want)
		}
	}
}
