package sync

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

type memoryIdempotencyStore struct {
	completed map[string]bool
	marks     []string
}

func (s *memoryIdempotencyStore) Completed(_ context.Context, key string) (bool, error) {
	return s.completed[key], nil
}
func (s *memoryIdempotencyStore) MarkCompleted(_ context.Context, key string) error {
	if s.completed == nil {
		s.completed = map[string]bool{}
	}
	s.completed[key] = true
	s.marks = append(s.marks, key)
	return nil
}

func TestRunnerRetriesTemporaryFailuresWithBoundedExponentialBackoff(t *testing.T) {
	var attempts int
	var waits []time.Duration
	runner := Runner{Policy: RetryPolicy{MaxAttempts: 4, InitialWait: 10 * time.Millisecond, MaxWait: 15 * time.Millisecond, Multiplier: 2}, Sleep: func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }}
	err := runner.Run(context.Background(), SyncOperation{Key: "backend/2/push/entity/7", Run: func(context.Context) error {
		attempts++
		if attempts < 4 {
			return Retryable(errors.New("temporary provider outage"))
		}
		return nil
	}})
	if err != nil || attempts != 4 {
		t.Fatalf("run: err=%v attempts=%d", err, attempts)
	}
	if want := []time.Duration{10 * time.Millisecond, 15 * time.Millisecond, 15 * time.Millisecond}; !reflect.DeepEqual(waits, want) {
		t.Fatalf("waits=%v want %v", waits, want)
	}
}

func TestRunnerDoesNotRetryPermanentFailure(t *testing.T) {
	attempts := 0
	err := Runner{Policy: RetryPolicy{MaxAttempts: 3, InitialWait: time.Hour, MaxWait: time.Hour, Multiplier: 2}, Sleep: func(context.Context, time.Duration) error { t.Fatal("permanent error was retried"); return nil }}.Run(context.Background(), SyncOperation{Key: "backend/2/push/entity/7", Run: func(context.Context) error { attempts++; return errors.New("invalid VTODO") }})
	if err == nil || attempts != 1 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
}

func TestRunnerSkipsCompletedIdempotencyKeyAndRecordsSuccess(t *testing.T) {
	store := &memoryIdempotencyStore{completed: map[string]bool{"already-done": true}}
	runs := 0
	runner := Runner{Policy: DefaultRetryPolicy, Store: store, Sleep: func(context.Context, time.Duration) error { return nil }}
	if err := runner.Run(context.Background(), SyncOperation{Key: "already-done", Run: func(context.Context) error { runs++; return nil }}); err != nil || runs != 0 {
		t.Fatalf("completed op: err=%v runs=%d", err, runs)
	}
	if err := runner.Run(context.Background(), SyncOperation{Key: "new-op", Run: func(context.Context) error { runs++; return nil }}); err != nil {
		t.Fatal(err)
	}
	if runs != 1 || !store.completed["new-op"] || !reflect.DeepEqual(store.marks, []string{"new-op"}) {
		t.Fatalf("store=%#v runs=%d", store, runs)
	}
}

func TestRunnerHonorsCancellationBeforeAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runs := 0
	err := (Runner{Policy: DefaultRetryPolicy}).Run(ctx, SyncOperation{Key: "cancelled", Run: func(context.Context) error { runs++; return nil }})
	if !errors.Is(err, context.Canceled) || runs != 0 {
		t.Fatalf("err=%v runs=%d", err, runs)
	}
}

