package marcajesapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/camiloengineer/autoclocking-backend/internal/marcajes"
	"github.com/camiloengineer/autoclocking-backend/internal/ruts"
)

type Server struct {
	store    marcajes.Store
	rutStore ruts.Store
}

func NewServer(store marcajes.Store, rutStore ruts.Store) *Server {
	return &Server{store: store, rutStore: rutStore}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		writeCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.URL.Path == "/marcajes" {
		s.handleMarcajes(w, r)
		return
	}

	if r.URL.Path == "/ruts" || strings.HasPrefix(r.URL.Path, "/ruts/") {
		s.handleRUTs(w, r)
		return
	}

	http.NotFound(w, r)
}

func (s *Server) handleMarcajes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleList(w, r)
	case http.MethodPost:
		s.handleCreate(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not supported"})
	}
}

func (s *Server) handleRUTs(w http.ResponseWriter, r *http.Request) {
	rawRUT := strings.TrimPrefix(r.URL.Path, "/ruts/")
	if rawRUT == r.URL.Path {
		rawRUT = ""
	}

	switch r.Method {
	case http.MethodGet:
		if rawRUT != "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "RUT endpoint not found"})
			return
		}
		s.handleListRUTs(w, r)
	case http.MethodPost:
		if rawRUT != "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "RUT endpoint not found"})
			return
		}
		s.handleSaveRUT(w, r)
	case http.MethodPatch:
		if rawRUT == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "RUT is required"})
			return
		}
		s.handleUpdateRUT(w, r, rawRUT)
	case http.MethodDelete:
		if rawRUT == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "RUT is required"})
			return
		}
		s.handleDeleteRUT(w, r, rawRUT)
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

type rutPayload struct {
	RUT    string `json:"rut"`
	Active *bool  `json:"active"`
}

func (s *Server) handleListRUTs(w http.ResponseWriter, r *http.Request) {
	items, err := s.rutStore.ListRUTs(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list RUTs"})
		return
	}

	writeJSON(w, http.StatusOK, ruts.Response{
		Count: len(items),
		Items: items,
	})
}

func (s *Server) handleSaveRUT(w http.ResponseWriter, r *http.Request) {
	payload, ok := decodeRUTPayload(w, r)
	if !ok {
		return
	}

	active := true
	if payload.Active != nil {
		active = *payload.Active
	}

	record, err := ruts.NewRecord(payload.RUT, active)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	saved, err := s.rutStore.SaveRUT(r.Context(), record)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to save RUT"})
		return
	}

	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handleUpdateRUT(w http.ResponseWriter, r *http.Request, rawRUT string) {
	payload, ok := decodeRUTPayload(w, r)
	if !ok {
		return
	}
	if payload.Active == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "active is required"})
		return
	}

	record, err := ruts.NewRecord(rawRUT, *payload.Active)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	saved, err := s.rutStore.SaveRUT(r.Context(), record)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update RUT"})
		return
	}

	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteRUT(w http.ResponseWriter, r *http.Request, rawRUT string) {
	if err := s.rutStore.DeleteRUT(r.Context(), rawRUT); err != nil {
		if errors.Is(err, ruts.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "RUT not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete RUT"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func decodeRUTPayload(w http.ResponseWriter, r *http.Request) (rutPayload, bool) {
	var payload rutPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON body is required"})
		return rutPayload{}, false
	}

	return payload, true
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
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}
