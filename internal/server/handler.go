package server

import (
 "encoding/json"
 "net/http"
)

// Version identifies the development service, not backup readiness.
const Version="0.1.0-dev"

type healthResponse struct{
 Status string `json:"status"`
 Version string `json:"version"`
}
func healthHandler() http.Handler{
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Method!=http.MethodGet{
   w.Header().Set("Allow",http.MethodGet)
   http.Error(w,"method not allowed",http.StatusMethodNotAllowed)
   return
  }
  w.Header().Set("Content-Type","application/json; charset=utf-8")
  _=json.NewEncoder(w).Encode(healthResponse{Status:"ok",Version:Version})
 })
}
func withSecurityHeaders(h http.Handler)http.Handler{
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Cache-Control","no-store")
  w.Header().Set("X-Content-Type-Options","nosniff")
  h.ServeHTTP(w,r)
 })
}

// Without a configured catalogue, the service remains health-check only.
func NewHandler()http.Handler{
 mux:=http.NewServeMux()
 mux.Handle("/api/v1/health",healthHandler())
 return withSecurityHeaders(mux)
}
