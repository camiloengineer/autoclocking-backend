package marcajesapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

const (
	DefaultLimit = 100
	MaxLimit     = 500
)

var (
	validActions = map[string]struct{}{
		"ENTRADA": {},
		"SALIDA":  {},
		"FERIADO": {},
	}
	validStatuses = map[string]struct{}{
		"success": {},
		"error":   {},
		"info":    {},
	}
)

type Store interface {
	AddMarcaje(context.Context, MarcajeRecord) (string, error)
	ListMarcajes(context.Context, int) ([]MarcajeRecord, error)
}

type Server struct {
	store Store
}

type MarcajeRecord struct {
	ID         string    `json:"id,omitempty" firestore:"-"`
	ActionType string    `json:"action_type" firestore:"action_type"`
	Status     string    `json:"status" firestore:"status"`
	Message    string    `json:"message" firestore:"message"`
	Details    string    `json:"details" firestore:"details"`
	RutMasked  string    `json:"rut_masked" firestore:"rut_masked"`
	RunNumber  string    `json:"run_number" firestore:"run_number"`
	FechaCLT   string    `json:"fecha_clt" firestore:"fecha_clt"`
	CreatedAt  time.Time `json:"created_at" firestore:"created_at"`
}

type marcajesResponse struct {
	Count int             `json:"count"`
	Items []MarcajeRecord `json:"items"`
}

func NewServer(store Store) *Server {
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
	var payload MarcajeRecord
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON body is required"})
		return
	}

	if err := validatePayload(payload); err != nil {
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

	writeJSON(w, http.StatusOK, marcajesResponse{
		Count: len(items),
		Items: items,
	})
}

func parseLimit(raw string) int {
	if raw == "" {
		return DefaultLimit
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return DefaultLimit
	}

	if value < 1 {
		return 1
	}
	if value > MaxLimit {
		return MaxLimit
	}
	return value
}

func validatePayload(payload MarcajeRecord) error {
	if _, ok := validActions[payload.ActionType]; !ok {
		return errors.New("Invalid action_type or status")
	}
	if _, ok := validStatuses[payload.Status]; !ok {
		return errors.New("Invalid action_type or status")
	}
	return nil
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
