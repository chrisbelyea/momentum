package sync

import (
	"context"
	"errors"
	"reflect"
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
