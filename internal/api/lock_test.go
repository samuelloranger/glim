package api

import (
	"testing"
	"time"
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
