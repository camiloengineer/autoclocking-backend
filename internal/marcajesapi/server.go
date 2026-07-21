package marcajesapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/camiloengineer/autoclocking-backend/internal/accounts"
	"github.com/camiloengineer/autoclocking-backend/internal/buk"
	"github.com/camiloengineer/autoclocking-backend/internal/marcajes"
)

type Server struct {
	store        marcajes.Store
	accountStore accounts.Store
}

func NewServer(store marcajes.Store, accountStore accounts.Store) *Server {
	return &Server{store: store, accountStore: accountStore}
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

	if r.URL.Path == "/accounts" {
		s.handleAccounts(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/accounts/") {
		s.handleAccountByEmail(w, r)
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

func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListAccounts(w, r)
	case http.MethodPost:
		s.handleCreateAccount(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not supported"})
	}
}

func (s *Server) handleAccountByEmail(w http.ResponseWriter, r *http.Request) {
	rawEmail := strings.TrimPrefix(r.URL.Path, "/accounts/")
	if rawEmail == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email is required"})
		return
	}

	switch r.Method {
	case http.MethodPatch:
		s.handleUpdateAccount(w, r, rawEmail)
	case http.MethodDelete:
		s.handleDeleteAccount(w, r, rawEmail)
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

type accountPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Active   *bool  `json:"active"`
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	items, err := s.accountStore.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list accounts"})
		return
	}

	views := make([]accounts.View, 0, len(items))
	for _, item := range items {
		views = append(views, item.Redact())
	}

	writeJSON(w, http.StatusOK, accounts.Response{Count: len(views), Items: views})
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var payload accountPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON body is required"})
		return
	}

	email := accounts.NormalizeEmail(payload.Email)
	if email == "" || strings.TrimSpace(payload.Password) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password are required"})
		return
	}

	client, err := buk.New()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to build Buk client"})
		return
	}

	if err := client.Login(r.Context(), email, payload.Password); err != nil {
		switch {
		case errors.Is(err, buk.ErrInvalidCredentials):
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Email o contraseña de Buk incorrectos", "code": "invalid_credentials"})
		case errors.Is(err, buk.ErrLocked):
			writeJSON(w, http.StatusLocked, map[string]string{"error": "Cuenta bloqueada por Buk; revisa tu correo para desbloquearla", "code": "account_locked"})
		default:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "No se pudo validar contra Buk", "code": "buk_unreachable"})
		}
		return
	}

	portal, err := client.LoadPortal(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "No se pudo leer el portal de Buk", "code": "buk_unreachable"})
		return
	}

	active := true
	if payload.Active != nil {
		active = *payload.Active
	}

	account, err := accounts.NewAccount(email, payload.Password, active)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	account.JobID = portal.JobID

	saved, err := s.accountStore.Save(r.Context(), account)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to save account"})
		return
	}

	writeJSON(w, http.StatusCreated, saved.Redact())
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request, rawEmail string) {
	var payload accountPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON body is required"})
		return
	}
	if payload.Active == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "active is required"})
		return
	}

	saved, err := s.accountStore.SetActive(r.Context(), rawEmail, *payload.Active)
	if err != nil {
		if errors.Is(err, accounts.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Account not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update account"})
		return
	}

	writeJSON(w, http.StatusOK, saved.Redact())
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request, rawEmail string) {
	if err := s.accountStore.Delete(r.Context(), rawEmail); err != nil {
		if errors.Is(err, accounts.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Account not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete account"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
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
