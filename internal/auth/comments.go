package auth

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Limits on what a comment may hold.
const (
	MaxCommentBody        = 4000 // characters
	MaxCommentReply       = 4000 // characters
	MaxCommentQuote       = 200  // characters; longer quotes are cut
	MaxCommentSelector    = 1000 // bytes
	MaxCommentsPerPreview = 500
)

var (
	ErrNoSuchComment    = errors.New("no such comment")
	ErrCommentEmpty     = errors.New("comment is empty")
	ErrCommentTooLong   = errors.New("comment is too long")
	ErrTooManyComments  = errors.New("too many comments on this preview")
	ErrCommentBadAnchor = errors.New("comment anchor is invalid")
)

// Comment is a note left on a preview from the glim bar, optionally anchored
// to an element of the page. Reply is the answer an agent left over MCP.
type Comment struct {
	ID      int64
	Preview string
	// Version is the preview's manifest version when the comment was made, so
	// a reader can tell the page has been republished since.
	Version string
	Author  string // the commenter's email; empty once that account is removed
	// Selector is a CSS selector of the element commented on, empty for a
	// comment on the whole page. X and Y place the comment within that
	// element's box, as fractions of its width and height.
	Selector string
	Quote    string // the start of the element's text, to recognise it by
	X, Y     float64
	Body     string
	Reply    string
	Created  time.Time
	Updated  time.Time
	Resolved *time.Time
}

// NewComment is what the glim bar submits.
type NewComment struct {
	Preview  string
	Version  string
	UserID   int64
	Selector string
	Quote    string
	X, Y     float64
	Body     string
}

func cleanText(s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) {
		return "", ErrCommentBadAnchor
	}
	if utf8.RuneCountInString(s) > max {
		return "", ErrCommentTooLong
	}
	return s, nil
}

// cutRunes shortens s to at most n characters.
func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func clamp01(f float64) float64 {
	if f != f || f < 0 { // NaN or negative
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// AddComment stores c and returns it as read back.
func (db *DB) AddComment(ctx context.Context, c NewComment) (Comment, error) {
	body, err := cleanText(c.Body, MaxCommentBody)
	if err != nil {
		return Comment{}, err
	}
	if body == "" {
		return Comment{}, ErrCommentEmpty
	}
	sel := strings.TrimSpace(c.Selector)
	if len(sel) > MaxCommentSelector || !utf8.ValidString(sel) {
		return Comment{}, ErrCommentBadAnchor
	}
	quote := strings.Join(strings.Fields(c.Quote), " ")
	if !utf8.ValidString(quote) {
		return Comment{}, ErrCommentBadAnchor
	}
	quote = cutRunes(quote, MaxCommentQuote)
	now := db.now().UnixMilli()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return Comment{}, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM comments WHERE preview = ?`, c.Preview).Scan(&n); err != nil {
		return Comment{}, err
	}
	if n >= MaxCommentsPerPreview {
		return Comment{}, ErrTooManyComments
	}
	var uid any
	if c.UserID > 0 {
		uid = c.UserID
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO comments(preview, version, user_id, selector, quote, x, y, body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Preview, c.Version, uid, sel, quote, clamp01(c.X), clamp01(c.Y), body, now, now)
	if err != nil {
		return Comment{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Comment{}, err
	}
	if err := bumpCommentsRev(ctx, tx); err != nil {
		return Comment{}, err
	}
	if err := tx.Commit(); err != nil {
		return Comment{}, err
	}
	return db.GetComment(ctx, id)
}

const commentCols = `c.id, c.preview, c.version, COALESCE(u.email, ''), c.selector, c.quote, c.x, c.y,
	c.body, c.reply, c.created_at, c.updated_at, c.resolved_at
	FROM comments c LEFT JOIN users u ON u.id = c.user_id`

type rowScanner interface{ Scan(dest ...any) error }

func scanComment(r rowScanner) (Comment, error) {
	var c Comment
	var created, updated int64
	var resolved sql.NullInt64
	if err := r.Scan(&c.ID, &c.Preview, &c.Version, &c.Author, &c.Selector, &c.Quote, &c.X, &c.Y,
		&c.Body, &c.Reply, &created, &updated, &resolved); err != nil {
		return Comment{}, err
	}
	c.Created = time.UnixMilli(created).UTC()
	c.Updated = time.UnixMilli(updated).UTC()
	if resolved.Valid {
		t := time.UnixMilli(resolved.Int64).UTC()
		c.Resolved = &t
	}
	return c, nil
}

// GetComment returns one comment by id.
func (db *DB) GetComment(ctx context.Context, id int64) (Comment, error) {
	c, err := scanComment(db.sql.QueryRowContext(ctx, `SELECT `+commentCols+` WHERE c.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Comment{}, ErrNoSuchComment
	}
	return c, err
}

// Comments lists comments oldest first: those on preview, or on every preview
// when preview is empty. Resolved ones are left out unless includeResolved.
func (db *DB) Comments(ctx context.Context, preview string, includeResolved bool) ([]Comment, error) {
	q := `SELECT ` + commentCols + ` WHERE 1 = 1`
	var args []any
	if preview != "" {
		q += ` AND c.preview = ?`
		args = append(args, preview)
	}
	if !includeResolved {
		q += ` AND c.resolved_at IS NULL`
	}
	rows, err := db.sql.QueryContext(ctx, q+` ORDER BY c.preview, c.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ResolveComment marks a comment resolved or open again. A non-nil reply
// replaces the comment's reply (an empty one clears it). preview, when not
// empty, must be the comment's preview.
func (db *DB) ResolveComment(ctx context.Context, preview string, id int64, resolved bool, reply *string) (Comment, error) {
	cur, err := db.GetComment(ctx, id)
	if err != nil {
		return Comment{}, err
	}
	if preview != "" && cur.Preview != preview {
		return Comment{}, ErrNoSuchComment
	}
	text := cur.Reply
	if reply != nil {
		if text, err = cleanText(*reply, MaxCommentReply); err != nil {
			return Comment{}, err
		}
	}
	now := db.now().UnixMilli()
	var at any
	if resolved {
		at = now
		if cur.Resolved != nil {
			at = cur.Resolved.UnixMilli()
		}
	}
	if err := db.writeComments(ctx, func(tx *sql.Tx) (int64, error) {
		res, err := tx.ExecContext(ctx,
			`UPDATE comments SET resolved_at = ?, reply = ?, updated_at = ? WHERE id = ?`, at, text, now, id)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}); err != nil {
		return Comment{}, err
	}
	return db.GetComment(ctx, id)
}

// DeleteComment removes one comment of preview.
func (db *DB) DeleteComment(ctx context.Context, preview string, id int64) error {
	return db.writeComments(ctx, func(tx *sql.Tx) (int64, error) {
		res, err := tx.ExecContext(ctx, `DELETE FROM comments WHERE id = ? AND preview = ?`, id, preview)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err == nil && n == 0 {
			err = ErrNoSuchComment
		}
		return n, err
	})
}

// DeleteComments forgets every comment of a preview (it was removed or
// expired).
func (db *DB) DeleteComments(ctx context.Context, preview string) error {
	return db.writeComments(ctx, func(tx *sql.Tx) (int64, error) {
		res, err := tx.ExecContext(ctx, `DELETE FROM comments WHERE preview = ?`, preview)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	})
}

// writeComments runs write in a transaction and, when it changed any row,
// bumps the comments revision that CommentsStamp reports.
func (db *DB) writeComments(ctx context.Context, write func(*sql.Tx) (int64, error)) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	n, err := write(tx)
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	if err := bumpCommentsRev(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func bumpCommentsRev(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO settings(key, value) VALUES ('comments_rev', 1)
		ON CONFLICT(key) DO UPDATE SET value = value + 1`)
	return err
}

// OpenCommentCounts returns how many unresolved comments each preview has;
// previews without any are absent.
func (db *DB) OpenCommentCounts(ctx context.Context) (map[string]int, error) {
	rows, err := db.sql.QueryContext(ctx,
		`SELECT preview, COUNT(*) FROM comments WHERE resolved_at IS NULL GROUP BY preview`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			return nil, err
		}
		out[name] = n
	}
	return out, rows.Err()
}

