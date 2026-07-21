package marcaje

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/camiloengineer/autoclocking-backend/internal/accounts"
	"github.com/camiloengineer/autoclocking-backend/internal/buk"
	"github.com/camiloengineer/autoclocking-backend/internal/circuitbreaker"
	"github.com/camiloengineer/autoclocking-backend/internal/config"
	"github.com/camiloengineer/autoclocking-backend/internal/delay"
	"github.com/camiloengineer/autoclocking-backend/internal/metrics"
	"github.com/camiloengineer/autoclocking-backend/internal/reporter"
	"github.com/camiloengineer/autoclocking-backend/internal/schedule"
)

const markTimeout = 60 * time.Second

type Service struct {
	reporter       *reporter.Reporter
	delayManager   *delay.Manager
	debugMode      bool
	execConfig     config.ExecutionConfig
	metrics        *metrics.Collector
	circuitBreaker *circuitbreaker.CircuitBreaker
}

type ExecutionResult struct {
	Message string
	Details string
	Status  string
}

func New(rep *reporter.Reporter, delayMgr *delay.Manager, debug bool, execCfg config.ExecutionConfig, m *metrics.Collector, cb *circuitbreaker.CircuitBreaker) *Service {
	return &Service{
		reporter:       rep,
		delayManager:   delayMgr,
		debugMode:      debug,
		execConfig:     execCfg,
		metrics:        m,
		circuitBreaker: cb,
	}
}

// ProcessAccount runs the full mark cycle for one Buk account with retry,
// circuit-breaker and delay guards. It returns true when the mark succeeded.
func (s *Service) ProcessAccount(account accounts.Account) bool {
	emailMasked := accounts.Mask(account.Email)
	startTime := time.Now()

	if !s.circuitBreaker.CanExecute() {
		slog.Warn("Circuit breaker OPEN - skipping account", "email", emailMasked)
		return false
	}

	slog.Info("STARTING ACCOUNT", "email", emailMasked)
	s.metrics.RecordRUTStart()

	maxAttempts := s.execConfig.RetryAttempts + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		success, err := s.processAttempt(account, emailMasked, attempt, maxAttempts)
		if success {
			duration := time.Since(startTime).Seconds()
			actionType := s.determineActionType()

			slog.Info("Account processed successfully", "email", emailMasked, "action_type", actionType, "duration", duration)
			s.metrics.RecordSuccess(duration)
			s.circuitBreaker.RecordSuccess()
			return true
		}

		slog.Error(fmt.Sprintf("Attempt %d/%d failed for %s: %v", attempt, maxAttempts, emailMasked, err))

		if attempt < maxAttempts {
			retryDelay := time.Duration(s.execConfig.RetryDelaySeconds) * time.Second
			slog.Info(fmt.Sprintf("Waiting %s before next attempt...", retryDelay))
			time.Sleep(retryDelay)
		}
	}

	s.circuitBreaker.RecordFailure()
	s.metrics.RecordError()
	return false
}

func (s *Service) processAttempt(account accounts.Account, emailMasked string, attempt, maxAttempts int) (bool, error) {
	slog.Info(fmt.Sprintf("Attempt %d/%d - Starting %s", attempt, maxAttempts, emailMasked))

	actionType := s.determineActionType()
	s.applyDelay(account.Email, actionType)

	slog.Info("EXECUTING marking", "email", emailMasked, "action_type", actionType)

	var result ExecutionResult
	var err error

	if s.debugMode {
		result = ExecutionResult{
			Message: debugSummary(actionType),
			Details: fmt.Sprintf("Debug mode active. No real %s was sent.", strings.ToLower(actionSummary(actionType))),
			Status:  "info",
		}
	} else {
		result, err = s.executeRealMarcaje(account, actionType)
		if err != nil {
			s.reporter.Report(actionType, "error", failureSummary(actionType), fmt.Sprintf("%s\n%v", emailMasked, err), emailMasked)
			return false, err
		}
	}

	slog.Info("MARKING COMPLETED", "email", emailMasked, "action_type", actionType, "status", result.Status)
	s.reporter.Report(actionType, result.Status, result.Message, result.Details, emailMasked)
	return true, nil
}

