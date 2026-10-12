package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
)

// BarHeader carries the glim bar's comment token (see auth.DB.BarToken).
const BarHeader = "X-Glim-Bar"

// CommentJSON is a comment as the glim bar and the dashboard see it.
type CommentJSON struct {
	ID       int64      `json:"id"`
	Author   string     `json:"author"`
	Selector string     `json:"selector"`
	Quote    string     `json:"quote"`
	X        float64    `json:"x"`
	Y        float64    `json:"y"`
	Body     string     `json:"body"`
	Reply    string     `json:"reply"`
	Created  time.Time  `json:"created"`
	Resolved *time.Time `json:"resolved"`
	// Stale is true when the preview was republished after the comment.
	Stale bool `json:"stale"`
}

func toCommentJSON(c auth.Comment, version string) CommentJSON {
	return CommentJSON{ID: c.ID, Author: c.Author, Selector: c.Selector, Quote: c.Quote, X: c.X, Y: c.Y,
		Body: c.Body, Reply: c.Reply, Created: c.Created, Resolved: c.Resolved,
		Stale: c.Version != "" && c.Version != version}
}

// barCORS answers the sandboxed preview page the glim bar runs in. Its origin
// is opaque ("null") and it sends no cookies; the token in BarHeader is the
// only credential, so credentials are never allowed.
func barCORS(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Add("Vary", "Origin")
	if r.Header.Get("Origin") == "null" {
		h.Set("Access-Control-Allow-Origin", "null")
	}
}

func (s *Server) barPreflight(w http.ResponseWriter, r *http.Request) {
	barCORS(w, r)
	h := w.Header()
	h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE")
	h.Set("Access-Control-Allow-Headers", "Content-Type, "+BarHeader)
	h.Set("Access-Control-Max-Age", "600")
	w.WriteHeader(http.StatusNoContent)
}

type barHandler func(w http.ResponseWriter, r *http.Request, name, version string, userID int64)

// barAuthed requires a live preview and a bar token minted for it.
func (s *Server) barAuthed(h barHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		barCORS(w, r)
		name, ok := s.livePreview(w, r)
		if !ok {
			return
		}
		uid, ok := s.d.Auth.CheckBarToken(r.Header.Get(BarHeader), name)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "Your dashboard session ended. Sign in again, then reload this page.")
			return
		}
		m, _ := s.d.Store.Live(name)
		h(w, r, name, m.Version, uid)
	})
}

func (s *Server) getBarComments(w http.ResponseWriter, r *http.Request, name, version string, _ int64) {
	list, err := s.d.Auth.Comments(r.Context(), name, true)
	if err != nil {
		s.internal(w, err)
		return
	}
	out := make([]CommentJSON, 0, len(list))
	for _, c := range list {
		out = append(out, toCommentJSON(c, version))
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": out})
}

func (s *Server) postBarComment(w http.ResponseWriter, r *http.Request, name, version string, uid int64) {
	var body struct {
		Selector string  `json:"selector"`
		Quote    string  `json:"quote"`
		X        float64 `json:"x"`
		Y        float64 `json:"y"`
		Body     string  `json:"body"`
	}
	if !decode(w, r, &body) {
		return
	}
	c, err := s.d.Auth.AddComment(r.Context(), auth.NewComment{Preview: name, Version: version, UserID: uid,
		Selector: body.Selector, Quote: body.Quote, X: body.X, Y: body.Y, Body: body.Body})
	if err != nil {
		s.failComment(w, err)
		return
	}
	s.poke()
	writeJSON(w, http.StatusCreated, toCommentJSON(c, version))
}

func (s *Server) patchBarComment(w http.ResponseWriter, r *http.Request, name, version string, _ int64) {
	id, ok := commentID(w, r)
	if !ok {
		return
	}
	var body struct {
		Resolved bool `json:"resolved"`
	}
	if !decode(w, r, &body) {
		return
	}
	c, err := s.d.Auth.ResolveComment(r.Context(), name, id, body.Resolved, nil)
	if err != nil {
		s.failComment(w, err)
		return
	}
	s.poke()
	writeJSON(w, http.StatusOK, toCommentJSON(c, version))
}

func (s *Server) deleteBarComment(w http.ResponseWriter, r *http.Request, name, _ string, _ int64) {
	id, ok := commentID(w, r)
	if !ok {
		return
	}
	if err := s.d.Auth.DeleteComment(r.Context(), name, id); err != nil {
		s.failComment(w, err)
		return
	}
	s.poke()
	w.WriteHeader(http.StatusNoContent)
}

func commentID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid", "That isn't a valid comment id.")
		return 0, false
	}
	return id, true
}

func (s *Server) failComment(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrNoSuchComment):
		writeErr(w, http.StatusNotFound, "not_found", "That comment no longer exists.")
	case errors.Is(err, auth.ErrCommentEmpty):
		writeErr(w, http.StatusBadRequest, "invalid", "Write something first.")
	case errors.Is(err, auth.ErrCommentTooLong):
		writeErr(w, http.StatusBadRequest, "invalid", fmt.Sprintf("Comments can be at most %d characters.", auth.MaxCommentBody))
	case errors.Is(err, auth.ErrCommentBadAnchor):
		writeErr(w, http.StatusBadRequest, "invalid", "That comment can't be saved. Try commenting on the whole page.")
	case errors.Is(err, auth.ErrTooManyComments):
		writeErr(w, http.StatusConflict, "conflict", fmt.Sprintf("This preview already has %d comments. Delete some first.", auth.MaxCommentsPerPreview))
	default:
		s.internal(w, err)
	}
}