// CommentsStamp changes whenever a comment is added, changed or deleted, so a
// watcher in another process can notice without reading every comment.
func (db *DB) CommentsStamp(ctx context.Context) (string, error) {
	var rev int64
	err := db.sql.QueryRowContext(ctx,
		`SELECT COALESCE((SELECT value FROM settings WHERE key = 'comments_rev'), 0)`).Scan(&rev)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(rev, 10), nil
}

// --- the glim bar's comment token ---

// BarToken returns the token the glim bar on slug sends with its comment
// requests, minted from a valid owner cookie value: "<user id>.<session
// id>.<hmac>", bound to the slug and dying with the session. It is "" when
// the owner cookie is not valid.
//
// The bar runs inside the preview's sandboxed page, which sends no cookies, so
// the token travels in a request header. The page's own scripts can read it,
// which lets them act on that one preview's comments and nothing else.
func (db *DB) BarToken(ownerCookie, slug string) string {
	if !db.IsOwnerToken(ownerCookie) {
		return ""
	}
	parts := strings.Split(ownerCookie, ".")
	key, err := db.ownerKey()
	if err != nil || len(key) == 0 {
		return ""
	}
	return parts[0] + "." + parts[1] + "." + ownerMAC(key, "bar\x00"+slug+"\x00"+parts[0]+"."+parts[1])
}

// CheckBarToken returns the user a BarToken for slug was minted for, if its
// session is still alive.
func (db *DB) CheckBarToken(tok, slug string) (int64, bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return 0, false
	}
	key, err := db.ownerKey()
	if err != nil || len(key) == 0 {
		return 0, false
	}
	if !hmac.Equal([]byte(parts[2]), []byte(ownerMAC(key, "bar\x00"+slug+"\x00"+parts[0]+"."+parts[1]))) {
		return 0, false
	}
	return db.liveSessionUser(parts[0], parts[1])
}
