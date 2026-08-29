package server

import "net/http"

func (s *Server) withAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.adminAuthorized(w, r) {
			return
		}
		next(w, r)
	}
}

func (s *Server) adminAuthorized(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.AdminUsername == "" && s.cfg.AdminPassword == "" {
		return true
	}
	user, pass, ok := r.BasicAuth()
	if ok && subtleConstantTimeCompare(user, s.cfg.AdminUsername) && subtleConstantTimeCompare(pass, s.cfg.AdminPassword) {
		return true
	}
	w.Header().Set("WWW-Authenticate", `Basic realm="Vexlo Dashboard"`)
	s.writeError(w, r, http.StatusUnauthorized, "admin authentication required", nil)
	return false
}
