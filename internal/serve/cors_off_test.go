package serve

import (
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/store"
)

func TestSelfFetchCanBeDisabled(t *testing.T) {
	st := store.New(t.TempDir(), "https://glim.example.com")
	publish(t, st, "demo-1234", map[string]string{"index.html": "<h1>x</h1>", "data.json": `{"a":1}`}, time.Hour)
	h := NewRouter(Options{Store: st, NoSelfFetch: true})
	rec := corsDo(h, "GET", "/demo-1234/data.json", map[string]string{"Origin": "null"})
	if rec.Code != 200 || rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Header().Get("Vary") != "" {
		t.Errorf("GET with self-fetch off: %d %v", rec.Code, rec.Header())
	}
	rec = corsDo(h, "OPTIONS", "/demo-1234/data.json", map[string]string{"Origin": "null", "Access-Control-Request-Method": "GET"})
	if rec.Code == 204 || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("preflight with self-fetch off: %d %v", rec.Code, rec.Header())
	}
	on := corsDo(NewRouter(Options{Store: st}), "GET", "/demo-1234/data.json", map[string]string{"Origin": "null"})
	if on.Header().Get("Access-Control-Allow-Origin") != "null" {
		t.Error("self-fetch should default on")
	}
}
