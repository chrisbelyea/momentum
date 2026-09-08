package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chrisbelyea/momentum/internal/models"
)

type cycleAdapter struct {
	pull       PullResult
	pushes     []RemoteEntity
	deletes    []RemoteEntity
	pushErrors int
}

func (a *cycleAdapter) Capabilities() Capabilities {
	return Capabilities(CapabilityPull | CapabilityPush | CapabilityDelete)
}
func (a *cycleAdapter) Pull(context.Context, string) (PullResult, error) { return a.pull, nil }
func (a *cycleAdapter) Push(_ context.Context, entity RemoteEntity) (PushResult, error) {
	a.pushes = append(a.pushes, entity)
	if a.pushErrors > 0 {
		a.pushErrors--
		return PushResult{}, Retryable(errors.New("temporary provider error"))
	}
	return PushResult{RemoteUID: entity.RemoteUID, RemoteHref: "/tasks/" + entity.RemoteUID, ETag: "new-etag"}, nil
}
func (a *cycleAdapter) Delete(_ context.Context, entity RemoteEntity) error {
	a.deletes = append(a.deletes, entity)
	return nil
}

func TestRunCycleExecutesPushAndAppliesRemoteChanges(t *testing.T) {
	local := &models.Task{ID: 1, BackendID: 7, UID: "local", Title: "new", UpdatedAt: ptrTime(timeNow())}
	remote := RemoteEntity{RemoteUID: "remote", Task: models.Task{BackendID: 7, UID: "remote", Title: "provider"}, ETag: "r1"}
	adapter := &cycleAdapter{pull: PullResult{Entities: []RemoteEntity{remote}, NextCursor: "next"}}
	var applied []ActionKind
	policy := RetryPolicy{MaxAttempts: 2, InitialWait: time.Nanosecond, MaxWait: time.Nanosecond, Multiplier: 2}
	result, err := RunCycle(context.Background(), adapter, Runner{Policy: policy, Sleep: func(context.Context, time.Duration) error { return nil }}, CycleInput{BackendID: 7, Local: []*models.Task{local}, Cursor: "old"}, func(_ context.Context, action ReconcileAction, pushed *PushResult) error {
		applied = append(applied, action.Kind)
		if action.Kind == ActionPush && (pushed == nil || pushed.ETag != "new-etag") {
			t.Fatal("push result was not passed to apply callback")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.NextCursor != "next" || len(adapter.pushes) != 1 || len(applied) != 2 {
		t.Fatalf("unexpected cycle result: %#v pushes=%d applied=%v", result, len(adapter.pushes), applied)
	}
}

func TestRunCycleCorrelatesPushResultForUnsyncedLocalTask(t *testing.T) {
	local := &models.Task{ID: 1, BackendID: 9, UID: "local-new", Title: "new", UpdatedAt: ptrTime(timeNow())}
	adapter := &cycleAdapter{pull: PullResult{NextCursor: "next"}}
	policy := RetryPolicy{MaxAttempts: 1, InitialWait: time.Nanosecond, MaxWait: time.Nanosecond, Multiplier: 2}
	var pushed *PushResult
	_, err := RunCycle(context.Background(), adapter, Runner{Policy: policy}, CycleInput{
		BackendID: 9,
		Local:     []*models.Task{local},
	}, func(_ context.Context, action ReconcileAction, result *PushResult) error {
		if action.Kind != ActionPush {
			t.Fatalf("expected unsynced task to be pushed, got %s", action.Kind)
		}
		pushed = result
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if pushed == nil || pushed.RemoteUID != local.UID {
		t.Fatalf("push result was not correlated with local UID: %#v", pushed)
	}
	if len(adapter.pushes) != 1 || adapter.pushes[0].RemoteUID != local.UID {
		t.Fatalf("adapter received unexpected remote identity: %#v", adapter.pushes)
	}
}

func TestRunCycleRetriesPushAndNeverDeletesFromPartialPull(t *testing.T) {
	local := &models.Task{ID: 1, BackendID: 8, UID: "local", Title: "new", UpdatedAt: ptrTime(timeNow())}
	adapter := &cycleAdapter{pull: PullResult{HasMore: true, Entities: []RemoteEntity{{RemoteUID: "local", Task: models.Task{BackendID: 8, UID: "local", Title: "old"}, ETag: "same"}}}, pushErrors: 1}
	policy := RetryPolicy{MaxAttempts: 2, InitialWait: time.Nanosecond, MaxWait: time.Nanosecond, Multiplier: 2}
	pulled := timeNow().Add(-time.Hour)
	result, err := RunCycle(context.Background(), adapter, Runner{Policy: policy, Sleep: func(context.Context, time.Duration) error { return nil }}, CycleInput{BackendID: 8, Local: []*models.Task{local}, Mappings: []EntityMapping{{BackendID: 8, TaskID: ptrInt(1), RemoteUID: "local", RemoteETag: "same", LastPulledAt: &pulled}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(adapter.pushes) != 2 || result.RemoteDeletes != 0 {
		t.Fatalf("unexpected retry/deletion behavior: pushes=%d deletes=%d", len(adapter.pushes), result.RemoteDeletes)
	}
}

func timeNow() time.Time { return time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC) }
