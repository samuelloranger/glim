package render

import (
	"strings"
	"testing"
)

func TestSupported(t *testing.T) {
	for _, n := range []string{"a.md", "a.MARKDOWN", "a.txt", "a.log", "a.json", "a.png", "a.JPG", "a.jpeg", "a.gif", "a.webp", "a.avif", "a.svg"} {
		if !Supported(n) {
			t.Errorf("%s should be supported", n)
		}
	}
	for _, n := range []string{"a.html", "a.exe", "a"} {
		if Supported(n) {
			t.Errorf("%s should not be supported", n)
		}
	}
}

func TestMarkdownGFM(t *testing.T) {
	src := "# Hello\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n- [x] done\n\n~~gone~~ https://example.com\n"
	out, err := Convert("notes.md", []byte(src), "")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"<title>Hello</title>", "<table>", `type="checkbox"`, "<del>gone</del>", `<a href="https://example.com">`, "prefers-color-scheme: dark", `href="notes.md"`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output", want)
		}
	}
	if strings.Contains(s, "<script") {
		t.Error("page must not contain scripts")
	}
}

func TestMarkdownRawHTMLNotPassedThrough(t *testing.T) {
	out, err := Convert("x.md", []byte("hi <script>alert(1)</script>\n\n<div onclick=\"x()\">y</div>\n\n[l](javascript:alert(1))\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "<script>alert") || strings.Contains(s, "onclick") || strings.Contains(s, `href="javascript:`) {
		t.Errorf("raw html leaked: %s", s)
	}
}

func TestMarkdownTitleFallbacks(t *testing.T) {
	out, _ := Convert("doc.md", []byte("no heading\n"), "")
	if !strings.Contains(string(out), "<title>doc.md</title>") {
		t.Error("expected filename title")
	}
	out, _ = Convert("doc.md", []byte("# From H1\n"), "Given <&>")
	if !strings.Contains(string(out), "<title>Given &lt;&amp;&gt;</title>") {
		t.Errorf("manifest title should win and be escaped: %s", out)
	}
}

func TestTextEscaped(t *testing.T) {
	out, err := Convert("a.log", []byte("<b>&\"x\"</b>\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "&lt;b&gt;&amp;") || strings.Contains(s, "<b>&") {
		t.Errorf("text not escaped: %s", s)
	}
	if !strings.Contains(s, "pre-wrap") || !strings.Contains(s, `href="a.log"`) {
		t.Error("expected wrapping pre and raw link")
	}
}

func TestJSON(t *testing.T) {
	out, err := Convert("d.json", []byte(`{"a":"<i>","b":[1,2]}`), "")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "&lt;i&gt;") || strings.Contains(s, "<i>") {
		t.Errorf("json not escaped: %s", s)
	}
	if !strings.Contains(s, "\n  &#34;a&#34;") {
		t.Errorf("json not indented: %s", s)
	}
	if _, err := Convert("d.json", []byte(`{nope`), ""); err == nil {
		t.Error("invalid json must error")
	}
}

func TestImage(t *testing.T) {
	out, err := Convert("my pic.png", nil, "T")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `src="my%20pic.png"`) || !strings.Contains(s, "<title>T</title>") || strings.Contains(s, "<script") {
		t.Errorf("bad viewer: %s", s)
	}
}

func TestNoExternalRequests(t *testing.T) {
	out, _ := Convert("a.txt", []byte("x"), "")
	if strings.Contains(string(out), "http://") || strings.Contains(string(out), "https://") {
		t.Error("page must be self-contained")
	}
}
