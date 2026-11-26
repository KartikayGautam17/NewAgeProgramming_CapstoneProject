package scraper

import (
	"context"
	"sync"
	"time"
)

// hostLimiter spaces out requests to the same host so that a large worker
// pool cannot overload one website. Each host gets its own schedule: the
// n-th request to a host may start no earlier than (n-1) intervals after the
// first. Requests to different hosts do not delay each other.
//
// It uses only the standard library: instead of a token bucket it keeps, for
// every host, the earliest time the next request may start.
type hostLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     map[string]time.Time // earliest start time of the next request per host
	now      func() time.Time     // replaceable clock, for tests
}

// newHostLimiter returns a limiter allowing about `rate` requests per second
// per host. A rate of zero or less means "unlimited" and returns nil; calling
// Wait on a nil limiter is allowed and returns immediately.
func newHostLimiter(rate float64) *hostLimiter {
	if rate <= 0 {
		return nil
	}
	return &hostLimiter{
		interval: time.Duration(float64(time.Second) / rate),
		next:     make(map[string]time.Time),
		now:      time.Now,
	}
}

// Wait blocks until a request to host may start, or until ctx is cancelled,
// in which case it returns ctx.Err().
func (l *hostLimiter) Wait(ctx context.Context, host string) error {
	if l == nil {
		return ctx.Err()
	}
	// Reserve a slot under the lock, then sleep outside it so that waiting
	// for one host never blocks reservations for another.
	l.mu.Lock()
	now := l.now()
	start := l.next[host]
	if start.Before(now) {
		start = now
	}
	l.next[host] = start.Add(l.interval)
	l.mu.Unlock()

	wait := start.Sub(now)
	if wait <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
