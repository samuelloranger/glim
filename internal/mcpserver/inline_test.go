package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readIndex(t *testing.T, root, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// isolateTmp points TMPDIR at a fresh dir so leftover staging dirs are visible.
func isolateTmp(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	t.Setenv("TMPDIR", d)
	return d
}

func assertNoLeftover(t *testing.T, d string) {
	t.Helper()
	e, err := os.ReadDir(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(e) != 0 {
		t.Fatalf("temp dir left behind: %v", e)
	}
}

func TestPresentContentFormats(t *testing.T) {
	cases := []struct{ format, content, want string }{
		{"", "<html><head><title>T</title></head><body><p>hello-html</p></body></html>", "hello-html"},
		{"html", "<p>explicit-html</p>", "explicit-html"},
		{"md", "# Heading One\n\nbody-md", "<h1"},
		{"txt", "plain <text> here", "plain &lt;text&gt; here"},
		{"json", `{"a":1}`, "&#34;a&#34;: 1"},
	}
	for _, c := range cases {
		t.Run("format_"+c.format, func(t *testing.T) {
			tmp := isolateTmp(t)
			s := newTestStore(t)
			res, err := publishPreview(s, time.Hour, PresentInput{Content: c.content, Format: c.format})
			if err != nil {
				t.Fatal(err)
			}
			if got := readIndex(t, s.Root, res.Name); !strings.Contains(got, c.want) {
				t.Fatalf("index missing %q:\n%s", c.want, got)
			}
			assertNoLeftover(t, tmp)
		})
	}
}

func TestPresentContentMarkdownHeadingBecomesTitle(t *testing.T) {
	isolateTmp(t)
	s := newTestStore(t)
	res, err := publishPreview(s, time.Hour, PresentInput{Content: "# Quarterly Report\n\ntext", Format: "md"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Name, "quarterly-report-") {
		t.Fatalf("name %q not derived from heading", res.Name)
	}
}

func TestPresentContentDefaultTitle(t *testing.T) {
	isolateTmp(t)
	s := newTestStore(t)
	res, err := publishPreview(s, time.Hour, PresentInput{Content: "<p>x</p>"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Name, "preview-") {
		t.Fatalf("name %q", res.Name)
	}
}

func TestPresentPathContentExclusive(t *testing.T) {
	s := newTestStore(t)
	for _, in := range []PresentInput{
		{},
		{Path: "/x/y.html", Content: "<p>x</p>"},
		{Path: "/x/y.html", Format: "md"},
		{Content: "x", Format: "pdf"},
	} {
		if _, err := publishPreview(s, time.Hour, in); err == nil {
			t.Errorf("%+v: want error", in)
		}
	}
	_, err := publishPreview(s, time.Hour, PresentInput{})
	if err == nil || !strings.Contains(err.Error(), "path") || !strings.Contains(err.Error(), "content") {
		t.Fatalf("err = %v", err)
	}
	_, err = publishPreview(s, time.Hour, PresentInput{Path: "a", Content: "b"})
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("err = %v", err)
	}
}

func TestPresentContentOversize(t *testing.T) {
	tmp := isolateTmp(t)
	s := newTestStore(t)
	big := strings.Repeat("a", 20<<20+1)
	_, err := publishPreview(s, time.Hour, PresentInput{Content: big})
	if err == nil || !strings.Contains(err.Error(), "20 MB") {
		t.Fatalf("err = %v", err)
	}
	assertNoLeftover(t, tmp)
}

func TestPresentContentFailedPublishCleansTemp(t *testing.T) {
	tmp := isolateTmp(t)
	s := newTestStore(t)
	if _, err := publishPreview(s, time.Hour, PresentInput{Content: "{not json", Format: "json"}); err == nil {
		t.Fatal("want invalid json error")
	}
	assertNoLeftover(t, tmp)
}

func TestPresentContentNameReuse(t *testing.T) {
	isolateTmp(t)
	s := newTestStore(t)
	first, err := publishPreview(s, time.Hour, PresentInput{Content: "<p>v1</p>", Name: "my-page"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := publishPreview(s, time.Hour, PresentInput{Content: "# v2", Format: "md", Name: "my-page"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "my-page" || second.Name != "my-page" || first.URL != second.URL {
		t.Fatalf("%+v %+v", first, second)
	}
	if got := readIndex(t, s.Root, "my-page"); !strings.Contains(got, "v2") || strings.Contains(got, "v1") {
		t.Fatalf("not replaced in place:\n%s", got)
	}
}

func TestPresentContentRepublishKeepsTitle(t *testing.T) {
	isolateTmp(t)
	s := newTestStore(t)
	if _, err := publishPreview(s, time.Hour, PresentInput{Content: "<p>v1</p>", Title: "Kept Title", Name: "kept"}); err != nil {
		t.Fatal(err)
	}
	if _, err := publishPreview(s, time.Hour, PresentInput{Content: "<p>v2</p>", Name: "kept"}); err != nil {
		t.Fatal(err)
	}
	m, err := s.Get("kept")
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "Kept Title" {
		t.Fatalf("title = %q, want the stored one kept", m.Title)
	}
}
