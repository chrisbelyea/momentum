package sync

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/chrisbelyea/momentum/internal/models"
)

// ActionKind is a provider-neutral decision produced by Plan. Applying an
// action is deliberately separate from planning: callers can persist the
// batch and its checkpoint atomically, and can retry provider mutations with
// Runner.
type ActionKind string

const (
	ActionNoop         ActionKind = "noop"
	ActionImport       ActionKind = "import"
	ActionUpdateLocal  ActionKind = "update-local"
	ActionPush         ActionKind = "push"
	ActionDeleteLocal  ActionKind = "delete-local"
	ActionDeleteRemote ActionKind = "delete-remote"
	ActionConflict     ActionKind = "conflict"
)

// ReconcileAction describes one deterministic decision for a task/entity.
// Remote and Mapping are nil for a local task that has never been synced.
type ReconcileAction struct {
	Kind    ActionKind
	Task    *models.Task
	Remote  *RemoteEntity
	Mapping *EntityMapping
	Reason  string
}

// EntityMapping is the provider-neutral subset of a durable sync entity that
// the planner needs. The database package can translate its SyncEntity rows
// into these values without making this package depend on SQLite.
type EntityMapping struct {
	BackendID      int
	TaskID         *int
	RemoteUID      string
	RemoteETag     string
	RemoteSequence *int
	LastPulledAt   *time.Time
}

// ReconcileInput is the complete state needed to plan one pull page. A
// caller must only set Complete when Pull contains the provider's final page;
// an incomplete page must never cause local deletions.
type ReconcileInput struct {
	BackendID int
	Local     []*models.Task
	Mappings  []EntityMapping
	Pull      PullResult
}

// PlanResult contains actions and a stable summary suitable for an operation
// record. Actions are ordered by remote UID, then local task ID.
type PlanResult struct {
	Actions []ReconcileAction
}

