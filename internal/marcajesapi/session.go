package marcajesapi

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"

	"github.com/camiloengineer/autoclocking-backend/internal/accounts"
	"github.com/camiloengineer/autoclocking-backend/internal/buk"
)

type sessionPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type sessionResponse struct {
	Email   string `json:"email"`
	JobID   string `json:"job_id"`
	Active  bool   `json:"active"`
	IsAdmin bool   `json:"is_admin"`
}

// handleCreateSession is the single entry point for users: it throttles
// repeated failures, validates the credentials live against Buk, upserts the
// account (creation IS the login) and reports whether the email is an admin.
func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not supported"})
		return
	}

	var payload sessionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON body is required"})
		return
	}

	email := accounts.NormalizeEmail(payload.Email)
	if email == "" || strings.TrimSpace(payload.Password) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password are required"})
		return
	}

	if wait, blocked := s.throttle.Check(email); blocked {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":               "Demasiados intentos fallidos; espera antes de volver a intentarlo",
			"code":                "throttled",
			"retry_after_seconds": int(math.Ceil(wait.Seconds())),
		})
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
			attemptsLeft := s.throttle.Fail(email)
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error":         "Email o contraseña de Buk incorrectos",
				"code":          "invalid_credentials",
				"attempts_left": attemptsLeft,
			})
		case errors.Is(err, buk.ErrLocked):
			s.throttle.Reset(email)
			writeJSON(w, http.StatusLocked, map[string]string{"error": "Cuenta bloqueada por Buk; revisa tu correo para desbloquearla", "code": "account_locked"})
		default:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "No se pudo validar contra Buk", "code": "buk_unreachable"})
		}
		return
	}
	s.throttle.Reset(email)

	portal, err := client.LoadPortal(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "No se pudo leer el portal de Buk", "code": "buk_unreachable"})
		return
	}

	active, err := s.resolveActiveFlag(r, email)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read accounts"})
		return
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

	writeJSON(w, http.StatusOK, sessionResponse{
		Email:   saved.Email,
		JobID:   saved.JobID,
		Active:  saved.Active,
		IsAdmin: s.adminEmails[saved.Email],
	})
}

// resolveActiveFlag keeps the stored active flag when the account already
// exists so a re-login never re-enables a deliberately paused account; new
// accounts start enabled.
func (s *Server) resolveActiveFlag(r *http.Request, email string) (bool, error) {
	existing, err := s.accountStore.List(r.Context())
	if err != nil {
		return false, err
	}
	for _, item := range existing {
		if item.Email == email {
			return item.Active, nil
		}
	}
	return true, nil
}
