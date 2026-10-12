package api

import (
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
)

func TestSnapshotMarksLockedPreviews(t *testing.T) {
	e := newEnv(t)
	open := publishPreview(t, e.st, "Open one", time.Hour)
	locked := publishPreview(t, e.st, "Locked one", time.Hour)
	if err := e.st.SetPasswordHash(locked, "$2a$hash"); err != nil {
		t.Fatal(err)
	}
	snap, err := BuildSnapshot(e.st, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, p := range snap.Previews {
		got[p.Name] = p.Locked
	}
	if got[open] || !got[locked] {
		t.Fatalf("locked flags = %v", got)
	}
}

func TestLockAndUnlockFromDashboard(t *testing.T) {
	e := newEnv(t)
	sess := e.signIn("me@example.com")
	name := publishPreview(t, e.st, "Secret", time.Hour)

	rec := e.do("POST", "/_glim/api/previews/"+name+"/lock", map[string]string{"password": "open sesame"}, &sess, nil)
	if rec.Code != 200 {
		t.Fatalf("lock = %d %s", rec.Code, rec.Body)
	}
	if got := jsonBody(t, rec)["locked"]; got != true {
		t.Fatalf("lock response locked = %v", got)
	}
	m, _ := e.st.Get(name)
	if !auth.CheckPassword(m.PasswordHash, "open sesame") {
		t.Fatal("stored hash does not match the password")
	}

	rec = e.do("POST", "/_glim/api/previews/"+name+"/unlock", nil, &sess, nil)
	if rec.Code != 200 || jsonBody(t, rec)["locked"] != false {
		t.Fatalf("unlock = %d %s", rec.Code, rec.Body)
	}
	if m, _ := e.st.Get(name); m.Locked() {
		t.Fatal("still locked after unlock")
	}
}

func TestLockRejectsBadInput(t *testing.T) {
	e := newEnv(t)
	sess := e.signIn("me@example.com")
	name := publishPreview(t, e.st, "Secret", time.Hour)
	for _, tc := range []struct {
		path string
		body any
		want int
	}{
		{"/_glim/api/previews/" + name + "/lock", map[string]string{"password": "short"}, 400},
		{"/_glim/api/previews/" + name + "/lock", map[string]string{"password": strings.Repeat("x", auth.MaxPasswordBytes+1)}, 400},
		{"/_glim/api/previews/" + name + "/lock", nil, 400},
		{"/_glim/api/previews/gone-0000/lock", map[string]string{"password": "open sesame"}, 404},
		{"/_glim/api/previews/gone-0000/unlock", nil, 404},
	} {
		if rec := e.do("POST", tc.path, tc.body, &sess, nil); rec.Code != tc.want {
			t.Errorf("POST %s %v = %d, want %d (%s)", tc.path, tc.body, rec.Code, tc.want, rec.Body)
		}
	}
	if m, _ := e.st.Get(name); m.Locked() {
		t.Fatal("a rejected lock still locked the preview")
	}
	// Without a session nothing changes.
	if rec := e.do("POST", "/_glim/api/previews/"+name+"/lock", map[string]string{"password": "open sesame"}, nil, nil); rec.Code != 401 {
		t.Fatalf("unauthenticated lock = %d", rec.Code)
	}
}