func TestRunnerLimitsConcurrentOperationsPerBackend(t *testing.T) {
	limiter, err := NewBackendLimiter(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	runner := Runner{Policy: RetryPolicy{MaxAttempts: 1, Multiplier: 2}, Limiter: limiter}
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var mu sync.Mutex
	active, maxActive := 0, 0
	run := func(key string, backend int) <-chan error {
		done := make(chan error, 1)
		go func() {
			done <- runner.Run(context.Background(), SyncOperation{BackendID: backend, Operation: "push", Key: key, Run: func(context.Context) error {
				mu.Lock()
				active++
				if active > maxActive {
					maxActive = active
				}
				mu.Unlock()
				startedOnce.Do(func() { close(started) })
				<-release
				mu.Lock()
				active--
				mu.Unlock()
				return nil
			}})
		}()
		return done
	}
	first := run("backend/4/push/one", 4)
	<-started
	second := run("backend/4/push/two", 4)
	select {
	case <-second:
		t.Fatal("second operation entered while first was still running")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if maxActive != 1 {
		t.Fatalf("max concurrent operations=%d, want 1", maxActive)
	}
}

func TestRunnerAllowsDifferentBackendsToRunConcurrently(t *testing.T) {
	limiter, err := NewBackendLimiter(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	runner := Runner{Policy: RetryPolicy{MaxAttempts: 1, Multiplier: 2}, Limiter: limiter}
	started := make(chan int, 2)
	release := make(chan struct{})
	var wg sync.WaitGroup
	for _, backend := range []int{1, 2} {
		wg.Add(1)
		go func(backend int) {
			defer wg.Done()
			if err := runner.Run(context.Background(), SyncOperation{BackendID: backend, Operation: "pull", Key: "backend", Run: func(context.Context) error {
				started <- backend
				<-release
				return nil
			}}); err != nil {
				t.Errorf("backend %d: %v", backend, err)
			}
		}(backend)
	}
	seen := map[int]bool{<-started: true, <-started: true}
	close(release)
	wg.Wait()
	if len(seen) != 2 {
		t.Fatalf("operations did not run independently: %#v", seen)
	}
}

func TestRunnerRateLimitsOperationStartsAndEmitsAttempts(t *testing.T) {
	limiter, err := NewBackendLimiter(2, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	var events []SyncEvent
	runner := Runner{
		Policy:   RetryPolicy{MaxAttempts: 2, InitialWait: time.Nanosecond, MaxWait: time.Nanosecond, Multiplier: 2},
		Limiter:  limiter,
		Observer: func(event SyncEvent) { events = append(events, event) },
	}
	var mu sync.Mutex
	starts := make([]time.Time, 0, 2)
	attempts := 0
	err = runner.Run(context.Background(), SyncOperation{BackendID: 9, Operation: "pull", Key: "backend/9/pull", Run: func(context.Context) error {
		mu.Lock()
		starts = append(starts, time.Now())
		attempts++
		mu.Unlock()
		if attempts == 1 {
			return Retryable(errors.New("temporary"))
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(starts) != 2 || starts[1].Sub(starts[0]) < 18*time.Millisecond {
		t.Fatalf("operation starts=%v, expected at least 18ms apart", starts)
	}
	if len(events) != 2 || events[0].Outcome != "retry" || events[1].Outcome != "success" || events[0].Attempt != 1 || events[1].Attempt != 2 {
		t.Fatalf("unexpected observer events: %#v", events)
	}
	if events[0].Error == "" || events[1].Error != "" || events[0].BackendID != 9 || events[0].Operation != "pull" {
		t.Fatalf("observer event details missing: %#v", events)
	}
}

func TestRunnerLimiterCancellationReleasesQueuedOperation(t *testing.T) {
	limiter, err := NewBackendLimiter(1, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	runner := Runner{Policy: RetryPolicy{MaxAttempts: 1, Multiplier: 2}, Limiter: limiter}
	release := make(chan struct{})
	started := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- runner.Run(context.Background(), SyncOperation{BackendID: 3, Key: "first", Run: func(context.Context) error { close(started); <-release; return nil }})
	}()
	// Wait until the first operation has acquired its slot.
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := runner.Run(ctx, SyncOperation{BackendID: 3, Key: "queued", Run: func(context.Context) error { t.Fatal("cancelled operation ran"); return nil }}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued operation error=%v, want deadline exceeded", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}
