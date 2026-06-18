package marcajes

import (
	"context"
	"errors"
	"time"
)

const (
	DefaultLimit = 100
	MaxLimit     = 500
)

const (
	ActionEntrada = "ENTRADA"
	ActionSalida  = "SALIDA"
	ActionFeriado = "FERIADO"
)

const (
	StatusSuccess = "success"
	StatusError   = "error"
	StatusInfo    = "info"
)

type Store interface {
	AddMarcaje(context.Context, Record) (string, error)
	ListMarcajes(context.Context, int) ([]Record, error)
}

type Record struct {
	ID         string    `json:"id,omitempty" firestore:"-"`
	ActionType string    `json:"action_type" firestore:"action_type"`
	Status     string    `json:"status" firestore:"status"`
	Message    string    `json:"message" firestore:"message"`
	Details    string    `json:"details" firestore:"details"`
	RutMasked  string    `json:"rut_masked" firestore:"rut_masked"`
	RutKey     string    `json:"rut_key,omitempty" firestore:"rut_key,omitempty"`
	RunNumber  string    `json:"run_number" firestore:"run_number"`
	FechaCLT   string    `json:"fecha_clt" firestore:"fecha_clt"`
	CreatedAt  time.Time `json:"created_at" firestore:"created_at"`
}

type Response struct {
	Count int      `json:"count"`
	Items []Record `json:"items"`
}

func ValidateRecord(record Record) error {
	if !validAction(record.ActionType) || !validStatus(record.Status) {
		return errors.New("invalid action_type or status")
	}
	return nil
}

func validAction(actionType string) bool {
	switch actionType {
	case ActionEntrada, ActionSalida, ActionFeriado:
		return true
	default:
		return false
	}
}

func validStatus(status string) bool {
	switch status {
	case StatusSuccess, StatusError, StatusInfo:
		return true
	default:
		return false
	}
}
