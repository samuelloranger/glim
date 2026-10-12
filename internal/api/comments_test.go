package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
)

// barToken signs in and mints the token the glim bar on name would carry.
func (e *env) barToken(name string) (string, auth.Session) {
	e.t.Helper()
	sess := e.signIn("sam@example.com")
	owner, err := e.db.OwnerToken(sess.User.ID, sess.Token)
	if err != nil {
		e.t.Fatal(err)
	}
	tok := e.db.BarToken(owner, name)
	if tok == "" {
		e.t.Fatal("no bar token")
	}
	return tok, sess
}

// bar sends a request as the glim bar does: from the sandboxed page's opaque
// origin, without cookies, with the token header.
func (e *env) bar(method, path, tok string, body any) map[string]any {
	e.t.Helper()
	rec := e.do(method, path, body, nil, map[string]string{"Origin": "null", BarHeader: tok})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "null" {
		e.t.Fatalf("%s %s: Access-Control-Allow-Origin = %q", method, path, got)
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		e.t.Fatalf("%s %s allows credentials", method, path)
	}
	out := map[string]any{"status": float64(rec.Code)}
	if rec.Body.Len() > 0 {
		for k, v := range jsonBody(e.t, rec) {
			out[k] = v
		}
	}
	return out
}

func TestBarCommentsRoundTrip(t *testing.T) {
	e := newEnv(t)
	name := publishPreview(t, e.st, "Page", time.Hour)
	tok, sess := e.barToken(name)
	base := "/_glim/api/bar/" + name + "/comments"

	pre := e.do("OPTIONS", base, nil, nil, map[string]string{"Origin": "null",
		"Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "content-type, x-glim-bar"})
	if pre.Code != 204 || pre.Header().Get("Access-Control-Allow-Origin") != "null" ||
		pre.Header().Get("Access-Control-Allow-Headers") != "Content-Type, X-Glim-Bar" {
		t.Fatalf("preflight = %d %v", pre.Code, pre.Header())
	}

	c := e.bar("POST", base, tok, map[string]any{"selector": "h1", "quote": "Page", "x": 0.5, "y": 0.5, "body": "Bigger title"})
	if c["status"] != float64(201) || c["body"] != "Bigger title" || c["author"] != "sam@example.com" || c["stale"] != false {
		t.Fatalf("create = %v", c)
	}
	id := strconv.Itoa(int(c["id"].(float64)))

	snap := jsonBody(t, e.do(http.MethodGet, "/_glim/api/previews", nil, &sess, nil))
	if got := snap["previews"].([]any)[0].(map[string]any)["comments"]; got != float64(1) {
		t.Fatalf("snapshot comments = %v", got)
	}

	if r := e.bar("PATCH", base+"/"+id, tok, map[string]any{"resolved": true}); r["status"] != float64(200) || r["resolved"] == nil {
		t.Fatalf("resolve = %v", r)
	}
	list := e.bar("GET", base, tok, nil)["comments"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["resolved"] == nil {
		t.Fatalf("list = %v", list)
	}

	// A republish marks the comment stale.
	publishAs(t, e.st, name)
	list = e.bar("GET", base, tok, nil)["comments"].([]any)
	if list[0].(map[string]any)["stale"] != true {
		t.Fatalf("after republish = %v", list)
	}

	if r := e.bar("DELETE", base+"/"+id, tok, nil); r["status"] != float64(204) {
		t.Fatalf("delete = %v", r)
	}
	if r := e.bar("DELETE", base+"/"+id, tok, nil); r["status"] != float64(404) {
		t.Fatalf("second delete = %v", r)
	}
}

func TestBarCommentsRefuseWithoutTheRightToken(t *testing.T) {
	e := newEnv(t)
	name := publishPreview(t, e.st, "Page", time.Hour)
	other := publishPreview(t, e.st, "Other", time.Hour)
	tok, sess := e.barToken(name)
	body := map[string]any{"body": "hi"}
	for _, tc := range []struct {
		path, tok string
		want      float64
	}{
		{"/_glim/api/bar/" + name + "/comments", "", 401},
		{"/_glim/api/bar/" + name + "/comments", "1.2.3", 401},
		{"/_glim/api/bar/" + other + "/comments", tok, 401},
		{"/_glim/api/bar/gone-0000/comments", tok, 404},
	} {
		if r := e.bar("POST", tc.path, tc.tok, body); r["status"] != tc.want {
			t.Errorf("POST %s with %q = %v, want %v", tc.path, tc.tok, r["status"], tc.want)
		}
	}
	// A comment of one preview can't be changed through another's endpoint.
	c := e.bar("POST", "/_glim/api/bar/"+name+"/comments", tok, body)
	id := strconv.Itoa(int(c["id"].(float64)))
	otherTok := e.db.BarToken(mustOwner(t, e, sess), other)
	if r := e.bar("PATCH", "/_glim/api/bar/"+other+"/comments/"+id, otherTok, map[string]any{"resolved": true}); r["status"] != float64(404) {
		t.Fatalf("cross-preview resolve = %v", r)
	}
	if r := e.bar("POST", "/_glim/api/bar/"+name+"/comments", tok, map[string]any{"body": "  "}); r["status"] != float64(400) {
		t.Fatalf("empty comment = %v", r)
	}
	// Signing out kills the token.
	if rec := e.do("POST", "/_glim/api/logout", nil, &sess, nil); rec.Code != 204 && rec.Code != 200 {
		t.Fatalf("logout = %d", rec.Code)
	}
	if r := e.bar("GET", "/_glim/api/bar/"+name+"/comments", tok, nil); r["status"] != float64(401) {
		t.Fatalf("after sign-out = %v", r)
	}
}

func mustOwner(t *testing.T, e *env, sess auth.Session) string {
	t.Helper()
	owner, err := e.db.OwnerToken(sess.User.ID, sess.Token)
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

// publishAs republishes new content in place under name.
func publishAs(t *testing.T, st *store.Store, name string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "p.html")
	if err := os.WriteFile(p, []byte("<p>v2</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Publish(p, "Page", "proj", "", time.Hour, name); err != nil {
		t.Fatal(err)
	}
}
