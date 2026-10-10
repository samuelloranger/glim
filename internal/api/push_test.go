package api

import "testing"

var goodSub = map[string]any{
	"endpoint": "https://push.example/abc",
	"keys":     map[string]string{"p256dh": "BPubKey_-0", "auth": "authKey_-0"},
}

func TestPushRoutesRequireSignIn(t *testing.T) {
	e := newEnv(t)
	if rec := e.do("GET", "/_glim/api/push/key", nil, nil, nil); rec.Code != 401 {
		t.Fatalf("key = %d", rec.Code)
	}
	for _, p := range []string{"subscribe", "unsubscribe"} {
		if rec := e.do("POST", "/_glim/api/push/"+p, goodSub, nil, nil); rec.Code != 401 {
			t.Fatalf("%s = %d", p, rec.Code)
		}
	}
}

func TestPushKeyIsStable(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("a@example.com")
	a := jsonBody(t, e.do("GET", "/_glim/api/push/key", nil, &s, nil))["key"]
	b := jsonBody(t, e.do("GET", "/_glim/api/push/key", nil, &s, nil))["key"]
	if a == nil || a == "" || a != b {
		t.Fatalf("keys %v %v", a, b)
	}
}

func TestPushSubscribeAndUnsubscribe(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("a@example.com")
	if rec := e.do("POST", "/_glim/api/push/subscribe", goodSub, &s, nil); rec.Code != 204 {
		t.Fatalf("subscribe = %d %s", rec.Code, rec.Body)
	}
	// Re-subscribing the same endpoint replaces rather than duplicates.
	if rec := e.do("POST", "/_glim/api/push/subscribe", goodSub, &s, nil); rec.Code != 204 {
		t.Fatalf("resubscribe = %d", rec.Code)
	}
	subs, _ := e.db.PushSubs(t.Context())
	if len(subs) != 1 || subs[0].UserID != s.User.ID {
		t.Fatalf("subs = %+v", subs)
	}
	if rec := e.do("POST", "/_glim/api/push/unsubscribe", map[string]any{"endpoint": "https://push.example/abc"}, &s, nil); rec.Code != 204 {
		t.Fatalf("unsubscribe = %d", rec.Code)
	}
	if subs, _ := e.db.PushSubs(t.Context()); len(subs) != 0 {
		t.Fatalf("subs = %+v", subs)
	}
}

func TestPushUnsubscribeOnlyOwn(t *testing.T) {
	e := newEnv(t)
	a := e.signIn("a@example.com")
	b := e.signIn("b@example.com")
	e.do("POST", "/_glim/api/push/subscribe", goodSub, &a, nil)
	e.do("POST", "/_glim/api/push/unsubscribe", map[string]any{"endpoint": "https://push.example/abc"}, &b, nil)
	if subs, _ := e.db.PushSubs(t.Context()); len(subs) != 1 {
		t.Fatalf("another user removed it: %+v", subs)
	}
}

func TestPushSubscribeValidation(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("a@example.com")
	keys := map[string]string{"p256dh": "BPub", "auth": "Auth"}
	bad := []map[string]any{
		{"endpoint": "http://push.example/abc", "keys": keys},
		{"endpoint": "https://127.0.0.1/abc", "keys": keys},
		{"endpoint": "https://localhost/abc", "keys": keys},
		{"endpoint": "https://10.0.0.5/abc", "keys": keys},
		{"endpoint": "not a url", "keys": keys},
		{"endpoint": "", "keys": keys},
		{"endpoint": "https://push.example/abc"},
		{"endpoint": "https://push.example/abc", "keys": map[string]string{"p256dh": "BPub"}},
		{"endpoint": "https://push.example/abc", "keys": map[string]string{"p256dh": "b!d", "auth": "Auth"}},
	}
	for i, b := range bad {
		if rec := e.do("POST", "/_glim/api/push/subscribe", b, &s, nil); rec.Code != 400 {
			t.Errorf("case %d: %d, want 400", i, rec.Code)
		}
	}
	if subs, _ := e.db.PushSubs(t.Context()); len(subs) != 0 {
		t.Fatalf("stored %+v", subs)
	}
}

func TestPushSubscribeNeedsCSRF(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("a@example.com")
	if rec := e.do("POST", "/_glim/api/push/subscribe", goodSub, &s, map[string]string{"X-Glim-CSRF": "wrong"}); rec.Code != 403 {
		t.Fatalf("bad csrf = %d", rec.Code)
	}
	if rec := e.do("POST", "/_glim/api/push/subscribe", goodSub, &s, map[string]string{"Origin": "https://evil.example"}); rec.Code != 403 {
		t.Fatalf("cross origin = %d", rec.Code)
	}
}
