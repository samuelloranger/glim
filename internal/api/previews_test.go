package api

import (
	"image/jpeg"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/listen"
	"github.com/samuelloranger/glim/internal/qrcode"
	"github.com/samuelloranger/glim/internal/thumb"
)

func TestListPreviews(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("sam@example.com")
	publishPreview(t, e.st, "Diff review", time.Hour)
	b := jsonBody(t, e.do(http.MethodGet, "/_glim/api/previews", nil, &s, nil))
	if len(b["previews"].([]any)) != 1 || b["status"].(map[string]any)["live"] != float64(1) {
		t.Fatalf("body = %v", b)
	}
}

func TestExtendValidation(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("sam@example.com")
	name := publishPreview(t, e.st, "Ext", time.Hour)
	path := "/_glim/api/previews/" + name + "/extend"
	for _, ttl := range []string{"", "abc", "0s", "-1h", "8761h", "366d", "1.5d"} {
		if rec := e.do(http.MethodPost, path, map[string]string{"ttl": ttl}, &s, nil); rec.Code != 400 {
			t.Errorf("ttl %q = %d, want 400", ttl, rec.Code)
		}
	}
	for _, bad := range []string{"UPPER", "bad_name", "a%2Fb"} {
		rec := e.do(http.MethodPost, "/_glim/api/previews/"+bad+"/extend", map[string]string{"ttl": "1h"}, &s, nil)
		if rec.Code != 400 {
			t.Errorf("name %q = %d, want 400", bad, rec.Code)
		}
	}
	if rec := e.do(http.MethodPost, "/_glim/api/previews/nope-zzzz/extend", map[string]string{"ttl": "1h"}, &s, nil); rec.Code != 404 {
		t.Errorf("missing = %d", rec.Code)
	}
	before := time.Now()
	rec := e.do(http.MethodPost, path, map[string]string{"ttl": "48h"}, &s, nil)
	if rec.Code != 200 {
		t.Fatalf("extend = %d %s", rec.Code, rec.Body.String())
	}
	exp, _ := time.Parse(time.RFC3339Nano, jsonBody(t, rec)["expires"].(string))
	if exp.Before(before.Add(47*time.Hour)) || exp.After(time.Now().Add(49*time.Hour)) {
		t.Fatalf("expires = %v", exp)
	}
}

