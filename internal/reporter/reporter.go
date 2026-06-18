package reporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

const (
	DefaultAPIURL         = "https://marcajes-vg7vvkcauq-ue.a.run.app"
	RequestTimeoutSeconds = 15
)

// Reporter sends attendance marking results to the Cloud Function.
type Reporter struct {
	apiURL string
	client *http.Client
}

// New creates a new Reporter.
func New() *Reporter {
	apiURL := os.Getenv("MARCAJE_API_URL")
	if apiURL == "" {
		apiURL = DefaultAPIURL
	}

	return &Reporter{
		apiURL: apiURL,
		client: &http.Client{
			Timeout: RequestTimeoutSeconds * time.Second,
		},
	}
}

// Payload represents the JSON body sent to the API.
type Payload struct {
	ActionType string `json:"action_type"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	Details    string `json:"details,omitempty"`
	RutMasked  string `json:"rut_masked"`
	RunNumber  string `json:"run_number"`
	FechaCLT   string `json:"fecha_clt"`
}

// Report posts a result to the API. Returns true if persisted.
func (r *Reporter) Report(actionType, status, message, details, rutMasked string) bool {
	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		slog.Error("Failed to load timezone", "error", err)
		loc = time.UTC
	}

	now := time.Now().In(loc).Format("2006-01-02 15:04:05")
	runNumber := os.Getenv("GITHUB_RUN_NUMBER")

	payload := Payload{
		ActionType: actionType,
		Status:     status,
		Message:    message,
		Details:    details,
		RutMasked:  rutMasked,
		RunNumber:  runNumber,
		FechaCLT:   now,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		slog.Error("Error marshaling payload", "error", err)
		return false
	}

	req, err := http.NewRequest("POST", r.apiURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		slog.Error("Error creating request", "error", err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		slog.Error("Error reporting to API", "error", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated {
		slog.Info(fmt.Sprintf("Marcaje reported to API for RUT %s (%s/%s)", rutMasked, actionType, status))
		return true
	}

	slog.Error(fmt.Sprintf("API responded %d when reporting marcaje", resp.StatusCode))
	return false
}
