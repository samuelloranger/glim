package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
)

type userJSON struct {
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

type sessionJSON struct {
	User userJSON `json:"user"`
	CSRF string   `json:"csrf"`
}

func toUserJSON(u auth.User) userJSON { return userJSON{Email: u.Email, CreatedAt: u.CreatedAt} }

func (s *Server) getSetup(w http.ResponseWriter, r *http.Request) {
	n, err := s.d.Auth.CountUsers(r.Context())
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"needed": n == 0})
}

func (s *Server) postSetup(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeErr(w, http.StatusForbidden, "forbidden", "This request was blocked. Reload the page and try again.")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	user, err := s.d.Auth.CompleteSetup(r.Context(), body.Email, body.Password)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.d.Logf("setup complete: first account created")
	s.startSession(w, r, user, http.StatusCreated)
}

func (s *Server) postLogin(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeErr(w, http.StatusForbidden, "forbidden", "This request was blocked. Reload the page and try again.")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(body.Email))
	ip := clientIP(r)
	if wait := s.d.Limiter.Check(name, ip); wait > 0 {
		tooMany(w, wait)
		return
	}
	user, err := s.d.Auth.Authenticate(r.Context(), name, body.Password)
	if err != nil {
		if errors.Is(err, auth.ErrBadCredentials) {
			s.d.Limiter.Fail(name, ip)
		}
		s.fail(w, err)
		return
	}
	s.d.Limiter.Succeed(name)
	s.startSession(w, r, user, http.StatusOK)
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, user auth.User, status int) {
	sess, err := s.d.Auth.CreateSession(r.Context(), user)
	if err != nil {
		s.internal(w, err)
		return
	}
	s.setSessionCookie(w, sess)
	writeJSON(w, status, sessionJSON{User: toUserJSON(user), CSRF: sess.CSRF})
}

func (s *Server) postLogout(w http.ResponseWriter, r *http.Request, sess auth.Session) {
	if err := s.d.Auth.DeleteSession(r.Context(), sess.Token); err != nil {
		s.internal(w, err)
		return
	}
	s.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// getSession also re-issues the cookie so its browser-side expiry tracks the
// sliding server-side expiry.
func (s *Server) getSession(w http.ResponseWriter, r *http.Request, sess auth.Session) {
	s.setSessionCookie(w, sess)
	writeJSON(w, http.StatusOK, sessionJSON{User: toUserJSON(sess.User), CSRF: sess.CSRF})
}