func TestPinAndRemove(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("sam@example.com")
	name := publishPreview(t, e.st, "Pin me", time.Hour)
	rec := e.do(http.MethodPost, "/_glim/api/previews/"+name+"/pin", nil, &s, nil)
	if rec.Code != 200 || jsonBody(t, rec)["pinned"] != true {
		t.Fatalf("pin = %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(http.MethodDelete, "/_glim/api/previews/"+name, nil, &s, nil); rec.Code != 204 {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := e.do(http.MethodDelete, "/_glim/api/previews/"+name, nil, &s, nil); rec.Code != 404 {
		t.Fatalf("delete twice = %d", rec.Code)
	}
	if rec := e.do(http.MethodDelete, "/_glim/api/previews/"+name, nil, &s, map[string]string{"X-Glim-CSRF": ""}); rec.Code != 403 {
		t.Fatalf("no csrf = %d", rec.Code)
	}
}

func TestPreviewSnapshotCarriesViews(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("sam@example.com")
	seen := publishPreview(t, e.st, "Seen", time.Hour)
	fresh := publishPreview(t, e.st, "Fresh", time.Hour)
	for i := 0; i < 3; i++ {
		if err := e.db.RecordView(t.Context(), seen); err != nil {
			t.Fatal(err)
		}
	}
	b := jsonBody(t, e.do(http.MethodGet, "/_glim/api/previews", nil, &s, nil))
	got := map[string]map[string]any{}
	for _, p := range b["previews"].([]any) {
		m := p.(map[string]any)
		got[m["name"].(string)] = m
	}
	if got[seen]["views"] != float64(3) || got[seen]["lastSeen"] == nil {
		t.Fatalf("seen = %v", got[seen])
	}
	if got[fresh]["views"] != float64(0) || got[fresh]["lastSeen"] != nil {
		t.Fatalf("fresh = %v", got[fresh])
	}
	// pin/extend responses carry the same fields
	rec := e.do(http.MethodPost, "/_glim/api/previews/"+seen+"/pin", nil, &s, nil)
	if jsonBody(t, rec)["views"] != float64(3) {
		t.Fatalf("pin response = %s", rec.Body.String())
	}
}

func TestExtendAcceptsDayUnit(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("sam@example.com")
	name := publishPreview(t, e.st, "Days", time.Hour)
	if rec := e.do(http.MethodPost, "/_glim/api/previews/"+name+"/extend", map[string]string{"ttl": "3d"}, &s, nil); rec.Code != 200 {
		t.Fatalf("ttl 3d = %d, want 200", rec.Code)
	}
}

func TestQRCodeOfAPreviewLink(t *testing.T) {
	e := newEnv(t)
	sess := e.signIn("me@example.com")
	name := publishPreview(t, e.st, "Share me", time.Hour)
	rec := e.do("GET", "/_glim/api/previews/"+name+"/qr.svg", nil, &sess, nil)
	if rec.Code != 200 {
		t.Fatalf("qr = %d %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Fatalf("content type %q", ct)
	}
	want, _ := qrcode.Encode(e.st.URL(name))
	if rec.Body.String() != want.SVG() {
		t.Fatal("body is not the QR code of the preview's link")
	}
	if rec := e.do("GET", "/_glim/api/previews/"+name+"/qr.svg", nil, nil, nil); rec.Code != 401 {
		t.Fatalf("anonymous qr = %d", rec.Code)
	}
	if rec := e.do("GET", "/_glim/api/previews/gone-0000/qr.svg", nil, &sess, nil); rec.Code != 404 {
		t.Fatalf("missing qr = %d", rec.Code)
	}
}

func TestThumbOffWithoutBrowser(t *testing.T) {
	e := newEnv(t)
	sess := e.signIn("me@example.com")
	name := publishPreview(t, e.st, "Shot", time.Hour)
	if rec := e.do("GET", "/_glim/api/previews/"+name+"/thumb.jpg", nil, &sess, nil); rec.Code != 404 {
		t.Fatalf("thumb with no service = %d", rec.Code)
	}
	if rec := e.do("GET", "/_glim/api/session", nil, &sess, nil); strings.Contains(rec.Body.String(), `"thumbs":true`) {
		t.Fatalf("session advertises thumbs: %s", rec.Body)
	}
}

func TestThumbOfAPreview(t *testing.T) {
	chrome := listen.FindChrome(os.Getenv("GLIM_CHROME"))
	if chrome == "" {
		t.Skip("no Chrome or Chromium on PATH")
	}
	var svc *thumb.Service
	e := newEnvWith(t, func(d *Deps) {
		svc = thumb.New(d.Store, chrome, t.Logf)
		d.Thumbs = svc
	})
	t.Cleanup(svc.Close)
	sess := e.signIn("me@example.com")
	name := publishPreview(t, e.st, "Shot", time.Hour)
	if rec := e.do("GET", "/_glim/api/previews/"+name+"/thumb.jpg", nil, nil, nil); rec.Code != 401 {
		t.Fatalf("anonymous thumb = %d", rec.Code)
	}
	if rec := e.do("GET", "/_glim/api/session", nil, &sess, nil); !strings.Contains(rec.Body.String(), `"thumbs":true`) {
		t.Fatalf("session doesn't advertise thumbs: %s", rec.Body)
	}
	rec := e.do("GET", "/_glim/api/previews/"+name+"/thumb.jpg", nil, &sess, nil)
	if rec.Code == http.StatusServiceUnavailable {
		t.Skipf("the browser on this machine doesn't render: %s", rec.Body)
	}
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("thumb = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("cache-control %q", cc)
	}
	if _, err := jpeg.Decode(rec.Body); err != nil {
		t.Fatal(err)
	}
}
