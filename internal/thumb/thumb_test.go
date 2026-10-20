package thumb

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/listen"
	"github.com/samuelloranger/glim/internal/store"
)

func publish(t *testing.T, st *store.Store, name, html string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "p.html")
	if err := os.WriteFile(p, []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Publish(p, "", "", "", time.Hour, name); err != nil {
		t.Fatal(err)
	}
}

// service returns a Service driving the local browser, or skips the test
// when there is none that starts.
func service(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	chrome := listen.FindChrome(os.Getenv("GLIM_CHROME"))
	if chrome == "" {
		t.Skip("no Chrome or Chromium on PATH")
	}
	st := store.New(t.TempDir(), "https://glim.example.com")
	s := New(st, chrome, t.Logf)
	t.Cleanup(s.Close)
	return s, st
}

func TestOffWithoutBrowser(t *testing.T) {
	s := New(store.New(t.TempDir(), ""), "", t.Logf)
	if s.Enabled() {
		t.Fatal("enabled without a browser")
	}
	if _, err := s.Path(context.Background(), "demo-1234"); !errors.Is(err, ErrOff) {
		t.Fatalf("err = %v, want ErrOff", err)
	}
}

func TestCapturesFirstScreenAndReusesIt(t *testing.T) {
	s, st := service(t)
	// A script paints the page red, so the shot proves scripts ran.
	publish(t, st, "demo-1234", `<!doctype html><body style="margin:0">
<script>document.body.style.background = "rgb(220,0,0)"</script></body>`)
	p, err := s.Path(context.Background(), "demo-1234")
	var se *listen.StartError
	if errors.As(err, &se) {
		t.Skipf("the browser on this machine doesn't start: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	want := image.Pt(int(Width*Scale), int(Height*Scale))
	if got := img.Bounds().Size(); got != want {
		t.Errorf("size %v, want %v", got, want)
	}
	if r, g, _, _ := img.At(want.X/2, want.Y/2).RGBA(); r>>8 < 180 || g>>8 > 60 {
		t.Errorf("centre pixel is not red: r=%d g=%d", r>>8, g>>8)
	}

	// The second request reads the file instead of rendering again.
	stat, _ := os.Stat(p)
	again, err := s.Path(context.Background(), "demo-1234")
	if err != nil || again != p {
		t.Fatalf("again = %q, %v", again, err)
	}
	if st2, _ := os.Stat(again); !st2.ModTime().Equal(stat.ModTime()) {
		t.Error("thumbnail was taken again")
	}

	// A republish gets a new thumbnail under the new version.
	publish(t, st, "demo-1234", `<p>v2</p>`)
	next, err := s.Path(context.Background(), "demo-1234")
	if err != nil {
		t.Fatal(err)
	}
	if next == p {
		t.Error("republish reused the old thumbnail")
	}
}

func TestUnknownPreview(t *testing.T) {
	s := New(store.New(t.TempDir(), ""), "/bin/true", t.Logf)
	if _, err := s.Path(context.Background(), "nope-1234"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want not exist", err)
	}
}

// A captured page loads its own files but can't reach this machine or read
// another preview, even by trying.
func TestCapturedPageIsConfined(t *testing.T) {
	s, st := service(t)
	var hits atomic.Int32
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}))
	defer local.Close()
	publish(t, st, "other-5678", "<p>locked content</p>")
	src := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	// paint.js turns the page green; the probes turn it red if one succeeds.
	must(os.WriteFile(filepath.Join(src, "paint.js"), []byte(`document.body.style.background = "rgb(0,200,0)"`), 0o644))
	must(os.WriteFile(filepath.Join(src, "index.html"), []byte(`<!doctype html><body style="margin:0">
<script src="paint.js"></script><script>
function bad() { document.body.style.background = "rgb(220,0,0)" }
fetch(`+"`"+local.URL+"/probe`"+`).then(bad, function () {});
fetch("/other-5678/").then(function (r) { if (r.ok) bad() }, function () {});
</script></body>`), 0o644))
	if _, err := st.Publish(src, "", "", "", time.Hour, "mine-1234"); err != nil {
		t.Fatal(err)
	}
	p, err := s.Path(context.Background(), "mine-1234")
	var se *listen.StartError
	if errors.As(err, &se) {
		t.Skipf("the browser on this machine doesn't start: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, _, _ := img.At(10, 10).RGBA(); g>>8 < 150 || r>>8 > 60 {
		t.Errorf("page is not green (r=%d g=%d): its own script didn't load or a probe got through", r>>8, g>>8)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("a loopback server was reached %d times", n)
	}
}
