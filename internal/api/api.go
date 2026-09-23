// Package api is the dashboard's JSON + SSE interface under /_glim/api.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
)

const cookieName = "glim_session"

type Deps struct {
	Store         *store.Store
	Auth          *auth.DB
	Limiter       *auth.Limiter
	Hub           *Hub
	SetupCodePath string
	SecureCookies bool
	Logf          func(format string, args ...any)
	// SSE timings; zero selects 25s keep-alive pings and 2s session checks.
	PingEvery  time.Duration
	CheckEvery time.Duration
}

type Server struct {
	d   Deps
	mux *http.ServeMux
}

func (s *Server) poke() {
	if s.d.Hub != nil {
		s.d.Hub.Poke()
	}
}

func New(d Deps) *Server {
	if d.Logf == nil {
		d.Logf = log.Printf
	}
	if d.Limiter == nil {
		d.Limiter = auth.NewLimiter(nil)
	}
	if d.PingEvery == 0 {
		d.PingEvery = 25 * time.Second
	}
	if d.CheckEvery == 0 {
		d.CheckEvery = 2 * time.Second
	}
	s := &Server{d: d, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /_glim/api/setup", s.getSetup)
	m.HandleFunc("POST /_glim/api/setup", s.postSetup)
	m.HandleFunc("POST /_glim/api/login", s.postLogin)
	m.Handle("POST /_glim/api/logout", s.authed(s.postLogout))
	m.Handle("GET /_glim/api/session", s.authed(s.getSession))
	m.Handle("GET /_glim/api/previews", s.authed(s.getPreviews))
	m.Handle("POST /_glim/api/previews/{name}/extend", s.authed(s.postExtend))
	m.Handle("POST /_glim/api/previews/{name}/pin", s.authed(s.postPin))
	m.Handle("DELETE /_glim/api/previews/{name}", s.authed(s.deletePreview))
	m.Handle("GET /_glim/api/events", s.authed(s.getEvents))
	m.HandleFunc("/_glim/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "not_found", "No such endpoint.")
	})
}

type sessionHandler func(w http.ResponseWriter, r *http.Request, sess auth.Session)

// authed requires a valid session cookie, and for anything but GET/HEAD a
// matching X-Glim-CSRF header and a same-origin Origin.
func (s *Server) authed(h sessionHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "Sign in to continue.")
			return
		}
		sess, err := s.d.Auth.LookupSession(r.Context(), c.Value)
		if errors.Is(err, auth.ErrNoSession) {
			s.clearCookie(w)
			writeErr(w, http.StatusUnauthorized, "unauthorized", "Your session ended. Sign in again.")
			return
		}
		if err != nil {
			s.internal(w, err)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			got := r.Header.Get("X-Glim-CSRF")
			if subtle.ConstantTimeCompare([]byte(got), []byte(sess.CSRF)) != 1 || !sameOrigin(r) {
				writeErr(w, http.StatusForbidden, "forbidden", "This request was blocked. Reload the page and try again.")
				return
			}
		}
		h(w, r, sess)
	})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, sess auth.Session) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: sess.Token, Path: "/_glim", Expires: sess.ExpiresAt,
		HttpOnly: true, Secure: s.d.SecureCookies, SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/_glim", MaxAge: -1,
		HttpOnly: true, Secure: s.d.SecureCookies, SameSite: http.SameSiteStrictMode,
	})
}

// --- JSON helpers ---

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorBody{Error: msg, Code: code})
}

func (s *Server) internal(w http.ResponseWriter, err error) {
	s.d.Logf("api: %v", err)
	writeErr(w, http.StatusInternalServerError, "internal", "Something went wrong on the server. Check the glim serve log.")
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeErr(w, http.StatusBadRequest, "invalid", "Expected a JSON body.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", "Malformed JSON body.")
		return false
	}
	return true
}

// fail maps domain errors to user-facing responses; anything else is a 500.
func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidUsername):
		writeErr(w, 400, "invalid", "Usernames use 1–32 lowercase letters, digits, dots, underscores or hyphens.")
	case errors.Is(err, auth.ErrPasswordTooShort):
		writeErr(w, 400, "invalid", fmt.Sprintf("Passwords need at least %d characters.", auth.MinPasswordChars))
	case errors.Is(err, auth.ErrPasswordTooLong):
		writeErr(w, 400, "invalid", fmt.Sprintf("Passwords can be at most %d bytes.", auth.MaxPasswordBytes))
	case errors.Is(err, auth.ErrUserExists):
		writeErr(w, 409, "conflict", "That username is taken.")
	case errors.Is(err, auth.ErrNoSuchUser):
		writeErr(w, 404, "not_found", "No such user.")
	case errors.Is(err, auth.ErrBadSetupCode):
		writeErr(w, 400, "invalid", "That setup code doesn't match. Check the glim serve log.")
	case errors.Is(err, auth.ErrSetupDone):
		writeErr(w, 409, "conflict", "Setup is already complete. Sign in instead.")
	case errors.Is(err, auth.ErrBadCredentials):
		writeErr(w, 401, "unauthorized", "Wrong username or password.")
	default:
		s.internal(w, err)
	}
}

func tooMany(w http.ResponseWriter, wait time.Duration) {
	secs := int(math.Ceil(wait.Seconds()))
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeErr(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Try again in "+humanWait(secs)+".")
}

func humanWait(secs int) string {
	if secs < 60 {
		if secs == 1 {
			return "1 second"
		}
		return fmt.Sprintf("%d seconds", secs)
	}
	m := (secs + 59) / 60
	if m == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", m)
}

// --- peers, client IP, origin ---

func trustedPeer(ip string) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate()
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// clientIP is the address throttling keys on. X-Forwarded-For is believed only
// when the direct peer is loopback or private (a reverse proxy); then the
// right-most untrusted hop wins.
func clientIP(r *http.Request) string {
	peer := remoteIP(r)
	if !trustedPeer(peer) {
		return peer
	}
	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		for _, h := range strings.Split(v, ",") {
			if h = strings.TrimSpace(h); h != "" {
				hops = append(hops, h)
			}
		}
	}
	for i := len(hops) - 1; i >= 0; i-- {
		if !trustedPeer(hops[i]) {
			return hops[i]
		}
	}
	if len(hops) > 0 {
		return hops[0]
	}
	return peer
}

func requestHost(r *http.Request) string {
	if trustedPeer(remoteIP(r)) {
		if h := r.Header.Get("X-Forwarded-Host"); h != "" {
			h, _, _ = strings.Cut(h, ",")
			return strings.TrimSpace(h)
		}
	}
	return r.Host
}

// sameOrigin accepts a missing Origin; "null" (sandboxed documents) and any
// other host are rejected.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, requestHost(r))
}
