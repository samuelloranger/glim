package render

import (
	"strings"
	"testing"
)

func TestLeadingBOMStripped(t *testing.T) {
	bom := "\xef\xbb\xbf"
	out, err := Convert("n.md", []byte(bom+"# Real Title\n\nbody\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "<h1>Real Title</h1>") || !strings.Contains(s, "<title>Real Title</title>") {
		t.Errorf("BOM broke markdown heading/title: %s", s)
	}
	out, err = Convert("d.json", []byte(bom+`{"k":1}`), "")
	if err != nil || !strings.Contains(string(out), "&#34;k&#34;: 1") {
		t.Errorf("BOM JSON: %v %s", err, out)
	}
	out, err = Convert("t.txt", []byte(bom+"hello"), "")
	if err != nil || strings.Contains(string(out), bom) {
		t.Errorf("BOM text kept: %v", err)
	}
}

func TestColonInNameIsNotAScheme(t *testing.T) {
	out, err := Convert("notes:v2.md", []byte("# x"), "")
	if err != nil || !strings.Contains(string(out), `href="notes%3Av2.md"`) {
		t.Errorf("raw link: %v %s", err, out)
	}
	out, err = Convert("chart:v2.png", nil, "")
	if err != nil || !strings.Contains(string(out), `src="chart%3Av2.png"`) {
		t.Errorf("image: %v %s", err, out)
	}
}
