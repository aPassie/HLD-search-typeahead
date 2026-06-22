package api

import (
	"encoding/json"
	"net/http"
)

// handleSuggest returns up to TopK prefix suggestions for ?q=. Optional ?mode=
// (count|recency) overrides the configured default — used to show the difference.
func (s *Server) handleSuggest(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	mode := r.URL.Query().Get("mode")
	writeJSON(w, http.StatusOK, s.eng.Suggest(q, mode))
}

// handleSearch records the submitted query (via the batch writer) and returns
// the dummy response. The count update is reflected after the next flush.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Q string `json:"q"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if !s.eng.Record(body.Q) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty query"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Searched"})
}

// handleTrending returns the current top trending queries (recency-ranked).
func (s *Server) handleTrending(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.Trending())
}

// handleCacheDebug shows which cache node owns a prefix and whether it is cached.
func (s *Server) handleCacheDebug(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	if prefix == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty prefix"})
		return
	}
	writeJSON(w, http.StatusOK, s.eng.CacheDebug(prefix))
}

// handleMetrics exposes latency, cache, write-reduction, and DB stats.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"latency": s.eng.LatencyStats(),
		"cache":   s.eng.CacheStats(),
		"writes":  s.eng.WriteStats(),
		"db":      s.eng.DBStats(),
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
