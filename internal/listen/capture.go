package listen

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// SandboxCSP is the policy previews are served with (serve.PreviewCSP), so a
// page renders for the screenshot exactly as it does for a visitor.
const SandboxCSP = "sandbox allow-scripts allow-forms allow-popups allow-modals allow-downloads"

const (
	viewW = 1280
	viewH = 900
	// tileH is the height of one screenshot at full scale; a longer page is
	// cut into up to maxTiles tiles, scaled down when it needs more.
	tileH    = 1600
	maxTiles = 3
	// figureMax is the longest side, in pixels, of an element close-up.
	figureMax = 1024
	settle    = 1500 * time.Millisecond
)

// chromeNames are tried in order. chrome-headless-shell, Chrome for Testing's
// automation build, comes first: it starts faster and uses less memory than a
// full browser. Google Chrome is next: on Ubuntu, chromium and
// chromium-browser are often stubs for a snap package that can't be driven
// headless from a service.
var chromeNames = []string{
	"chrome-headless-shell", "google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome", "headless-shell",
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/Applications/Chromium.app/Contents/MacOS/Chromium",
}

// FindChrome resolves the configured browser: "off" disables it, a path or
// name is looked up as given, and empty searches the usual names. It returns
// "" when there is none.
func FindChrome(setting string) string {
	setting = strings.TrimSpace(setting)
	if strings.EqualFold(setting, "off") {
		return ""
	}
	names := chromeNames
	if setting != "" {
		names = []string{setting}
	}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

// layout is what the page reports about itself after rendering.
type layout struct {
	Title   string   `json:"title"`
	Text    string   `json:"text"`
	Height  float64  `json:"height"`
	Figures []figure `json:"figures"`
}

type figure struct {
	X, Y, W, H float64
	Label      string `json:"label"`
}

// measureJS reads the rendered text, the page height and the largest visual
// elements (images, inline SVG, canvas, video) in page coordinates.
const measureJS = `(() => {
  const de = document.documentElement, b = document.body;
  const height = Math.max(de.scrollHeight, b ? b.scrollHeight : 0);
  const picked = [];
  for (const el of document.querySelectorAll("img, svg, canvas, video")) {
    if (el.closest("svg") && el.closest("svg") !== el) continue;
    const r = el.getBoundingClientRect();
    if (r.width < 160 || r.height < 120) continue;
    const cs = getComputedStyle(el);
    if (cs.visibility === "hidden" || cs.display === "none" || Number(cs.opacity) === 0) continue;
    let label = el.getAttribute("alt") || el.getAttribute("aria-label") || el.getAttribute("title") || "";
    const fig = el.closest("figure");
    const cap = fig && fig.querySelector("figcaption");
    if (cap && cap.innerText) label = (label ? label + " — " : "") + cap.innerText;
    picked.push({ X: r.left + scrollX, Y: r.top + scrollY, W: r.width, H: r.height,
      kind: el.localName, label: label.replace(/\s+/g, " ").trim().slice(0, 300) });
  }
  picked.sort((a, b) => b.W * b.H - a.W * a.H);
  return { title: document.title, text: b ? b.innerText : "", height, figures: picked.slice(0, 8) };
})()`

// RenderPage opens dir (a preview's folder, served as /slug/) in headless
// Chrome and captures its rendered text, screenshots from top to bottom and
// close-ups of its largest pictures, up to MaxImages pictures in all.
func RenderPage(ctx context.Context, chrome, dir, slug string) (Page, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Page{}, err
	}
	files := http.StripPrefix("/"+slug, http.FileServer(http.Dir(dir)))
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, seg := range strings.Split(r.URL.Path, "/") {
			if strings.HasPrefix(seg, ".") {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Content-Security-Policy", SandboxCSP)
		files.ServeHTTP(w, r)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chrome),
		chromedp.WindowSize(viewW, viewH),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("mute-audio", true),
	)
	if os.Geteuid() == 0 {
		// Chrome refuses to start its sandbox as root (typical in a container).
		opts = append(opts, chromedp.NoSandbox)
	}
	actx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()
	cctx, cancel := chromedp.NewContext(actx)
	defer cancel()

	url := fmt.Sprintf("http://%s/%s/", ln.Addr(), slug)
	var lay layout
	// Starting the browser is its own step, so a browser that can't start is
	// told apart from a page that fails to render.
	if err := chromedp.Run(cctx); err != nil {
		return Page{}, &StartError{Browser: chrome, Err: err}
	}
	err = chromedp.Run(cctx,
		chromedp.EmulateViewport(viewW, viewH),
		chromedp.Navigate(url),
		chromedp.Sleep(settle),
		// Scroll to the end and back so lazy images below the fold load.
		chromedp.Evaluate(`window.scrollTo(0, document.documentElement.scrollHeight)`, nil),
		chromedp.Sleep(400*time.Millisecond),
		chromedp.Evaluate(`window.scrollTo(0, 0)`, nil),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Evaluate(measureJS, &lay),
	)
	if err != nil {
		return Page{}, fmt.Errorf("render the page in %s: %w", chrome, err)
	}
	p := Page{Title: strings.TrimSpace(lay.Title), Text: tidy(lay.Text), Rendered: true}

	height := lay.Height
	if height < 1 {
		height = viewH
	}
	n := int(math.Min(maxTiles, math.Ceil(height/tileH)))
	seg := height / float64(n)
	scale := math.Min(1, tileH/seg)
	for i := 0; i < n; i++ {
		shot, err := shoot(cctx, &page.Viewport{X: 0, Y: float64(i) * seg, Width: viewW, Height: seg, Scale: scale})
		if err != nil {
			return Page{}, fmt.Errorf("screenshot the page: %w", err)
		}
		label := fmt.Sprintf("Screenshot %d of %d of the page, top to bottom", i+1, n)
		if n == 1 {
			label = "Screenshot of the whole page"
		}
		p.Images = append(p.Images, Image{Label: label, MIME: "image/jpeg", Data: shot})
	}

	figs := lay.Figures
	sort.SliceStable(figs, func(i, j int) bool { return figs[i].W*figs[i].H > figs[j].W*figs[j].H })
	for _, f := range figs {
		if len(p.Images) >= MaxImages {
			break
		}
		s := math.Min(1, figureMax/math.Max(f.W, f.H))
		shot, err := shoot(cctx, &page.Viewport{X: f.X, Y: f.Y, Width: f.W, Height: f.H, Scale: s})
		if err != nil {
			continue
		}
		label := "Close-up of a picture on the page"
		if f.Label != "" {
			label += ", labelled: " + f.Label
		}
		p.Images = append(p.Images, Image{Label: label, MIME: "image/jpeg", Data: shot})
	}
	return p, nil
}

// StartError means the browser didn't start, as opposed to a page that
// failed to render.
type StartError struct {
	Browser string
	Err     error
}

func (e *StartError) Error() string { return "start " + e.Browser + ": " + e.Err.Error() }
func (e *StartError) Unwrap() error { return e.Err }

func shoot(ctx context.Context, clip *page.Viewport) ([]byte, error) {
	var out []byte
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		b, err := page.CaptureScreenshot().
			WithFormat(page.CaptureScreenshotFormatJpeg).
			WithQuality(80).
			WithCaptureBeyondViewport(true).
			WithClip(clip).
			Do(ctx)
		out = b
		return err
	}))
	return out, err
}
