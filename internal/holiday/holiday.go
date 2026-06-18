package holiday

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/camiloengineer/autoclocking-backend/internal/reporter"
)

// Service checks for holidays in Chile.
type Service struct {
	reporter         *reporter.Reporter
	activeRutsCount  int
	activeRutsMasked []string
	apiURL           string
	httpClient       *http.Client
}

// New creates a new Holiday Service.
func New(rep *reporter.Reporter, activeRutsCount int, activeRutsMasked []string) *Service {
	return &Service{
		reporter:         rep,
		activeRutsCount:  activeRutsCount,
		activeRutsMasked: activeRutsMasked,
		apiURL:           "https://api.boostr.cl/holidays.json",
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type APIResponse struct {
	Status string    `json:"status"`
	Data   []Holiday `json:"data"`
}

type Holiday struct {
	Date  string `json:"date"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// IsHoliday checks if today is a holiday.
func (s *Service) IsHoliday() bool {
	slog.Info("Checking if today is a holiday...")

	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		slog.Error("Could not load America/Santiago timezone", "error", err)
		loc = time.UTC
	}
	today := time.Now().In(loc).Format("2006-01-02")

	// Try online API first
	if holiday, ok := s.checkOnlineAPI(today); ok {
		slog.Info(fmt.Sprintf("🎉 Today is a holiday!: %s (%s)", holiday.Title, holiday.Type))
		s.reportHoliday(holiday, "API")
		return true
	}

	// Fallback to local dynamic list
	if holiday, ok := s.checkLocalHolidays(today, time.Now().In(loc).Year()); ok {
		slog.Info(fmt.Sprintf("🎉 Today is a holiday! (local list): %s (%s)", holiday.Title, holiday.Type))
		s.reportHoliday(holiday, "LOCAL")
		return true
	}

	slog.Info("✅ Not a holiday, continuing with attendance marking")
	return false
}

func (s *Service) reportHoliday(holiday Holiday, source string) {
	maskedStr := strings.Join(s.activeRutsMasked, ", ")
	details := fmt.Sprintf("Holiday: %s (%s). Source: %s. Configured RUTs: %d - [%s]",
		holiday.Title, holiday.Type, source, s.activeRutsCount, maskedStr)

	s.reporter.Report("FERIADO", "info", "Holiday detected", details, maskedStr, "")
}

func (s *Service) checkOnlineAPI(today string) (Holiday, bool) {
	slog.Info("🌐 Querying online holidays API...")

	req, err := http.NewRequest("GET", s.apiURL, nil)
	if err != nil {
		slog.Warn("Failed to create request for holiday API", "error", err)
		return Holiday{}, false
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		slog.Warn("Holidays API unavailable", "error", err)
		return Holiday{}, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn(fmt.Sprintf("API returned status code: %d", resp.StatusCode))
		return Holiday{}, false
	}

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		slog.Warn("Error decoding holiday API response", "error", err)
		return Holiday{}, false
	}

	if apiResp.Status != "success" {
		slog.Warn(fmt.Sprintf("API returned unsuccessful status: %s", apiResp.Status))
		return Holiday{}, false
	}

	for _, h := range apiResp.Data {
		if h.Date == today {
			return h, true
		}
	}

	slog.Info("📅 Not a holiday according to online API")
	return Holiday{}, false
}

func (s *Service) checkLocalHolidays(today string, currentYear int) (Holiday, bool) {
	slog.Info("📋 Checking local fixed-date holidays list...")

	fixedHolidays := []Holiday{
		{Date: fmt.Sprintf("%d-01-01", currentYear), Title: "New Year's Day", Type: "Civil"},
		{Date: fmt.Sprintf("%d-05-01", currentYear), Title: "National Labor Day", Type: "Civil"},
		{Date: fmt.Sprintf("%d-05-21", currentYear), Title: "Navy Glories Day", Type: "Civil"},
		{Date: fmt.Sprintf("%d-07-16", currentYear), Title: "Virgin of Carmen Day", Type: "Religious"},
		{Date: fmt.Sprintf("%d-08-15", currentYear), Title: "Assumption of Mary", Type: "Religious"},
		{Date: fmt.Sprintf("%d-09-18", currentYear), Title: "National Independence Day", Type: "Civil"},
		{Date: fmt.Sprintf("%d-09-19", currentYear), Title: "Army Glories Day", Type: "Civil"},
		{Date: fmt.Sprintf("%d-10-31", currentYear), Title: "Evangelical and Protestant Churches Day", Type: "Religious"},
		{Date: fmt.Sprintf("%d-11-01", currentYear), Title: "All Saints' Day", Type: "Religious"},
		{Date: fmt.Sprintf("%d-12-08", currentYear), Title: "Immaculate Conception", Type: "Religious"},
		{Date: fmt.Sprintf("%d-12-25", currentYear), Title: "Christmas Day", Type: "Religious"},
	}

	for _, h := range fixedHolidays {
		if h.Date == today {
			return h, true
		}
	}

	return Holiday{}, false
}
