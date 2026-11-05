package api

import (
	_ "embed"
	"net/http"
)

// indexHTML is the self-contained session viewer served at / and /ui/. It
// has no external dependencies: all CSS and JavaScript are inline, and it
// talks only to this server's JSON endpoints.
//
//go:embed static/index.html
var indexHTML []byte

// registerWebRoutes mounts the embedded viewer. The patterns are exact
// ("/{$}" matches only the root path) so unrelated paths still 404 and the
// JSON endpoints are never shadowed.
func (s *Server) registerWebRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /ui", s.handleIndex)
	mux.HandleFunc("GET /ui/{$}", s.handleIndex)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexHTML)
}
