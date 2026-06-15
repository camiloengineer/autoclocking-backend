//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/camiloengineer/autoclocking-backend/internal/holiday"
)

func TestHolidayAPI_IsReachable(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", "https://api.boostr.cl/holidays.json", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to reach API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var apiResp holiday.APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if apiResp.Status != "success" {
		t.Fatalf("Expected status 'success', got '%s'", apiResp.Status)
	}

	if len(apiResp.Data) == 0 {
		t.Fatalf("Expected non-empty data array")
	}
}
