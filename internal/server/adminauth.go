package server

import "net/http"

func (s *Server) withAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	if s.cfg.AdminUsername == "" && s.cfg.AdminPassword == "" {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok ||
			!subtleConstantTimeCompare(user, s.cfg.AdminUsername) ||
			!subtleConstantTimeCompare(pass, s.cfg.AdminPassword) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Vexlo Dashboard"`)
			s.writeError(w, r, http.StatusUnauthorized, "admin authentication required", nil)
			return
		}
		next(w, r)
	}
}
