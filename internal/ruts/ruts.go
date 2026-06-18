package ruts

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/camiloengineer/autoclocking-backend/internal/rut"
)

var ErrNotFound = errors.New("rut not found")

type Store interface {
	SeedRUTs(context.Context, []Record) error
	ListRUTs(context.Context) ([]Record, error)
	SaveRUT(context.Context, Record) (Record, error)
	DeleteRUT(context.Context, string) error
}

type Record struct {
	RUT       string    `json:"rut" firestore:"rut"`
	Active    bool      `json:"active" firestore:"active"`
	CreatedAt time.Time `json:"created_at" firestore:"created_at"`
	UpdatedAt time.Time `json:"updated_at" firestore:"updated_at"`
}

type Response struct {
	Count int      `json:"count"`
	Items []Record `json:"items"`
}

func NewRecord(value string, active bool) (Record, error) {
	normalized := Normalize(value)
	if !rut.IsValid(normalized) {
		return Record{}, errors.New("invalid rut")
	}

	now := time.Now().UTC()
	return Record{
		RUT:       normalized,
		Active:    active,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func Normalize(value string) string {
	return strings.ToLower(strings.NewReplacer(".", "", "-", "", " ", "").Replace(value))
}

func SeedRecords(values []string) ([]Record, error) {
	seen := make(map[string]struct{}, len(values))
	records := make([]Record, 0, len(values))

	for _, value := range values {
		record, err := NewRecord(value, true)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[record.RUT]; exists {
			continue
		}
		seen[record.RUT] = struct{}{}
		records = append(records, record)
	}

	return records, nil
}
