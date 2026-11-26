package scraper

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNewHostLimiterZeroIsUnlimited(t *testing.T) {
	if newHostLimiter(0) != nil || newHostLimiter(-3) != nil {
		t.Fatal("rate <= 0 should give a nil (unlimited) limiter")
	}
	var l *hostLimiter // nil limiter must be safe to use
	if err := l.Wait(context.Background(), "a.com"); err != nil {
		t.Fatalf("nil limiter Wait = %v, want nil", err)
	}
}

func TestLimiterReservesEvenlySpacedSlots(t *testing.T) {
	l := newHostLimiter(100) // 10 ms interval
	fixed := time.Now()
	l.now = func() time.Time { return fixed } // frozen clock: slots are exact

	for i := 0; i < 5; i++ {
		if err := l.Wait(context.Background(), "a.com"); err != nil {
			t.Fatal(err)
		}
	}
	want := fixed.Add(5 * l.interval)
	if got := l.next["a.com"]; !got.Equal(want) {
		t.Errorf("next slot = %v after 5 requests, want %v", got.Sub(fixed), want.Sub(fixed))
	}
}

func TestLimiterHostsAreIndependent(t *testing.T) {
	l := newHostLimiter(1) // one request per second per host
	ctx := context.Background()
	start := time.Now()
	for _, h := range []string{"a.com", "b.com", "c.com"} {
		if err := l.Wait(ctx, h); err != nil { // first request to each host is immediate
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Errorf("first request to three different hosts took %v, want immediate", d)
	}
}

func TestLimiterWaitIsCancellable(t *testing.T) {
	l := newHostLimiter(1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_ = l.Wait(ctx, "a.com") // immediate; the next one would wait ~1 s
	start := time.Now()
	err := l.Wait(ctx, "a.com")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Error("Wait did not return promptly after cancellation")
	}
}

func TestLimiterConcurrentUseIsSafeAndSpaced(t *testing.T) {
	l := newHostLimiter(200) // 5 ms interval
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = l.Wait(context.Background(), "same.com")
		}()
	}
	wg.Wait()
	// 20 requests need 19 gaps of 5 ms = 95 ms; allow some timer slack.
	if d := time.Since(start); d < 85*time.Millisecond {
		t.Errorf("20 requests finished in %v, expected at least ~95ms", d)
	}
}
