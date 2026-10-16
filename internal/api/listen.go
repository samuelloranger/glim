package api

import (
	"errors"
	"net/http"
	"os"

	"github.com/samuelloranger/glim/internal/auth"
)

// features tells the dashboard which optional parts are set up.
type featuresJSON struct {
	// Listen is true when a model is configured to describe previews;
	// Speech when a speech endpoint makes the audio (otherwise the client
	// reads the narration with the device's voice).
	Listen bool `json:"listen"`
	Speech bool `json:"speech"`
}

func (s *Server) features() featuresJSON {
	return featuresJSON{Listen: s.d.Listen.Enabled(), Speech: s.d.Listen.Speech()}
}

func (s *Server) getListen(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	if name, ok := s.listenPreview(w, r); ok {
		writeJSON(w, http.StatusOK, s.d.Listen.Status(name))
	}
}

func (s *Server) postListen(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	if name, ok := s.listenPreview(w, r); ok {
		s.startListen(w, r, name)
	}
}

func (s *Server) getListenAudio(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	if name, ok := s.listenPreview(w, r); ok {
		s.serveListenAudio(w, r, name)
	}
}

func (s *Server) getBarListen(w http.ResponseWriter, r *http.Request, name, _ string, _ int64) {
	if s.listenOn(w) {
		writeJSON(w, http.StatusOK, s.d.Listen.Status(name))
	}
}

func (s *Server) postBarListen(w http.ResponseWriter, r *http.Request, name, _ string, _ int64) {
	if s.listenOn(w) {
		s.startListen(w, r, name)
	}
}

func (s *Server) getBarListenAudio(w http.ResponseWriter, r *http.Request, name, _ string, _ int64) {
	if s.listenOn(w) {
		s.serveListenAudio(w, r, name)
	}
}

func (s *Server) listenOn(w http.ResponseWriter) bool {
	if !s.d.Listen.Enabled() {
		writeErr(w, http.StatusNotFound, "not_found", "Listen isn't set up on this server. See the Listen section of the configuration docs.")
		return false
	}
	return true
}

func (s *Server) listenPreview(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !s.listenOn(w) {
		return "", false
	}
	return s.livePreview(w, r)
}

// startListen begins making the audio version; a JSON body {"again": true}
// makes a new one even when the current one is up to date.
func (s *Server) startListen(w http.ResponseWriter, r *http.Request, name string) {
	var body struct {
		Again bool `json:"again"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	st, err := s.d.Listen.Start(name, body.Again)
	switch {
	case errors.Is(err, os.ErrNotExist):
		writeErr(w, http.StatusNotFound, "not_found", "That preview no longer exists.")
	case err != nil:
		s.internal(w, err)
	default:
		writeJSON(w, http.StatusAccepted, st)
	}
}

func (s *Server) serveListenAudio(w http.ResponseWriter, r *http.Request, name string) {
	path, res, ok := s.d.Listen.Audio(name)
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "This preview has no audio version yet.")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "This preview has no audio version yet.")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "audio/mpeg")
	// The audio is private to the owner, like the rest of the API, but it
	// may be fetched in ranges: let the browser keep it for this session.
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, "", res.Created, f)
}
