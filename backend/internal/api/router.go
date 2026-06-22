// Package api exposes the HTTP endpoints and serves the embedded static UI.
package api

import (
	"net/http"

	"searchtypeahead/internal/engine"
	"searchtypeahead/web"
)

// Server holds the dependencies the handlers need.
type Server struct {
	eng *engine.Engine
}

// NewServer constructs the API server.
func NewServer(eng *engine.Engine) *Server { return &Server{eng: eng} }

// Routes wires the endpoints. The static UI is served from the binary's embedded
// filesystem for any unmatched path; method-prefixed patterns win over "/".
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /suggest", s.handleSuggest)
	mux.HandleFunc("POST /search", s.handleSearch)
	mux.HandleFunc("GET /trending", s.handleTrending)
	mux.HandleFunc("GET /cache/debug", s.handleCacheDebug)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	mux.Handle("/", http.FileServer(http.FS(web.Files)))
	return mux
}
