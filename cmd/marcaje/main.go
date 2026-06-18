package main

import (
	"log/slog"
	"os"
	"sync"

	"github.com/camiloengineer/autoclocking-backend/internal/circuitbreaker"
	"github.com/camiloengineer/autoclocking-backend/internal/config"
	"github.com/camiloengineer/autoclocking-backend/internal/delay"
	"github.com/camiloengineer/autoclocking-backend/internal/holiday"
	"github.com/camiloengineer/autoclocking-backend/internal/marcaje"
	"github.com/camiloengineer/autoclocking-backend/internal/metrics"
	"github.com/camiloengineer/autoclocking-backend/internal/reporter"
	"github.com/camiloengineer/autoclocking-backend/internal/rut"
	"github.com/camiloengineer/autoclocking-backend/internal/schedule"
)

func main() {
	setupLogging()
	slog.Info("Starting attendance marking service...")

	if !schedule.ShouldRun() {
		os.Exit(0)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("Configuration error", "error", err)
		os.Exit(1)
	}

	if !cfg.ClockInActive {
		slog.Info("CLOCK_IN_ACTIVE is false or not configured. Terminating execution.")
		os.Exit(0)
	}

	var maskedRuts []string
	for _, r := range cfg.ActiveRUTs {
		if !rut.IsValid(r) {
			slog.Error("Configuration contains invalid RUT", "rut", rut.Mask(r))
			os.Exit(1)
		}
		maskedRuts = append(maskedRuts, rut.Mask(r))
	}

	rep := reporter.New()
	holidaySvc := holiday.New(rep, len(cfg.ActiveRUTs), maskedRuts)

	if holidaySvc.IsHoliday() {
		os.Exit(0)
	}

	delayMgr := delay.New()
	metricsCol := metrics.New()
	cb := circuitbreaker.New(cfg.Execution.CircuitBreakerThreshold, cfg.Execution.RetryDelaySeconds*2)

	marcajeSvc := marcaje.New(rep, delayMgr, cfg.DebugMode, cfg.Execution, metricsCol, cb)

	processRUTs(cfg, marcajeSvc, metricsCol)
}

func processRUTs(cfg *config.Config, marcajeSvc *marcaje.Service, metricsCol *metrics.Collector) {
	slog.Info("Starting parallel processing", "total_ruts", len(cfg.ActiveRUTs))

	var wg sync.WaitGroup

	for _, r := range cfg.ActiveRUTs {
		wg.Add(1)

		go func(r string) {
			defer wg.Done()
			marcajeSvc.ProcessRUT(r)
		}(r)
	}

	wg.Wait()

	if cfg.Execution.EnableMetrics {
		summary := metricsCol.Summary()
		slog.Info("Execution metrics", "metrics", summary)
	}

	slog.Info("Processing completed")
}

func setupLogging() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
}