func (s *Service) applyDelay(email, actionType string) {
	if s.debugMode {
		slog.Info("DEBUG mode active: no delay", "email", accounts.Mask(email))
		return
	}

	delayMins := s.delayManager.GetRandomDelay(email)
	s.metrics.RecordDelayApplied()

	if actionType == "ENTRADA" {
		s.waitForEntryAnchor(email, delayMins)
		return
	}

	slog.Info(fmt.Sprintf("Applying delay of %d minutes for %s", delayMins, accounts.Mask(email)))
	time.Sleep(time.Duration(delayMins) * time.Minute)
	slog.Info(fmt.Sprintf("Delay completed for %s", accounts.Mask(email)))
}

func (s *Service) waitForEntryAnchor(email string, delayMins int) {
	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		slog.Error("Could not load America/Santiago timezone; applying delay from now", "error", err)
		time.Sleep(time.Duration(delayMins) * time.Minute)
		return
	}

	now := time.Now().In(loc)
	target := time.Date(now.Year(), now.Month(), now.Day(), schedule.EntryAnchorHour, schedule.EntryAnchorMin, 0, 0, loc).
		Add(time.Duration(delayMins) * time.Minute)

	wait := time.Until(target)
	if wait <= 0 {
		slog.Info(fmt.Sprintf("Entry anchor %s already passed; marking immediately for %s", target.Format("15:04"), accounts.Mask(email)))
		return
	}

	slog.Info(fmt.Sprintf("Waiting until %s (anchor 08:00 + %d min) for %s", target.Format("15:04"), delayMins, accounts.Mask(email)))
	time.Sleep(wait)
	slog.Info(fmt.Sprintf("Anchored wait completed for %s", accounts.Mask(email)))
}

func (s *Service) determineActionType() string {
	loc, _ := time.LoadLocation("America/Santiago")
	now := time.Now().In(loc)
	if now.Hour() >= 5 && now.Hour() < 12 {
		return "ENTRADA"
	}
	return "SALIDA"
}

func (s *Service) executeRealMarcaje(account accounts.Account, actionType string) (ExecutionResult, error) {
	client, err := buk.New()
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("buk client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), markTimeout)
	defer cancel()

	if err := client.Login(ctx, account.Email, account.Password); err != nil {
		return ExecutionResult{}, fmt.Errorf("login failed: %w", err)
	}

	portal, err := client.LoadPortal(ctx)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("load portal: %w", err)
	}
	if account.JobID != "" {
		portal.JobID = account.JobID
	}

	result, err := client.Mark(ctx, portal, actionType)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("mark: %w", err)
	}

	if result.Duplicate {
		return ExecutionResult{
			Message: duplicateSummary(actionType),
			Details: duplicateDetails(actionType),
			Status:  "info",
		}, nil
	}

	loc, _ := time.LoadLocation("America/Santiago")
	return ExecutionResult{
		Message: confirmedSummary(actionType),
		Details: confirmedDetails(time.Now().In(loc).Format("15:04:05")),
		Status:  "success",
	}, nil
}

func actionSummary(actionType string) string {
	if actionType == "ENTRADA" {
		return "Clock-in"
	}
	if actionType == "SALIDA" {
		return "Clock-out"
	}
	return "Clocking"
}

func confirmedSummary(actionType string) string {
	return fmt.Sprintf("%s confirmed", actionSummary(actionType))
}

func duplicateSummary(actionType string) string {
	return fmt.Sprintf("Duplicate %s prevented", strings.ToLower(actionSummary(actionType)))
}

func failureSummary(actionType string) string {
	return fmt.Sprintf("%s failed", actionSummary(actionType))
}

func debugSummary(actionType string) string {
	return fmt.Sprintf("%s debug simulation", actionSummary(actionType))
}

func confirmedDetails(timeLabel string) string {
	return fmt.Sprintf("Recorded at %s CLT.", timeLabel)
}

func duplicateDetails(actionType string) string {
	return fmt.Sprintf("%s was already registered for this direction.", actionSummary(actionType))
}
