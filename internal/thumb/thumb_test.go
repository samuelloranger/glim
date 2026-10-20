package thumb

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
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
