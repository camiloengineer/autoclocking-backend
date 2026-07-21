package main

import (
	"context"
	"log/slog"
	"os"
	"sync"

	"github.com/camiloengineer/autoclocking-backend/internal/accounts"
	"github.com/camiloengineer/autoclocking-backend/internal/accountsstore"
	"github.com/camiloengineer/autoclocking-backend/internal/circuitbreaker"
	"github.com/camiloengineer/autoclocking-backend/internal/config"
	"github.com/camiloengineer/autoclocking-backend/internal/delay"
	"github.com/camiloengineer/autoclocking-backend/internal/holiday"
	"github.com/camiloengineer/autoclocking-backend/internal/marcaje"
	"github.com/camiloengineer/autoclocking-backend/internal/metrics"
	"github.com/camiloengineer/autoclocking-backend/internal/reporter"
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

	ctx := context.Background()
	store, closeStore, err := accountsstore.FromEnv(ctx)
	if err != nil {
		slog.Error("Failed to build accounts store", "error", err)
		os.Exit(1)
	}
	defer func() { _ = closeStore() }()

	all, err := store.List(ctx)
	if err != nil {
		slog.Error("Failed to load accounts", "error", err)
		os.Exit(1)
	}

	active := make([]accounts.Account, 0, len(all))
	maskedEmails := make([]string, 0, len(all))
	for _, account := range all {
		if account.Active {
			active = append(active, account)
			maskedEmails = append(maskedEmails, accounts.Mask(account.Email))
		}
	}

	if len(active) == 0 {
		slog.Info("No active accounts to process. Terminating execution.")
		os.Exit(0)
	}

	rep := reporter.New()
	holidaySvc := holiday.New(rep, len(active), maskedEmails)

	if holidaySvc.IsHoliday() {
		os.Exit(0)
	}

	delayMgr := delay.New()
	metricsCol := metrics.New()
	cb := circuitbreaker.New(cfg.Execution.CircuitBreakerThreshold, cfg.Execution.RetryDelaySeconds*2)

	marcajeSvc := marcaje.New(rep, delayMgr, cfg.DebugMode, cfg.Execution, metricsCol, cb)

	processAccounts(active, marcajeSvc, cfg, metricsCol)
}

func processAccounts(active []accounts.Account, marcajeSvc *marcaje.Service, cfg *config.Config, metricsCol *metrics.Collector) {
	slog.Info("Starting parallel processing", "total_accounts", len(active))

	var wg sync.WaitGroup

	for _, account := range active {
		wg.Add(1)

		go func(a accounts.Account) {
			defer wg.Done()
			marcajeSvc.ProcessAccount(a)
		}(account)
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
