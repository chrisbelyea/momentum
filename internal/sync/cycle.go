package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/chrisbelyea/momentum/internal/models"
)

// CycleInput is the durable local state used for one provider synchronization
// pass. The caller supplies the current checkpoint cursor and mappings; the
// cycle never guesses missing state.
type CycleInput struct {
	BackendID int
	Cursor    string
	Local     []*models.Task
	Mappings  []EntityMapping
}

// ApplyAction persists a local-side reconciliation decision. For imports and
// remote updates, result is nil. For a successful push, result contains the
// provider identity and conditional metadata that should be persisted with the
// task mapping. The callback runs only after the provider operation succeeds.
type ApplyAction func(context.Context, ReconcileAction, *PushResult) error

// CycleResult reports the provider pull and every planned action. Callers can
// persist the returned cursor and mappings in the same transaction as their
// ApplyAction implementation.
type CycleResult struct {
	Pull          PullResult
	Plan          PlanResult
	PushResults   map[string]PushResult
	NextCursor    string
	RemoteDeletes int
}

// RunCycle executes one bounded synchronization pass. Pull is performed once;
// provider mutations are routed through Runner so retryable failures are
// bounded and idempotency keys survive process restarts when a store is used.
// A partial pull can produce imports/updates but never causes destructive
// local or remote deletion actions because Plan enforces that invariant.
func RunCycle(ctx context.Context, adapter Adapter, runner Runner, input CycleInput, apply ApplyAction) (CycleResult, error) {
	if adapter == nil {
		return CycleResult{}, fmt.Errorf("sync adapter is required")
	}
	if input.BackendID <= 0 {
		return CycleResult{}, fmt.Errorf("backend ID must be positive")
	}
	if !adapter.Capabilities().Has(CapabilityPull) {
		return CycleResult{}, fmt.Errorf("sync adapter does not support pull")
	}
	if runner.Policy.MaxAttempts == 0 {
		runner.Policy = DefaultRetryPolicy
	}
	// Pulls participate in the same per-backend limiter and observability as
	// mutations, but are never marked completed in the idempotency store: a
	// successful read must run again when the scheduler retries that cursor.
	pullRunner := runner
	pullRunner.Store = nil
	var pull PullResult
	pullKey := fmt.Sprintf("backend/%d/pull/%s", input.BackendID, input.Cursor)
	if err := pullRunner.Run(ctx, SyncOperation{BackendID: input.BackendID, Operation: "pull", Key: pullKey, Run: func(ctx context.Context) error {
		var err error
		pull, err = adapter.Pull(ctx, input.Cursor)
		return err
	}}); err != nil {
		return CycleResult{}, fmt.Errorf("pull backend %d: %w", input.BackendID, err)
	}
	planned, err := Plan(ReconcileInput{BackendID: input.BackendID, Local: input.Local, Mappings: input.Mappings, Pull: pull})
	if err != nil {
		return CycleResult{}, fmt.Errorf("plan backend %d: %w", input.BackendID, err)
	}
	result := CycleResult{Pull: pull, Plan: planned, NextCursor: pull.NextCursor, PushResults: make(map[string]PushResult)}
	for index, action := range planned.Actions {
		if err := ctx.Err(); err != nil {
			return CycleResult{}, err
		}
		var pushed *PushResult
		switch action.Kind {
		case ActionPush:
			if !adapter.Capabilities().Has(CapabilityPush) {
				return CycleResult{}, fmt.Errorf("sync adapter does not support push for action %d", index)
			}
			entity := remoteEntityForAction(action, input.BackendID)
			var push PushResult
			pushKey := pushOperationKey(input.BackendID, entity, index)
			operation := SyncOperation{BackendID: input.BackendID, Operation: "push", Key: pushKey, Run: func(ctx context.Context) error {
				var err error
				push, err = adapter.Push(ctx, entity)
				return err
			}, Resume: func(ctx context.Context) error {
				resumer, ok := adapter.(interface {
					ResumePush(context.Context, RemoteEntity) (PushResult, error)
				})
				if !ok {
					return fmt.Errorf("sync adapter cannot resume completed push operation %q", pushKey)
				}
				var err error
				push, err = resumer.ResumePush(ctx, entity)
				return err
			}}
			if err := runner.Run(ctx, operation); err != nil {
				return CycleResult{}, err
			}
			result.PushResults[entity.RemoteUID] = push
			pushed = &push
		case ActionDeleteRemote:
			if !adapter.Capabilities().Has(CapabilityDelete) {
				return CycleResult{}, fmt.Errorf("sync adapter does not support remote delete for action %d", index)
			}
			entity := remoteEntityForAction(action, input.BackendID)
			if err := runner.Run(ctx, SyncOperation{BackendID: input.BackendID, Operation: "delete", Key: operationKey(input.BackendID, "delete", entity.RemoteUID, index), Run: func(ctx context.Context) error {
				return adapter.Delete(ctx, entity)
			}}); err != nil {
				return CycleResult{}, err
			}
			result.RemoteDeletes++
		}
		if apply != nil {
			if err := apply(ctx, action, pushed); err != nil {
				return CycleResult{}, fmt.Errorf("apply %s action %d: %w", action.Kind, index, err)
			}
		}
	}
	return result, nil
}

func remoteEntityForAction(action ReconcileAction, backendID int) RemoteEntity {
	entity := RemoteEntity{Task: models.Task{BackendID: backendID}}
	if action.Task != nil {
		entity.Task = *action.Task
		// A task without an existing mapping is being created remotely. Use
		// its canonical UID as the operation identity so the push result can
		// be correlated with the action when the cycle is applied. Existing
		// mappings below may replace this with the provider UID.
		entity.RemoteUID = action.Task.UID
	}
	if action.Remote != nil {
		entity.RemoteUID, entity.RemoteHref, entity.ETag, entity.RemoteSequence = action.Remote.RemoteUID, action.Remote.RemoteHref, action.Remote.ETag, action.Remote.RemoteSequence
	}
	if action.Mapping != nil {
		if entity.RemoteUID == "" {
			entity.RemoteUID = action.Mapping.RemoteUID
		}
		if entity.ETag == "" {
			entity.ETag = action.Mapping.RemoteETag
		}
	}
	return entity
}

func operationKey(backendID int, operation, uid string, index int) string {
	return fmt.Sprintf("backend/%d/%s/%s/%d", backendID, operation, uid, index)
}

// pushOperationKey includes the complete canonical task revision. A key that
// only contains UID/index would incorrectly suppress a later edit to the same
// task after the first push completed.
func pushOperationKey(backendID int, entity RemoteEntity, index int) string {
	revision, _ := json.Marshal(entity.Task)
	digest := sha256.Sum256(revision)
	return operationKey(backendID, "push", entity.RemoteUID, index) + "/" + hex.EncodeToString(digest[:])
}
