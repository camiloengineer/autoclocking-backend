package schedule

import (
	"log/slog"
	"math"
	"os"
	"time"
)

// Target represents a target hour and minute for the schedule.
type Target struct {
	Hour   int
	Minute int
}

var Targets = []Target{
	{8, 10},
	{17, 30},
}

const ToleranceMinutes = 25

// ShouldRun validates dynamically if the cron corresponds to the Chile time window.
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

	if isWithinWindow(now) {
		return true
	}

	slog.Info("Cron fired outside local window. Execution skipped.", "time", now.Format("15:04"))
	return false
}

func isWithinWindow(now time.Time) bool {
	for _, t := range Targets {
		target := time.Date(now.Year(), now.Month(), now.Day(), t.Hour, t.Minute, 0, 0, now.Location())
		diffMinutes := math.Abs(now.Sub(target).Seconds()) / 60
		if diffMinutes <= ToleranceMinutes {
			return true
		}
	}
	return false
}
