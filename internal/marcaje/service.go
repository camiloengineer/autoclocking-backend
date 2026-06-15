package marcaje

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/camiloengineer/autoclocking-backend/internal/circuitbreaker"
	"github.com/camiloengineer/autoclocking-backend/internal/config"
	"github.com/camiloengineer/autoclocking-backend/internal/delay"
	"github.com/camiloengineer/autoclocking-backend/internal/metrics"
	"github.com/camiloengineer/autoclocking-backend/internal/reporter"
	"github.com/camiloengineer/autoclocking-backend/internal/rut"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

type Service struct {
	reporter       *reporter.Reporter
	delayManager   *delay.Manager
	debugMode      bool
	execConfig     config.ExecutionConfig
	metrics        *metrics.Collector
	circuitBreaker *circuitbreaker.CircuitBreaker
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

func (s *Service) ProcessRUT(rutStr string) bool {
	rutMasked := rut.Mask(rutStr)
	startTime := time.Now()

	if !s.circuitBreaker.CanExecute() {
		slog.Warn("Circuit breaker OPEN - skipping RUT", "rut", rutMasked)
		return false
	}

	slog.Info("STARTING RUT", "rut", rutMasked)
	s.metrics.RecordRUTStart()

	maxAttempts := s.execConfig.RetryAttempts + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		success, err := s.processAttempt(rutStr, rutMasked, attempt, maxAttempts)
		if success {
			duration := time.Since(startTime).Seconds()
			actionType := s.determineActionType()

			slog.Info("RUT processed successfully", "rut", rutMasked, "action_type", actionType, "duration", duration)
			s.metrics.RecordSuccess(duration)
			s.circuitBreaker.RecordSuccess()
			return true
		}

		slog.Error(fmt.Sprintf("Attempt %d/%d failed for RUT %s: %v", attempt, maxAttempts, rutMasked, err))

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

func (s *Service) processAttempt(rutStr, rutMasked string, attempt, maxAttempts int) (bool, error) {
	slog.Info(fmt.Sprintf("Attempt %d/%d - Starting RUT %s", attempt, maxAttempts, rutMasked))

	s.applyDelay(rutStr)

	actionType := s.determineActionType()
	slog.Info("EXECUTING marking", "rut", rutMasked, "action_type", actionType)

	var message string
	var err error

	if s.debugMode {
		message = fmt.Sprintf("🧪 DEBUG active: no real marking executed. Type: %s, Chile Time: %s", actionType, time.Now().Format("15:04:05"))
	} else {
		message, err = s.executeRealMarcaje(rutStr, actionType)
		if err != nil {
			s.reporter.Report(actionType, "error", fmt.Sprintf("❌ Error marking for RUT %s:\n%v", rutMasked, err), rutMasked)
			return false, err
		}
	}

	slog.Info("MARKING COMPLETED", "rut", rutMasked, "action_type", actionType)
	s.reporter.Report(actionType, "success", message, rutMasked)
	return true, nil
}

func (s *Service) applyDelay(rutStr string) {
	if !s.debugMode {
		delayMins := s.delayManager.GetRandomDelay(rutStr)
		slog.Info(fmt.Sprintf("Applying delay of %d minutes for RUT %s", delayMins, rut.Mask(rutStr)))
		s.metrics.RecordDelayApplied()
		time.Sleep(time.Duration(delayMins) * time.Minute)
		slog.Info(fmt.Sprintf("Delay completed for RUT %s", rut.Mask(rutStr)))
	} else {
		slog.Info("DEBUG mode active: no delay", "rut", rut.Mask(rutStr))
	}
}

func (s *Service) determineActionType() string {
	loc, _ := time.LoadLocation("America/Santiago")
	now := time.Now().In(loc)
	if now.Hour() >= 5 && now.Hour() < 12 {
		return "ENTRADA"
	}
	return "SALIDA"
}

func (s *Service) executeRealMarcaje(rutStr, actionType string) (string, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-software-rasterizer", true),
		chromedp.WindowSize(1920, 1080),
		chromedp.Flag("disable-geolocation", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-plugins", true),
		chromedp.Flag("disable-images", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx,
		chromedp.Navigate("https://app.ctrlit.cl/ctrl/dial/web/K1NBpBqyjf"),
		chromedp.WaitVisible(`#dial button`, chromedp.ByQuery),
	); err != nil {
		return "", fmt.Errorf("failed to navigate and wait for action buttons: %w", err)
	}

	// Disable geolocation
	err := chromedp.Run(ctx, chromedp.Evaluate(`
		navigator.geolocation.getCurrentPosition = function(success, error) {
			if (error) error({ code: 1, message: 'User denied Geolocation' });
		};
		navigator.geolocation.watchPosition = function() { return null; };
	`, nil))
	if err != nil {
		slog.Warn("Failed to disable geolocation via JS", "error", err)
	}

	err = chromedp.Run(ctx, emulation.SetGeolocationOverride().WithLatitude(0).WithLongitude(0).WithAccuracy(0))
	if err != nil {
		slog.Warn("Failed to set geolocation override via CDP", "error", err)
	}

	time.Sleep(2 * time.Second)

	// Click action button
	err = s.clickActionButton(ctx, actionType)
	if err != nil {
		return "", fmt.Errorf("failed to click action button: %w", err)
	}

	if err := chromedp.Run(ctx, chromedp.WaitVisible(`li.digits`, chromedp.ByQuery)); err != nil {
		return "", fmt.Errorf("failed to wait for RUT keypad after action: %w", err)
	}

	// Enter RUT
	err = s.enterRUT(ctx, rutStr)
	if err != nil {
		return "", fmt.Errorf("failed to enter RUT: %w", err)
	}

	// Submit form
	err = s.submitForm(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to submit form: %w", err)
	}

	loc, _ := time.LoadLocation("America/Santiago")
	now := time.Now().In(loc)

	msg := fmt.Sprintf("✅ %s successful at %s (Chile - CLT).\n📍 Geolocation: No coordinates\n📍 Location: No address\n\n",
		actionType, now.Format("15:04:05"))

	if actionType == "ENTRADA" {
		msg += "Have a great day!"
	} else {
		msg += "Rest and enjoy your free time!"
	}

	return msg, nil
}

func (s *Service) clickActionButton(ctx context.Context, actionType string) error {
	// We need to find the button that has the exact text.
	// Since chromedp doesn't have an exact text selector built-in, we use JS.
	js := fmt.Sprintf(`
		var els = document.querySelectorAll('button, div, span, li');
		var target = null;
		for (var i = 0; i < els.length; i++) {
			if (els[i].textContent.trim().toUpperCase() === '%s') {
				target = els[i];
				break;
			}
		}
		if (target) { target.click(); 'ok'; } else { 'not found'; }
	`, actionType)

	var res string
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &res)); err != nil {
		return err
	}
	if res != "ok" {
		return fmt.Errorf("button %s not found", actionType)
	}
	time.Sleep(2 * time.Second)
	return nil
}

func (s *Service) enterRUT(ctx context.Context, rutStr string) error {
	for _, ch := range rutStr {
		charUpper := strings.ToUpper(string(ch))

		js := fmt.Sprintf(`
			var els = document.querySelectorAll('li.digits');
			var target = null;
			for (var i = 0; i < els.length; i++) {
				if (els[i].textContent.trim().toUpperCase() === '%s') {
					target = els[i];
					break;
				}
			}
			if (target) { target.click(); 'ok'; } else { 'not found'; }
		`, charUpper)

		var res string
		if err := chromedp.Run(ctx, chromedp.Evaluate(js, &res)); err != nil {
			return fmt.Errorf("error clicking %s: %w", charUpper, err)
		}
		if res != "ok" {
			return fmt.Errorf("digit button %s not found", charUpper)
		}
		time.Sleep(300 * time.Millisecond)
	}
	time.Sleep(1 * time.Second)
	return nil
}

func (s *Service) submitForm(ctx context.Context) error {
	js := `
		var els = document.querySelectorAll('li.pad-action.digits');
		var target = null;
		for (var i = 0; i < els.length; i++) {
			if (els[i].textContent.trim().toUpperCase() === 'ENVIAR') {
				target = els[i];
				break;
			}
		}
		if (target) { target.click(); 'ok'; } else { 'not found'; }
	`
	var res string
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &res)); err != nil {
		return err
	}
	if res != "ok" {
		return fmt.Errorf("ENVIAR button not found")
	}
	time.Sleep(2 * time.Second)

	var readyState string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.readyState`, &readyState)); err != nil {
		return fmt.Errorf("browser did not remain reachable after submit: %w", err)
	}
	if readyState == "" {
		return fmt.Errorf("browser returned empty ready state after submit")
	}

	return nil
}
