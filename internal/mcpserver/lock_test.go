package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
)

func TestPresentPasswordParam(t *testing.T) {
	s := newTestStore(t)
	p := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(p, []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := publishPreview(s, time.Hour, PresentInput{Path: p, Name: "locked-1234", Password: "correct horse"})
	if err != nil || !res.Locked {
		t.Fatalf("present = %+v, %v", res, err)
	}
	m, _ := s.Get("locked-1234")
	if !auth.CheckPassword(m.PasswordHash, "correct horse") || strings.Contains(m.PasswordHash, "correct") {
		t.Fatalf("hash not a bcrypt of the password: %q", m.PasswordHash)
	}
	// Republish without a password keeps the lock.
	if _, err := publishPreview(s, time.Hour, PresentInput{Path: p, Name: "locked-1234"}); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.Get("locked-1234"); !m.Locked() {
		t.Fatal("republish without password dropped the lock")
	}
	out, _ := listPreviews(s, ListInput{}, nil)
	if len(out.Previews) != 1 || !out.Previews[0].Locked {
		t.Fatalf("list = %+v", out.Previews)
	}
	for _, bad := range []string{"short", strings.Repeat("x", auth.MaxPasswordBytes+1)} {
		if _, err := publishPreview(s, time.Hour, PresentInput{Path: p, Password: bad}); err == nil {
			t.Errorf("password %q should be rejected", bad)
		}
	}
}
