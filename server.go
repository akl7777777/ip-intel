package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// Server is the HTTP server.
type Server struct {
	service *Service
	authKey string
	mux     *http.ServeMux
}

// NewServer creates a new HTTP server.
func NewServer(svc *Service, authKey string) *Server {
	s := &Server{
		service: svc,
		authKey: authKey,
		mux:     http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/v1/lookup/", s.handleLookup)
	s.mux.HandleFunc("/api/v1/health", s.handleHealth)
	s.mux.HandleFunc("/api/v1/stats", s.handleStats)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Auth check (skip for health endpoint)
	if s.authKey != "" && r.URL.Path != "/api/v1/health" {
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "" || token == auth {
			// No Bearer prefix, try raw value
			token = auth
		}
		if token != s.authKey {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			log.Printf("[http] %s %s 401 unauthorized %s", r.Method, r.URL.Path, time.Since(start))
			return
		}
	}

	s.mux.ServeHTTP(w, r)

	log.Printf("[http] %s %s %s", r.Method, r.URL.Path, time.Since(start))
}

func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract IP from path: /api/v1/lookup/{ip}
	ip := strings.TrimPrefix(r.URL.Path, "/api/v1/lookup/")
	ip = strings.TrimSpace(ip)

	if ip == "" {
		writeError(w, http.StatusBadRequest, "IP address required")
		return
	}

	// Validate IP format
	if net.ParseIP(ip) == nil {
		writeError(w, http.StatusBadRequest, "invalid IP address format")
		return
	}

	// Skip private/reserved IPs
	if isPrivateIP(ip) {
		writeJSON(w, http.StatusOK, &IPInfo{
			IP:     ip,
			Source: "private",
		})
		return
	}

	info, err := s.service.Lookup(ip)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.service.Stats())
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, &ErrorResponse{
		Error: msg,
		Code:  status,
	})
}

func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	privateRanges := []struct {
		network string
	}{
		{"10.0.0.0/8"},
		{"172.16.0.0/12"},
		{"192.168.0.0/16"},
		{"127.0.0.0/8"},
		{"::1/128"},
		{"fc00::/7"},
		{"fe80::/10"},
	}

	for _, r := range privateRanges {
		_, cidr, _ := net.ParseCIDR(r.network)
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
