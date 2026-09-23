package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
)

func writeEvent(w io.Writer, snap Snapshot) error {
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", data)
	return err
}

// getEvents streams snapshots until the client leaves or its session ends.
func (s *Server) getEvents(w http.ResponseWriter, r *http.Request, sess auth.Session) {
	fl, ok := w.(http.Flusher)
	if !ok || s.d.Hub == nil {
		s.internal(w, errors.New("streaming unsupported"))
		return
	}
	ch, cur, cancel := s.d.Hub.Subscribe()
	defer cancel()
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if writeEvent(w, cur) != nil {
		return
	}
	fl.Flush()

	ping := time.NewTicker(s.d.PingEvery)
	defer ping.Stop()
	check := time.NewTicker(s.d.CheckEvery)
	defer check.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case snap := <-ch:
			if writeEvent(w, snap) != nil {
				return
			}
		case <-ping.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
		case <-check.C:
			if _, err := s.d.Auth.LookupSession(r.Context(), sess.Token); err != nil {
				return
			}
			continue
		}
		fl.Flush()
	}
}
