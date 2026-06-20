package schedule

import (
	"log/slog"
	"os"
	"time"
)

// EntryAnchorHour and EntryAnchorMin define the fixed local target time
// (America/Santiago) the entry marking is anchored to. The random delay is
// added on top of this anchor so GitHub Actions scheduling lateness never
// accumulates onto the marking time.
const (
	EntryAnchorHour = 8
	EntryAnchorMin  = 0
)

// window is a tolerated local-time range for a scheduled cron to fire. The wide
// range absorbs GitHub Actions scheduling lateness so a delayed cron is still
// recognized as the correct shift instead of being skipped silently.
type window struct {
	startHour, startMin int
	endHour, endMin     int
}

var windows = []window{
	{startHour: 7, startMin: 40, endHour: 8, endMin: 30},
	{startHour: 17, startMin: 5, endHour: 17, endMin: 55},
}

// ShouldRun validates dynamically if the cron corresponds to a Chile time window.
func ShouldRun() bool {
	if os.Getenv("GITHUB_EVENT_NAME") != "schedule" {
		return true
	}

	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		slog.Error("Could not load America/Santiago timezone", "error", err)
		return false
	}

	now := time.Now().In(loc)

	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		slog.Info("Weekend: marking skipped.", "weekday", now.Weekday().String())
		return false
	}

	for _, w := range windows {
		if w.contains(now) {
			return true
		}
	}

	slog.Info("Cron fired outside local window. Execution skipped.", "time", now.Format("15:04"))
	return false
}

func (w window) contains(now time.Time) bool {
	start := time.Date(now.Year(), now.Month(), now.Day(), w.startHour, w.startMin, 0, 0, now.Location())
	end := time.Date(now.Year(), now.Month(), now.Day(), w.endHour, w.endMin, 0, 0, now.Location())
	return !now.Before(start) && !now.After(end)
}
