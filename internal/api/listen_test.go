package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/listen"
)

// silentMP3 is two MPEG-1 layer III frames, the shape a speech API returns.
func silentMP3() []byte {
	var b []byte
	for i := 0; i < 2; i++ {
		f := make([]byte, 417)
		copy(f, []byte{0xFF, 0xFB, 0x90, 0x00})
		b = append(b, f...)
	}
	return b
}

// fakeProviders stands in for an OpenAI-compatible model and speech API.
func fakeProviders(t *testing.T) (llm, tts *httptest.Server) {
	llm = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"language\":\"en\",\"summary\":\"A page.\",\"narration\":\"This page says hello.\"}"}}]}`)
	}))
	tts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Input string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Input != "This page says hello." {
			http.Error(w, `{"error":"unexpected input"}`, 400)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write(silentMP3())
	}))
	t.Cleanup(llm.Close)
	t.Cleanup(tts.Close)
	return llm, tts
}

func newListenEnv(t *testing.T) *env {
	llm, tts := fakeProviders(t)
	return newEnvWith(t, func(d *Deps) {
		d.Listen = listen.New(d.Store, listen.Settings{
			LLM: listen.Endpoint{URL: llm.URL + "/v1", Model: "vision"},
			TTS: listen.Endpoint{URL: tts.URL + "/v1", Model: "speech"},
		}, t.Logf)
	})
}

func waitReady(t *testing.T, get func() map[string]any) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := get(); st["state"] != "working" {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("never finished")
	return nil
}

func TestListenFromDashboard(t *testing.T) {
	e := newListenEnv(t)
	sess := e.signIn("sam@example.com")
	name := publishPreview(t, e.st, "Hello", time.Hour)
	base := "/_glim/api/previews/" + name + "/listen"

	feat := jsonBody(t, e.do(http.MethodGet, "/_glim/api/session", nil, &sess, nil))["features"].(map[string]any)
	if feat["listen"] != true || feat["speech"] != true {
		t.Fatalf("features = %v", feat)
	}
	if st := jsonBody(t, e.do(http.MethodGet, base, nil, &sess, nil)); st["state"] != "none" {
		t.Fatalf("before = %v", st)
	}
	if rec := e.do(http.MethodPost, base, nil, &sess, map[string]string{"X-Glim-CSRF": ""}); rec.Code != 403 {
		t.Fatalf("POST without CSRF = %d", rec.Code)
	}
	if rec := e.do(http.MethodPost, base, nil, &sess, nil); rec.Code != 202 {
		t.Fatalf("start = %d %s", rec.Code, rec.Body)
	}
	st := waitReady(t, func() map[string]any { return jsonBody(t, e.do(http.MethodGet, base, nil, &sess, nil)) })
	res := st["result"].(map[string]any)
	if st["state"] != "ready" || res["summary"] != "A page." || res["audio"] != true || res["model"] != "vision" {
		t.Fatalf("after = %v", st)
	}

	rec := e.do(http.MethodGet, base+"/audio", nil, &sess, nil)
	// The speech API's frames, behind the Xing frame glim adds.
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "audio/mpeg" ||
		!bytes.HasSuffix(rec.Body.Bytes(), silentMP3()) || !bytes.Contains(rec.Body.Bytes(), []byte("Xing")) {
		t.Fatalf("audio = %d %q, %d bytes", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
	}
	if rec := e.do(http.MethodGet, base+"/audio", nil, &sess, map[string]string{"Range": "bytes=0-99"}); rec.Code != 206 || rec.Body.Len() != 100 {
		t.Fatalf("range = %d, %d bytes", rec.Code, rec.Body.Len())
	}
	if rec := e.do(http.MethodGet, base+"/audio", nil, nil, nil); rec.Code != 401 {
		t.Fatalf("audio without a session = %d", rec.Code)
	}
}

func TestListenFromBar(t *testing.T) {
	e := newListenEnv(t)
	name := publishPreview(t, e.st, "Hello", time.Hour)
	tok, _ := e.barToken(name)
	base := "/_glim/api/bar/" + name + "/listen"

	pre := e.do("OPTIONS", base, nil, nil, map[string]string{"Origin": "null", "Access-Control-Request-Method": "POST"})
	if pre.Code != 204 || pre.Header().Get("Access-Control-Allow-Origin") != "null" {
		t.Fatalf("preflight = %d %v", pre.Code, pre.Header())
	}
	if r := e.bar("POST", base, tok, nil); r["status"] != float64(202) {
		t.Fatalf("start = %v", r)
	}
	st := waitReady(t, func() map[string]any { return e.bar("GET", base, tok, nil) })
	if st["state"] != "ready" {
		t.Fatalf("after = %v", st)
	}
	rec := e.do(http.MethodGet, base+"/audio", nil, nil, map[string]string{"Origin": "null", BarHeader: tok})
	if rec.Code != 200 || rec.Header().Get("Access-Control-Allow-Origin") != "null" || !bytes.HasSuffix(rec.Body.Bytes(), silentMP3()) {
		t.Fatalf("audio = %d %v", rec.Code, rec.Header())
	}
	if rec := e.do(http.MethodGet, base+"/audio", nil, nil, map[string]string{"Origin": "null", BarHeader: "forged"}); rec.Code != 401 {
		t.Fatalf("forged token = %d", rec.Code)
	}
	other := publishPreview(t, e.st, "Other", time.Hour)
	if rec := e.do(http.MethodGet, "/_glim/api/bar/"+other+"/listen", nil, nil, map[string]string{"Origin": "null", BarHeader: tok}); rec.Code != 401 {
		t.Fatalf("token for another preview = %d", rec.Code)
	}
}

func TestListenOff(t *testing.T) {
	e := newEnv(t)
	name := publishPreview(t, e.st, "Hello", time.Hour)
	tok, sess := e.barToken(name)
	feat := jsonBody(t, e.do(http.MethodGet, "/_glim/api/session", nil, &sess, nil))["features"].(map[string]any)
	if feat["listen"] != false || feat["speech"] != false {
		t.Fatalf("features = %v", feat)
	}
	for _, path := range []string{"/listen", "/listen/audio"} {
		rec := e.do(http.MethodGet, "/_glim/api/previews/"+name+path, nil, &sess, nil)
		if rec.Code != 404 || !strings.Contains(rec.Body.String(), "isn't set up") {
			t.Fatalf("%s = %d %s", path, rec.Code, rec.Body)
		}
	}
	if r := e.bar("POST", "/_glim/api/bar/"+name+"/listen", tok, nil); r["status"] != float64(404) {
		t.Fatalf("bar start = %v", r)
	}
}
