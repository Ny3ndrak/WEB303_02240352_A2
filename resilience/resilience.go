// Package resilience implements a simple timeout+retry+circuit-breaker
// wrapper, used by Service A (claims) when it calls Service B (items).
package resilience

import (
	"context"
	"errors"
	"sync"
	"time"
)

type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ErrCircuitOpen is returned without even attempting a call when the
// breaker is open and still cooling down.
var ErrCircuitOpen = errors.New("circuit breaker open: downstream service is being given time to recover")

// CircuitBreaker trips after `threshold` consecutive failures, stays open
// for `cooldown`, then allows one trial call (half-open) before deciding.
type CircuitBreaker struct {
	mu        sync.Mutex
	state     State
	failures  int
	threshold int
	cooldown  time.Duration
	openedAt  time.Time
}

func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{threshold: threshold, cooldown: cooldown}
}

func (cb *CircuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case Open:
		if time.Since(cb.openedAt) >= cb.cooldown {
			cb.state = HalfOpen
			return true
		}
		return false
	default:
		return true
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.state = Closed
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	if cb.state == HalfOpen || cb.failures >= cb.threshold {
		cb.state = Open
		cb.openedAt = time.Now()
	}
}

func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Call runs fn with a per-attempt timeout, retrying up to `attempts` times
// with exponential backoff when isRetryable(err) is true, all guarded by
// the circuit breaker. isRetryable decides which errors are worth retrying
// (e.g. Unavailable, DeadlineExceeded) versus failing fast (e.g. NotFound,
// InvalidArgument).
func Call(
	ctx context.Context,
	cb *CircuitBreaker,
	attempts int,
	perAttemptTimeout time.Duration,
	baseDelay time.Duration,
	isRetryable func(error) bool,
	fn func(context.Context) error,
) error {
	if !cb.allow() {
		return ErrCircuitOpen
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		attemptCtx, cancel := context.WithTimeout(ctx, perAttemptTimeout)
		lastErr = fn(attemptCtx)
		cancel()

		if lastErr == nil {
			cb.recordSuccess()
			return nil
		}
		if !isRetryable(lastErr) || i == attempts-1 {
			break
		}
		time.Sleep(baseDelay * time.Duration(1<<i)) // 1x, 2x, 4x...
	}
	cb.recordFailure()
	return lastErr
}
