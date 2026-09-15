package sync

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrInvalidOperation is returned when an operation does not have the
// identity required for safe retries.
var ErrInvalidOperation = errors.New("sync operation requires an idempotency key and function")

// RetryableError lets an adapter distinguish a temporary transport failure
// from a permanent rejection (for example an invalid VTODO). Only errors
// marked retryable are attempted again.
type RetryableError struct{ Err error }

func (e *RetryableError) Error() string {
	if e == nil || e.Err == nil {
		return "retryable sync error"
	}
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error { return e.Err }

func Retryable(err error) error {
	if err == nil {
		return nil
	}
	return &RetryableError{Err: err}
}

func isRetryable(err error) bool {
	var retryable *RetryableError
	return errors.As(err, &retryable)
}

// IdempotencyStore records successful operation keys. Implementations should
// persist keys with the same durability as sync checkpoints so a process
// restart cannot repeat a non-idempotent provider mutation.
type IdempotencyStore interface {
	Completed(ctx context.Context, key string) (bool, error)
	MarkCompleted(ctx context.Context, key string) error
}

// RetryPolicy controls bounded exponential backoff for temporary failures.
type RetryPolicy struct {
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
	Multiplier  float64
}

func (p RetryPolicy) normalized() (RetryPolicy, error) {
	if p.MaxAttempts < 1 || p.InitialWait < 0 || p.MaxWait < 0 || p.Multiplier < 1 {
		return RetryPolicy{}, fmt.Errorf("invalid sync retry policy")
	}
	if p.MaxWait == 0 {
		p.MaxWait = p.InitialWait
	}
	if p.MaxWait < p.InitialWait {
		return RetryPolicy{}, fmt.Errorf("max wait cannot be less than initial wait")
	}
	return p, nil
}

// DefaultRetryPolicy is deliberately bounded: a sync run must yield control
// to its scheduler after a finite number of provider failures.
var DefaultRetryPolicy = RetryPolicy{MaxAttempts: 3, InitialWait: time.Second, MaxWait: time.Minute, Multiplier: 2}

// SyncOperation is one provider mutation or checkpoint-safe read. Key must
// be stable across retries and process restarts (backend/direction/entity).
type SyncOperation struct {
	// BackendID scopes concurrency and rate limiting. It is optional when the
	// runner has no limiter, preserving the provider-neutral operation contract.
	BackendID int
	Operation string
	Key       string
	Run       func(context.Context) error
	// Resume is called when Store reports a completed operation. Operations
	// whose successful result is needed by the caller (for example a push that
	// must return a provider ETag) can recover that result from the provider
	// rather than leaving the caller with an incomplete cycle result.
	Resume func(context.Context) error
}

// SyncEvent is emitted after each provider operation attempt. Consumers can
// use it for structured logs, metrics, or a user-visible sync status without
// coupling the engine to a logging or metrics package.
type SyncEvent struct {
	BackendID int
	Operation string
	Key       string
	Attempt   int
	Outcome   string
	Duration  time.Duration
	Error     string
}

// SyncObserver receives operation outcomes. Observers must be non-blocking;
// the engine invokes them synchronously after each attempt.
type SyncObserver func(SyncEvent)

// BackendLimiter bounds in-flight provider operations independently for each
// backend and spaces operation starts by the configured interval. Waiting is
// cancellation-aware, so a stopped sync run never holds a provider slot.
type BackendLimiter struct {
	maxConcurrent int
	interval      time.Duration
	mu            sync.Mutex
	backends      map[int]*backendGate
}

type backendGate struct {
	slots chan struct{}
	mu    sync.Mutex
	next  time.Time
}

// NewBackendLimiter creates a per-backend concurrency and rate limiter.
// maxConcurrent must be positive and interval cannot be negative.
func NewBackendLimiter(maxConcurrent int, interval time.Duration) (*BackendLimiter, error) {
	if maxConcurrent < 1 || interval < 0 {
		return nil, fmt.Errorf("invalid backend limiter configuration")
	}
	return &BackendLimiter{maxConcurrent: maxConcurrent, interval: interval, backends: make(map[int]*backendGate)}, nil
}

func (l *BackendLimiter) gate(backendID int) (*backendGate, error) {
	if backendID <= 0 {
		return nil, fmt.Errorf("backend ID must be positive")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if gate := l.backends[backendID]; gate != nil {
		return gate, nil
	}
	gate := &backendGate{slots: make(chan struct{}, l.maxConcurrent)}
	l.backends[backendID] = gate
	return gate, nil
}

func (l *BackendLimiter) acquire(ctx context.Context, backendID int) (func(), error) {
	gate, err := l.gate(backendID)
	if err != nil {
		return nil, err
	}
	select {
	case gate.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Reserve the next start while holding the gate lock. Reserving before
	// sleeping prevents concurrent callers from observing the same deadline.
	gate.mu.Lock()
	now := time.Now()
	wait := time.Duration(0)
	if now.Before(gate.next) {
		wait = gate.next.Sub(now)
	}
	start := now.Add(wait)
	gate.next = start.Add(l.interval)
	gate.mu.Unlock()
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			<-gate.slots
			return nil, ctx.Err()
		}
	}
	return func() { <-gate.slots }, nil
}

func (l *BackendLimiter) run(ctx context.Context, backendID int, fn func(context.Context) error) error {
	release, err := l.acquire(ctx, backendID)
	if err != nil {
		return err
	}
	defer release()
	return fn(ctx)
}

// Runner executes idempotent sync operations with bounded retries. It is
// provider-neutral; adapters decide which failures are temporary by wrapping
// them with Retryable.
type Runner struct {
	Policy   RetryPolicy
	Store    IdempotencyStore
	Sleep    func(context.Context, time.Duration) error
	Limiter  *BackendLimiter
	Observer SyncObserver
}

func (r Runner) Run(ctx context.Context, operation SyncOperation) error {
	if operation.Key == "" || operation.Run == nil {
		return ErrInvalidOperation
	}
	policy, err := r.Policy.normalized()
	if err != nil {
		return err
	}
	if r.Sleep == nil {
		r.Sleep = sleep
	}
	if r.Store != nil {
		completed, err := r.Store.Completed(ctx, operation.Key)
		if err != nil {
			return fmt.Errorf("check sync operation %q: %w", operation.Key, err)
		}
		if completed {
			if operation.Resume != nil {
				return operation.Resume(ctx)
			}
			return nil
		}
	}

	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		started := time.Now()
		run := operation.Run
		if r.Limiter != nil {
			run = func(ctx context.Context) error { return r.Limiter.run(ctx, operation.BackendID, operation.Run) }
		}
		lastErr = run(ctx)
		outcome := "failure"
		if lastErr == nil {
			outcome = "success"
		} else if isRetryable(lastErr) && attempt < policy.MaxAttempts {
			outcome = "retry"
		}
		if r.Observer != nil {
			r.Observer(SyncEvent{BackendID: operation.BackendID, Operation: operation.Operation, Key: operation.Key, Attempt: attempt, Outcome: outcome, Duration: time.Since(started), Error: errorString(lastErr)})
		}
		if lastErr == nil {
			if r.Store != nil {
				if err := r.Store.MarkCompleted(ctx, operation.Key); err != nil {
					return fmt.Errorf("mark sync operation %q complete: %w", operation.Key, err)
				}
			}
			return nil
		}
		if !isRetryable(lastErr) || attempt == policy.MaxAttempts {
			break
		}
		wait := backoff(policy, attempt)
		if err := r.Sleep(ctx, wait); err != nil {
			return err
		}
	}
	return fmt.Errorf("sync operation %q failed: %w", operation.Key, lastErr)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func backoff(policy RetryPolicy, attempt int) time.Duration {
	wait := float64(policy.InitialWait)
	for i := 1; i < attempt; i++ {
		wait *= policy.Multiplier
		if wait >= float64(policy.MaxWait) {
			return policy.MaxWait
		}
	}
	if wait > float64(policy.MaxWait) {
		return policy.MaxWait
	}
	return time.Duration(wait)
}

func sleep(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
