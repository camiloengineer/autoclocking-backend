//go:build integration

package integration

import (
	"testing"

	"github.com/camiloengineer/autoclocking-backend/internal/reporter"
)

func TestReporter_Report(t *testing.T) {
	r := reporter.New()
	
	success := r.Report("ENTRADA", "success", "test-go-migration", "1234****")
	if !success {
		t.Errorf("Expected Report to return true, got false")
	}
}
