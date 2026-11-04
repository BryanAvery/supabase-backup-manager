package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	NewHandler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing security headers")
	}
	if w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatal("incorrect JSON content type")
	}
	var body healthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Version != Version {
		t.Fatalf("unexpected process health: %#v", body)
	}
}

func TestNoManagementAPI(t *testing.T) {
	for _, path := range []string{"/api/v1/projects", "/api/v1/restores", "/api/v1/backups", "/events"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			NewHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			if w.Code != http.StatusNotFound {
				t.Fatalf("%s: got %d; want 404", path, w.Code)
			}
		})
	}
}

func TestHealthRejectsMutation(t *testing.T) {
	w := httptest.NewRecorder()
	NewHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/health", nil))
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET" {
		t.Fatalf("unexpected method result %d, allow %q", w.Code, w.Header().Get("Allow"))
	}
}
