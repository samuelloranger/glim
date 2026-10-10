package api

import (
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/samuelloranger/glim/internal/auth"
)

var b64url = regexp.MustCompile(`^[A-Za-z0-9_=-]+$`)

// validEndpoint accepts only https push-service URLs, and refuses hosts that
// would turn the server into a probe of its own network.
func validEndpoint(raw string) bool {
	if len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return false
	}
	if ip := net.ParseIP(h); ip != nil && (!ip.IsGlobalUnicast() || ip.IsPrivate()) {
		return false
	}
	return true
}

func (s *Server) getPushKey(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	_, pub, err := s.d.Auth.VAPIDKeys(r.Context())
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": pub})
}

func (s *Server) postPushSubscribe(w http.ResponseWriter, r *http.Request, sess auth.Session) {
	var body struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
		// Browsers include this in PushSubscription.toJSON(); it carries nothing we use.
		ExpirationTime *float64 `json:"expirationTime"`
	}
	if !decode(w, r, &body) {
		return
	}
	k := body.Keys
	if !validEndpoint(body.Endpoint) {
		writeErr(w, http.StatusBadRequest, "invalid", "The push endpoint must be an https URL.")
		return
	}
	if k.P256dh == "" || k.Auth == "" || len(k.P256dh) > 256 || len(k.Auth) > 64 ||
		!b64url.MatchString(k.P256dh) || !b64url.MatchString(k.Auth) {
		writeErr(w, http.StatusBadRequest, "invalid", "The push subscription keys are missing or malformed.")
		return
	}
	err := s.d.Auth.SavePushSub(r.Context(), auth.PushSub{
		Endpoint: body.Endpoint, P256dh: k.P256dh, Auth: k.Auth, UserID: sess.User.ID})
	if err != nil {
		s.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) postPushUnsubscribe(w http.ResponseWriter, r *http.Request, sess auth.Session) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Endpoint == "" || len(body.Endpoint) > 2048 {
		writeErr(w, http.StatusBadRequest, "invalid", "A push endpoint is required.")
		return
	}
	if err := s.d.Auth.DeletePushSub(r.Context(), sess.User.ID, body.Endpoint); err != nil {
		s.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
