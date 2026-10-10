package api

import (
	"net/http"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
)

func (s *Server) getPreviews(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	snap, err := BuildSnapshot(s.d.Store, s.d.Auth)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// livePreview validates {name} and confirms the preview can still be acted on.
func (s *Server) livePreview(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := r.PathValue("name")
	if !store.ValidName(name) {
		writeErr(w, http.StatusBadRequest, "invalid", "That isn't a valid preview name.")
		return "", false
	}
	if _, ok := s.d.Store.Live(name); !ok {
		writeErr(w, http.StatusNotFound, "not_found", "That preview no longer exists.")
		return "", false
	}
	return name, true
}

func (s *Server) respondPreview(w http.ResponseWriter, name string) {
	m, err := s.d.Store.Get(name)
	if err != nil {
		s.internal(w, err)
		return
	}
	s.poke()
	writeJSON(w, http.StatusOK, toPreview(s.d.Store, m, viewStats(s.d.Auth)[name]))
}

func (s *Server) postExtend(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	name, ok := s.livePreview(w, r)
	if !ok {
		return
	}
	var body struct {
		TTL string `json:"ttl"`
	}
	if !decode(w, r, &body) {
		return
	}
	ttl, err := store.ParseTTL(body.TTL)
	if err != nil || ttl > store.MaxTTL {
		writeErr(w, http.StatusBadRequest, "invalid", "Choose a lifetime between 1s and 8760h, like 30m, 6h or 3d.")
		return
	}
	if err := s.d.Store.Extend(name, ttl); err != nil {
		s.internal(w, err)
		return
	}
	s.respondPreview(w, name)
}

func (s *Server) postPin(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	name, ok := s.livePreview(w, r)
	if !ok {
		return
	}
	if err := s.d.Store.Pin(name); err != nil {
		s.internal(w, err)
		return
	}
	s.respondPreview(w, name)
}

func (s *Server) postUnpin(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	name, ok := s.livePreview(w, r)
	if !ok {
		return
	}
	// A configured default above the cap must not turn unpin into a 500.
	if err := s.d.Store.Unpin(name, min(s.d.DefaultTTL, store.MaxTTL)); err != nil {
		s.internal(w, err)
		return
	}
	s.respondPreview(w, name)
}

func (s *Server) deletePreview(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	name, ok := s.livePreview(w, r)
	if !ok {
		return
	}
	if err := s.d.Store.Remove(name); err != nil {
		s.internal(w, err)
		return
	}
	s.poke()
	w.WriteHeader(http.StatusNoContent)
}