// Plan compares canonical local tasks with one provider pull result. It
// detects the four important states (new, changed remotely, changed locally,
// and changed on both sides) and handles provider deletions only after a
// complete pull. It does not perform I/O or mutate either input.
func Plan(input ReconcileInput) (PlanResult, error) {
	if input.BackendID <= 0 {
		return PlanResult{}, fmt.Errorf("backend ID must be positive")
	}
	localByID := make(map[int]*models.Task, len(input.Local))
	localByUID := make(map[string]*models.Task, len(input.Local))
	for _, task := range input.Local {
		if task == nil || task.ID <= 0 || task.BackendID != input.BackendID || task.UID == "" {
			return PlanResult{}, fmt.Errorf("invalid local task")
		}
		if _, ok := localByID[task.ID]; ok {
			return PlanResult{}, fmt.Errorf("duplicate local task ID %d", task.ID)
		}
		if prior, ok := localByUID[task.UID]; ok {
			return PlanResult{}, fmt.Errorf("duplicate local task UID %q (IDs %d and %d)", task.UID, prior.ID, task.ID)
		}
		localByID[task.ID] = task
		localByUID[task.UID] = task
	}

	mappingByUID := make(map[string]*EntityMapping, len(input.Mappings))
	mappedTaskIDs := make(map[int]bool, len(input.Mappings))
	for i := range input.Mappings {
		mapping := &input.Mappings[i]
		if mapping.BackendID != input.BackendID || mapping.RemoteUID == "" {
			return PlanResult{}, fmt.Errorf("invalid sync mapping")
		}
		if _, ok := mappingByUID[mapping.RemoteUID]; ok {
			return PlanResult{}, fmt.Errorf("duplicate remote UID %q", mapping.RemoteUID)
		}
		if mapping.TaskID != nil {
			if _, ok := localByID[*mapping.TaskID]; !ok {
				// A missing task is recoverable (for example, after a local
				// delete). Keep the mapping so the plan can import the remote.
				mappedTaskIDs[*mapping.TaskID] = true
			} else if mappedTaskIDs[*mapping.TaskID] {
				return PlanResult{}, fmt.Errorf("task ID %d has multiple sync mappings", *mapping.TaskID)
			} else {
				mappedTaskIDs[*mapping.TaskID] = true
			}
		}
		mappingByUID[mapping.RemoteUID] = mapping
	}

	remoteByUID := make(map[string]*RemoteEntity, len(input.Pull.Entities))
	for i := range input.Pull.Entities {
		remote := &input.Pull.Entities[i]
		if remote.RemoteUID == "" {
			return PlanResult{}, fmt.Errorf("remote entity has empty UID")
		}
		if remote.Task.BackendID != 0 && remote.Task.BackendID != input.BackendID {
			return PlanResult{}, fmt.Errorf("remote entity %q belongs to backend %d", remote.RemoteUID, remote.Task.BackendID)
		}
		if _, ok := remoteByUID[remote.RemoteUID]; ok {
			return PlanResult{}, fmt.Errorf("duplicate remote UID %q", remote.RemoteUID)
		}
		remoteByUID[remote.RemoteUID] = remote
	}

	actions := make([]ReconcileAction, 0, len(remoteByUID)+len(localByID))
	remoteUIDs := make([]string, 0, len(remoteByUID))
	for uid := range remoteByUID {
		remoteUIDs = append(remoteUIDs, uid)
	}
	sort.Strings(remoteUIDs)
	for _, uid := range remoteUIDs {
		remote := remoteByUID[uid]
		mapping := mappingByUID[uid]
		var task *models.Task
		if mapping != nil && mapping.TaskID != nil {
			task = localByID[*mapping.TaskID]
		}
		// UID is the canonical identity. This lets a first pull link a
		// locally-created task even if its mapping row was not committed.
		if task == nil {
			task = localByUID[uid]
		}
		if mapping == nil || task == nil {
			if remote.Deleted {
				continue // nothing local to delete
			}
			actions = append(actions, ReconcileAction{Kind: ActionImport, Task: task, Remote: remote, Mapping: mapping, Reason: "remote entity has no local mapping"})
			continue
		}
		if remote.Deleted {
			if localChanged(task, mapping) {
				actions = append(actions, ReconcileAction{Kind: ActionConflict, Task: task, Remote: remote, Mapping: mapping, Reason: "remote deleted after local edit"})
			} else {
				actions = append(actions, ReconcileAction{Kind: ActionDeleteLocal, Task: task, Remote: remote, Mapping: mapping, Reason: "remote entity was deleted"})
			}
			continue
		}
		remoteChanged := changedRemotely(task, remote, mapping)
		localEdited := localChanged(task, mapping)
		switch {
		case remoteChanged && localEdited:
			actions = append(actions, ReconcileAction{Kind: ActionConflict, Task: task, Remote: remote, Mapping: mapping, Reason: "both local and remote changed"})
		case remoteChanged:
			actions = append(actions, ReconcileAction{Kind: ActionUpdateLocal, Task: task, Remote: remote, Mapping: mapping, Reason: "remote entity changed"})
		case localEdited:
			actions = append(actions, ReconcileAction{Kind: ActionPush, Task: task, Remote: remote, Mapping: mapping, Reason: "local task changed"})
		default:
			actions = append(actions, ReconcileAction{Kind: ActionNoop, Task: task, Remote: remote, Mapping: mapping, Reason: "local and remote are unchanged"})
		}
	}

	if !input.Pull.HasMore {
		ids := make([]int, 0, len(localByID))
		for id := range localByID {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		for _, id := range ids {
			task := localByID[id]
			var mapping *EntityMapping
			for i := range input.Mappings {
				if input.Mappings[i].TaskID != nil && *input.Mappings[i].TaskID == id {
					mapping = &input.Mappings[i]
					break
				}
			}
			if mapping == nil {
				actions = append(actions, ReconcileAction{Kind: ActionPush, Task: task, Reason: "local task has never been synced"})
				continue
			}
			if _, present := remoteByUID[mapping.RemoteUID]; present {
				continue
			}
			if localChanged(task, mapping) {
				actions = append(actions, ReconcileAction{Kind: ActionConflict, Task: task, Mapping: mapping, Reason: "remote entity disappeared after local edit"})
			} else {
				actions = append(actions, ReconcileAction{Kind: ActionDeleteLocal, Task: task, Mapping: mapping, Reason: "remote entity missing from complete pull"})
			}
		}
	}
	return PlanResult{Actions: actions}, nil
}

func localChanged(task *models.Task, mapping *EntityMapping) bool {
	return mapping != nil && mapping.LastPulledAt != nil && task.UpdatedAt != nil && task.UpdatedAt.After(*mapping.LastPulledAt)
}

func changedRemotely(local *models.Task, remote *RemoteEntity, mapping *EntityMapping) bool {
	if mapping == nil {
		return true
	}
	if remote.ETag != "" && mapping.RemoteETag != "" {
		return remote.ETag != mapping.RemoteETag
	}
	if remote.RemoteSequence != nil && mapping.RemoteSequence != nil {
		return *remote.RemoteSequence != *mapping.RemoteSequence
	}
	return !taskEquivalent(local, &remote.Task)
}

// taskEquivalent compares fields represented by a VTODO, excluding local
// database identity and bookkeeping timestamps. It is the safe fallback for
// providers that do not supply ETags or sequence numbers.
func taskEquivalent(left, right *models.Task) bool {
	if left == nil || right == nil {
		return left == right
	}
	leftData, _ := json.Marshal(struct {
		UID, Title, Status, Description, Tags, Related, URL, Location, Extra string
		Priority, Sequence, Percent                                          *int
		Due, Start, Completed                                                *time.Time
	}{left.UID, left.Title, left.Status, value(left.Description), value(left.TagsJSON), value(left.RelatedToJSON), value(left.URL), value(left.Location), value(left.ExtraJSON), left.Priority, &left.Sequence, left.PercentComplete, left.DueAt, left.StartAt, left.CompletedAt})
	rightData, _ := json.Marshal(struct {
		UID, Title, Status, Description, Tags, Related, URL, Location, Extra string
		Priority, Sequence, Percent                                          *int
		Due, Start, Completed                                                *time.Time
	}{right.UID, right.Title, right.Status, value(right.Description), value(right.TagsJSON), value(right.RelatedToJSON), value(right.URL), value(right.Location), value(right.ExtraJSON), right.Priority, &right.Sequence, right.PercentComplete, right.DueAt, right.StartAt, right.CompletedAt})
	return reflect.DeepEqual(leftData, rightData)
}

func value(pointer *string) string {
	if pointer == nil {
		return ""
	}
	return *pointer
}
