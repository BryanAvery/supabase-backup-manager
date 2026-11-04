package server

import (
	"encoding/json"
	"net/http"
)

// Version is the implementation version, not a statement about backup readiness.
const Version = "0.1.0-dev"

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// NewHandler exposes only a process health check. All management routes remain
// unavailable until authentication and project-level authorisation exist.
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok", Version: Version})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}
