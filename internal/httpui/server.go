package httpui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"mastergate/internal/workflow"
)

//go:embed assets/*
var assets embed.FS

type Server struct {
	service *workflow.Service
	mux     *http.ServeMux
}

func New(service *workflow.Service) *Server {
	s := &Server{service: service, mux: http.NewServeMux()}
	static, _ := fs.Sub(assets, "assets")
	s.mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(static))))
	s.mux.HandleFunc("GET /{$}", s.HandleWorkspace)
	s.mux.HandleFunc("GET /api/cases", s.HandleListCases)
	s.mux.HandleFunc("POST /api/cases", s.HandleCreateCase)
	s.mux.HandleFunc("/api/cases/", s.HandleCaseRoute)
	return s
}

func (s *Server) Handler() http.Handler { return securityHeaders(s.mux) }

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func splitCasePath(path string) (caseID, action string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/cases/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}
	if len(parts) > 2 {
		return "", "", false
	}
	caseID = parts[0]
	if len(parts) == 2 {
		action = parts[1]
	}
	return caseID, action, true
}
