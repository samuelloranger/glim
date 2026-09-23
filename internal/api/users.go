package api

import (
	"errors"
	"net/http"

	"github.com/samuelloranger/glim/internal/auth"
)

func (s *Server) getUsers(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	users, err := s.d.Auth.ListUsers(r.Context())
	if err != nil {
		s.internal(w, err)
		return
	}
	out := make([]userJSON, 0, len(users))
	for _, u := range users {
		out = append(out, toUserJSON(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (s *Server) postUser(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	u, err := s.d.Auth.CreateUser(r.Context(), body.Username, body.Password)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toUserJSON(u))
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request, sess auth.Session) {
	name, err := auth.NormalizeUsername(r.PathValue("name"))
	if err != nil {
		s.fail(w, err)
		return
	}
	if name == sess.User.Username {
		writeErr(w, http.StatusConflict, "conflict", "You can't remove your own account.")
		return
	}
	if err := s.d.Auth.DeleteUser(r.Context(), name); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) postPassword(w http.ResponseWriter, r *http.Request, sess auth.Session) {
	var body struct {
		Current string `json:"current"`
		Next    string `json:"next"`
	}
	if !decode(w, r, &body) {
		return
	}
	err := s.d.Auth.ChangePassword(r.Context(), sess.User, body.Current, body.Next, sess.Token)
	if errors.Is(err, auth.ErrBadCredentials) {
		writeErr(w, http.StatusBadRequest, "invalid", "Your current password is wrong.")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
