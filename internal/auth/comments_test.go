package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCommentLifecycle(t *testing.T) {
	db, clk := openTest(t)
	ctx := context.Background()
	u, err := db.CreateUser(ctx, "sam@example.com", goodPass)
	if err != nil {
		t.Fatal(err)
	}
	c, err := db.AddComment(ctx, NewComment{Preview: "demo-1234", Version: "v1", UserID: u.ID,
		Selector: "main > h1", Quote: "  The   title\n of the page ", X: 0.25, Y: 2, Body: "  Make this bigger  "})
	if err != nil {
		t.Fatal(err)
	}
	if c.Author != "sam@example.com" || c.Body != "Make this bigger" || c.Quote != "The title of the page" ||
		c.X != 0.25 || c.Y != 1 || c.Resolved != nil || c.Version != "v1" || c.Selector != "main > h1" {
		t.Fatalf("stored = %+v", c)
	}
	page, err := db.AddComment(ctx, NewComment{Preview: "demo-1234", UserID: u.ID, Body: "Overall fine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddComment(ctx, NewComment{Preview: "other-0000", Body: "elsewhere"}); err != nil {
		t.Fatal(err)
	}

	list, _ := db.Comments(ctx, "demo-1234", false)
	if len(list) != 2 || list[0].ID != c.ID || list[1].ID != page.ID {
		t.Fatalf("open comments = %+v", list)
	}
	if all, _ := db.Comments(ctx, "", false); len(all) != 3 {
		t.Fatalf("every open comment = %d, want 3", len(all))
	}

	clk.advance(time.Minute)
	reply := "Done: the title is now 2rem."
	r, err := db.ResolveComment(ctx, "", c.ID, true, &reply)
	if err != nil || r.Resolved == nil || r.Reply != reply {
		t.Fatalf("resolve = %+v, %v", r, err)
	}
	if list, _ := db.Comments(ctx, "demo-1234", false); len(list) != 1 {
		t.Fatalf("resolved comment still listed as open: %+v", list)
	}
	if list, _ := db.Comments(ctx, "demo-1234", true); len(list) != 2 {
		t.Fatalf("with resolved = %d, want 2", len(list))
	}
	counts, _ := db.OpenCommentCounts(ctx)
	if counts["demo-1234"] != 1 || counts["other-0000"] != 1 {
		t.Fatalf("open counts = %v", counts)
	}

	// Reopening keeps the reply; a comment of another preview is not found there.
	r, err = db.ResolveComment(ctx, "demo-1234", c.ID, false, nil)
	if err != nil || r.Resolved != nil || r.Reply != reply {
		t.Fatalf("reopen = %+v, %v", r, err)
	}
	if _, err := db.ResolveComment(ctx, "other-0000", c.ID, true, nil); !errors.Is(err, ErrNoSuchComment) {
		t.Fatalf("cross-preview resolve err = %v", err)
	}
	if err := db.DeleteComment(ctx, "other-0000", c.ID); !errors.Is(err, ErrNoSuchComment) {
		t.Fatalf("cross-preview delete err = %v", err)
	}
	if err := db.DeleteComment(ctx, "demo-1234", c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetComment(ctx, c.ID); !errors.Is(err, ErrNoSuchComment) {
		t.Fatalf("deleted comment err = %v", err)
	}
	if err := db.DeleteComments(ctx, "demo-1234"); err != nil {
		t.Fatal(err)
	}
	if list, _ := db.Comments(ctx, "demo-1234", true); len(list) != 0 {
		t.Fatalf("comments left after DeleteComments: %+v", list)
	}

	// Removing the account keeps its comments, without an author.
	if _, err := db.AddComment(ctx, NewComment{Preview: "keep-0000", UserID: u.ID, Body: "stays"}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteUser(ctx, "sam@example.com"); err != nil {
		t.Fatal(err)
	}
	if list, _ := db.Comments(ctx, "keep-0000", false); len(list) != 1 || list[0].Author != "" {
		t.Fatalf("after account removal = %+v", list)
	}
}

func TestCommentValidation(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	for _, tc := range []struct {
		in   NewComment
		want error
	}{
		{NewComment{Preview: "p-0000", Body: "   "}, ErrCommentEmpty},
		{NewComment{Preview: "p-0000", Body: strings.Repeat("é", MaxCommentBody+1)}, ErrCommentTooLong},
		{NewComment{Preview: "p-0000", Body: "x", Selector: strings.Repeat("a", MaxCommentSelector+1)}, ErrCommentBadAnchor},
		{NewComment{Preview: "p-0000", Body: "bad \xff utf8"}, ErrCommentBadAnchor},
	} {
		if _, err := db.AddComment(ctx, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("AddComment(%.40q) err = %v, want %v", tc.in.Body, err, tc.want)
		}
	}
	long, err := db.AddComment(ctx, NewComment{Preview: "p-0000", Body: "x", Quote: strings.Repeat("q", 500)})
	if err != nil || len(long.Quote) != MaxCommentQuote {
		t.Fatalf("long quote = %d chars, %v", len(long.Quote), err)
	}
	tooLong := strings.Repeat("r", MaxCommentReply+1)
	if _, err := db.ResolveComment(ctx, "", long.ID, true, &tooLong); !errors.Is(err, ErrCommentTooLong) {
		t.Fatalf("long reply err = %v", err)
	}
}

func TestCommentCapPerPreview(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	for i := 0; i < MaxCommentsPerPreview; i++ {
		if _, err := db.AddComment(ctx, NewComment{Preview: "p-0000", Body: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.AddComment(ctx, NewComment{Preview: "p-0000", Body: "one more"}); !errors.Is(err, ErrTooManyComments) {
		t.Fatalf("over the cap err = %v", err)
	}
	if _, err := db.AddComment(ctx, NewComment{Preview: "q-0000", Body: "other preview"}); err != nil {
		t.Fatalf("cap leaked to another preview: %v", err)
	}
}

func TestCommentsStampChangesOnEveryWrite(t *testing.T) {
	db, clk := openTest(t)
	ctx := context.Background()
	seen := map[string]bool{}
	stamp := func(step string) {
		t.Helper()
		s, err := db.CommentsStamp(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if seen[s] {
			t.Fatalf("stamp %q repeated after %s", s, step)
		}
		seen[s] = true
	}
	stamp("start")
	c, _ := db.AddComment(ctx, NewComment{Preview: "p-0000", Body: "x"})
	stamp("add")
	// Same clock tick: the stamp must still move.
	if _, err := db.ResolveComment(ctx, "", c.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	stamp("resolve")
	clk.advance(time.Second)
	if err := db.DeleteComment(ctx, "p-0000", c.ID); err != nil {
		t.Fatal(err)
	}
	stamp("delete")
}

func TestBarTokenIsBoundToSlugAndSession(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	u, _ := db.CreateUser(ctx, "sam@example.com", goodPass)
	sess, _ := db.CreateSession(ctx, u)
	owner, err := db.OwnerToken(u.ID, sess.Token)
	if err != nil {
		t.Fatal(err)
	}
	tok := db.BarToken(owner, "demo-1234")
	if tok == "" {
		t.Fatal("no bar token for a valid owner cookie")
	}
	if uid, ok := db.CheckBarToken(tok, "demo-1234"); !ok || uid != u.ID {
		t.Fatalf("check = %d, %v", uid, ok)
	}
	if _, ok := db.CheckBarToken(tok, "other-0000"); ok {
		t.Fatal("token accepted for another preview")
	}
	if _, ok := db.CheckBarToken(owner, "demo-1234"); ok {
		t.Fatal("the owner cookie itself must not pass as a bar token")
	}
	if db.BarToken("1.2.bad", "demo-1234") != "" {
		t.Fatal("bar token minted from an invalid owner cookie")
	}
	if err := db.DeleteSession(ctx, sess.Token); err != nil {
		t.Fatal(err)
	}
	if _, ok := db.CheckBarToken(tok, "demo-1234"); ok {
		t.Fatal("token outlived its session")
	}
}
