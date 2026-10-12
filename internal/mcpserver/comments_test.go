package mcpserver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

func newCommentDB(t *testing.T) *auth.DB {
	t.Helper()
	db, err := auth.Open(filepath.Join(t.TempDir(), "glim.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.BcryptCost = bcrypt.MinCost
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCommentsToolReadsAndResolves(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	db := newCommentDB(t)
	page := publish(t, s, "Page", "", time.Hour)
	other := publish(t, s, "Other", "", time.Hour)
	m, _ := s.Get(page)
	u, _ := db.CreateUser(ctx, "sam@example.com", "correct horse battery")
	pinned, err := db.AddComment(ctx, auth.NewComment{Preview: page, Version: m.Version, UserID: u.ID,
		Selector: "main > h1", Quote: "Quarterly plan", X: 0.5, Y: 0.5, Body: "Make the title bigger"})
	if err != nil {
		t.Fatal(err)
	}
	old, _ := db.AddComment(ctx, auth.NewComment{Preview: page, Version: "older", Body: "Typo in the intro"})
	db.AddComment(ctx, auth.NewComment{Preview: other, Body: "Fine"})
	db.AddComment(ctx, auth.NewComment{Preview: "gone-0000", Body: "on a preview that no longer exists"})

	all, err := listComments(ctx, s, db, CommentsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Comments) != 3 {
		t.Fatalf("every live comment = %+v", all.Comments)
	}
	out, err := listComments(ctx, s, db, CommentsInput{Name: page})
	if err != nil || len(out.Comments) != 2 {
		t.Fatalf("page comments = %+v, %v", out, err)
	}
	c := out.Comments[0]
	if c.ID != pinned.ID || c.Selector != "main > h1" || c.Quote != "Quarterly plan" || c.Author != "sam@example.com" ||
		c.URL != s.URL(page) || c.Stale || c.Resolved {
		t.Fatalf("pinned comment = %+v", c)
	}
	if !out.Comments[1].Stale {
		t.Fatalf("a comment from another version should be stale: %+v", out.Comments[1])
	}
	text := commentsText(out)
	for _, want := range []string{"main > h1", `"Quarterly plan"`, "Make the title bigger", "(whole page)", "earlier version"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}

	// Default: resolve, with a reply.
	r, err := resolveComment(ctx, s, db, ResolveCommentInput{ID: pinned.ID, Reply: "Title is now 2.5rem."})
	if err != nil || !r.Resolved || r.Reply != "Title is now 2.5rem." {
		t.Fatalf("resolve = %+v, %v", r, err)
	}
	// resolved=false replies but leaves it open.
	no := false
	r, err = resolveComment(ctx, s, db, ResolveCommentInput{ID: old.ID, Reply: "Which paragraph?", Resolved: &no})
	if err != nil || r.Resolved || r.Reply != "Which paragraph?" {
		t.Fatalf("reply only = %+v, %v", r, err)
	}
	if out, _ := listComments(ctx, s, db, CommentsInput{Name: page}); len(out.Comments) != 1 || out.Comments[0].ID != old.ID {
		t.Fatalf("open after resolve = %+v", out.Comments)
	}
	if out, _ := listComments(ctx, s, db, CommentsInput{Name: page, IncludeResolved: true}); len(out.Comments) != 2 {
		t.Fatalf("with resolved = %+v", out.Comments)
	}
	if counts := openCommentCounts(ctx, db); counts[page] != 1 || counts[other] != 1 {
		t.Fatalf("open counts = %v", counts)
	}
	list, _ := listPreviews(s, ListInput{}, nil, openCommentCounts(ctx, db))
	for _, p := range list.Previews {
		if p.Name == page && p.Comments != 1 {
			t.Fatalf("list comments for %s = %d", page, p.Comments)
		}
	}
}

func TestCommentsToolErrors(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	db := newCommentDB(t)
	if _, err := listComments(ctx, s, nil, CommentsInput{}); err == nil || !strings.Contains(err.Error(), "dashboard database") {
		t.Fatalf("no store err = %v", err)
	}
	if _, err := resolveComment(ctx, s, nil, ResolveCommentInput{ID: 1}); err == nil {
		t.Fatal("resolve without a store should fail")
	}
	if _, err := listComments(ctx, s, db, CommentsInput{Name: "gone-0000"}); err == nil {
		t.Fatal("comments of a missing preview should fail")
	}
	if _, err := resolveComment(ctx, s, db, ResolveCommentInput{ID: 99}); err == nil {
		t.Fatal("resolving a missing comment should fail")
	}
	if openCommentCounts(ctx, nil) != nil {
		t.Fatal("counts without a store should be nil")
	}
}
