package provider

import (
	"context"
	"fmt"
	"time"
)

// RetryPolicy is the retry-with-backoff policy applied to provider calls
// (generation spec 4.7: 失败自动重试（退避）与硬上限; 阶段 3 生成确认: 每方向最多
// 3 次总尝试). The backoff doubles per attempt and doubles as client-side rate
// limiting.
type RetryPolicy struct {
	MaxAttempts int           // total attempts, including the first
	Backoff     time.Duration // base delay between attempts
}

// DefaultRetryPolicy returns 3 total attempts with a 1s base backoff.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: DefaultMaxAttemptsPerDirection, Backoff: time.Second}
}

// PolicyFromConfig derives a policy from a provider config.
func PolicyFromConfig(c ProviderConfig) RetryPolicy {
	return RetryPolicy{MaxAttempts: c.EffectiveMaxAttempts(), Backoff: time.Second}
}

// RetryError reports that a call failed after exhausting all attempts.
type RetryError struct {
	Attempts int
	LastErr  error
}

func (e *RetryError) Error() string {
	return fmt.Sprintf("provider: call failed after %d attempts: %v", e.Attempts, e.LastErr)
}

func (e *RetryError) Unwrap() error { return e.LastErr }

// CallWithRetry runs fn up to policy.MaxAttempts times with exponential backoff
// between attempts. Errors marked with MarkNotRetryable (e.g. auth failures)
// stop immediately. The context is honored between attempts.
func CallWithRetry(ctx context.Context, policy RetryPolicy, fn func(ctx context.Context) error) error {
	if policy.MaxAttempts < 1 {
		policy.MaxAttempts = DefaultMaxAttemptsPerDirection
	}
	attempts := 0
	var lastErr error
	for {
		attempts++
		err := fn(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		if IsNotRetryable(err) || attempts >= policy.MaxAttempts {
			return &RetryError{Attempts: attempts, LastErr: lastErr}
		}
		delay := policy.Backoff
		for i := 1; i < attempts; i++ {
			delay *= 2
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}
