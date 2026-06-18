package marcajesapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/camiloengineer/autoclocking-backend/internal/marcajes"
)

type Server struct {
	store marcajes.Store
}

func NewServer(store marcajes.Store) *Server {
	return &Server{store: store}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/marcajes" {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodOptions {
		writeCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleList(w, r)
	case http.MethodPost:
		s.handleCreate(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not supported"})
	}
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var payload marcajes.Record
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON body is required"})
		return
	}

	if err := marcajes.ValidateRecord(payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	payload.CreatedAt = time.Now().UTC()
	id, err := s.store.AddMarcaje(r.Context(), payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to persist marcaje"})
		return
	}

	payload.ID = id
	writeJSON(w, http.StatusCreated, payload)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r.URL.Query().Get("limit"))
	items, err := s.store.ListMarcajes(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list marcajes"})
		return
	}

	writeJSON(w, http.StatusOK, marcajes.Response{
		Count: len(items),
		Items: items,
	})
}

func parseLimit(raw string) int {
	if raw == "" {
		return marcajes.DefaultLimit
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return marcajes.DefaultLimit
	}

	if value < 1 {
		return 1
	}
	if value > marcajes.MaxLimit {
		return marcajes.MaxLimit
	}
	return value
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	writeCORS(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}
