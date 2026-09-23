package api

import (
	"net/http"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
)

const maxTTL = 8760 * time.Hour

func (s *Server) getPreviews(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	snap, err := BuildSnapshot(s.d.Store)
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
	writeJSON(w, http.StatusOK, toPreview(s.d.Store, m))
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
	ttl, err := time.ParseDuration(body.TTL)
	if err != nil || ttl <= 0 || ttl > maxTTL {
		writeErr(w, http.StatusBadRequest, "invalid", "Choose a lifetime between 1s and 8760h, like 30m or 6h.")
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
