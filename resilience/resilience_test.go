package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errRetryable = errors.New("unavailable")
var errPermanent = errors.New("invalid argument")

func retryable(err error) bool { return errors.Is(err, errRetryable) }

func TestSucceedsFirstTry(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)
	calls := 0
	err := Call(context.Background(), cb, 3, time.Second, time.Millisecond, retryable, func(ctx context.Context) error {
		calls++
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	if cb.State() != Closed {
		t.Fatalf("want closed, got %v", cb.State())
	}
}

func TestRetriesThenSucceeds(t *testing.T) {
	cb := NewCircuitBreaker(5, time.Second)
	calls := 0
	err := Call(context.Background(), cb, 3, time.Second, time.Millisecond, retryable, func(ctx context.Context) error {
		calls++
		if calls < 3 {
			return errRetryable
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestPermanentErrorDoesNotRetry(t *testing.T) {
	cb := NewCircuitBreaker(5, time.Second)
	calls := 0
	err := Call(context.Background(), cb, 3, time.Second, time.Millisecond, retryable, func(ctx context.Context) error {
		calls++
		return errPermanent
	})
	if !errors.Is(err, errPermanent) || calls != 1 {
		t.Fatalf("err=%v calls=%d, want 1 call, no retry on permanent error", err, calls)
	}
}

func TestBreakerOpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)
	fail := func(ctx context.Context) error { return errRetryable }

	Call(context.Background(), cb, 1, time.Second, time.Millisecond, retryable, fail) // failure 1
	if cb.State() != Closed {
		t.Fatalf("want closed after 1 failure, got %v", cb.State())
	}
	Call(context.Background(), cb, 1, time.Second, time.Millisecond, retryable, fail) // failure 2 -> trips
	if cb.State() != Open {
		t.Fatalf("want open after threshold, got %v", cb.State())
	}

	calls := 0
	err := Call(context.Background(), cb, 3, time.Second, time.Millisecond, retryable, func(ctx context.Context) error {
		calls++
		return nil
	})
	if !errors.Is(err, ErrCircuitOpen) || calls != 0 {
		t.Fatalf("want fail-fast with no calls while open, err=%v calls=%d", err, calls)
	}
}

func TestBreakerHalfOpenThenCloses(t *testing.T) {
	cb := NewCircuitBreaker(1, 30*time.Millisecond)
	Call(context.Background(), cb, 1, time.Second, time.Millisecond, retryable, func(ctx context.Context) error { return errRetryable })
	if cb.State() != Open {
		t.Fatalf("want open, got %v", cb.State())
	}

	time.Sleep(40 * time.Millisecond) // let cooldown pass

	calls := 0
	err := Call(context.Background(), cb, 1, time.Second, time.Millisecond, retryable, func(ctx context.Context) error {
		calls++
		return nil // trial call succeeds
	})
	if err != nil || calls != 1 {
		t.Fatalf("want trial call to succeed, err=%v calls=%d", err, calls)
	}
	if cb.State() != Closed {
		t.Fatalf("want closed after successful trial, got %v", cb.State())
	}
}

func TestHalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker(1, 30*time.Millisecond)
	Call(context.Background(), cb, 1, time.Second, time.Millisecond, retryable, func(ctx context.Context) error { return errRetryable })
	time.Sleep(40 * time.Millisecond)

	Call(context.Background(), cb, 1, time.Second, time.Millisecond, retryable, func(ctx context.Context) error { return errRetryable }) // trial fails
	if cb.State() != Open {
		t.Fatalf("want re-opened after failed trial, got %v", cb.State())
	}
}

func TestTimeoutIsRetryable(t *testing.T) {
	cb := NewCircuitBreaker(5, time.Second)
	calls := 0
	err := Call(context.Background(), cb, 2, 20*time.Millisecond, time.Millisecond,
		func(err error) bool { return errors.Is(err, context.DeadlineExceeded) },
		func(ctx context.Context) error {
			calls++
			<-ctx.Done()
			return ctx.Err()
		})
	if !errors.Is(err, context.DeadlineExceeded) || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
