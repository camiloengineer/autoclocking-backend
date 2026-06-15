//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestMarcaje_DOM_Structure(t *testing.T) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(ctx, 15*time.Second)
	defer cancelTimeout()

	err := chromedp.Run(ctx, 
		chromedp.Navigate("https://app.ctrlit.cl/ctrl/dial/web/K1NBpBqyjf"),
		chromedp.WaitVisible(`li.digits`, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	// Check if any li.digits elements exist
	var res string
	err = chromedp.Run(ctx, chromedp.Evaluate(`
		(function() {
			var els = document.querySelectorAll('li.digits');
			if (els.length > 0) return 'found';
			return 'not found';
		})();
	`, &res))

	if err != nil {
		t.Fatalf("Failed to evaluate JS: %v", err)
	}

	if res != "found" {
		t.Errorf("Expected to find li.digits elements, got %s", res)
	}
}
