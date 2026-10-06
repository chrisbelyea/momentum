package sync

import (
	"testing"
	"time"

	"github.com/chrisbelyea/momentum/internal/models"
)

func TestPlanDetectsStableAndChangedSides(t *testing.T) {
	baseline := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	localEdited := baseline.Add(time.Hour)
	local := []*models.Task{
		{ID: 1, BackendID: 7, UID: "stable", Title: "same", UpdatedAt: ptrTime(baseline)},
		{ID: 2, BackendID: 7, UID: "remote", Title: "old", UpdatedAt: ptrTime(baseline)},
		{ID: 3, BackendID: 7, UID: "local", Title: "new", UpdatedAt: ptrTime(localEdited)},
		{ID: 4, BackendID: 7, UID: "both", Title: "local edit", UpdatedAt: ptrTime(localEdited)},
	}
	mappings := []EntityMapping{
		{BackendID: 7, TaskID: ptrInt(1), RemoteUID: "stable", RemoteETag: "s1", LastPulledAt: ptrTime(baseline)},
		{BackendID: 7, TaskID: ptrInt(2), RemoteUID: "remote", RemoteETag: "r1", LastPulledAt: ptrTime(baseline)},
		{BackendID: 7, TaskID: ptrInt(3), RemoteUID: "local", RemoteETag: "l1", LastPulledAt: ptrTime(baseline)},
		{BackendID: 7, TaskID: ptrInt(4), RemoteUID: "both", RemoteETag: "b1", LastPulledAt: ptrTime(baseline)},
	}
	result, err := Plan(ReconcileInput{BackendID: 7, Local: local, Mappings: mappings, Pull: PullResult{Entities: []RemoteEntity{
		{Task: models.Task{BackendID: 7, UID: "stable", Title: "same"}, RemoteUID: "stable", ETag: "s1"},
		{Task: models.Task{BackendID: 7, UID: "remote", Title: "provider edit"}, RemoteUID: "remote", ETag: "r2"},
		{Task: models.Task{BackendID: 7, UID: "local", Title: "old"}, RemoteUID: "local", ETag: "l1"},
		{Task: models.Task{BackendID: 7, UID: "both", Title: "provider edit"}, RemoteUID: "both", ETag: "b2"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	got := actionKindsByUID(result.Actions)
	want := map[string]ActionKind{"stable": ActionNoop, "remote": ActionUpdateLocal, "local": ActionPush, "both": ActionConflict}
	if len(got) != len(want) {
		t.Fatalf("actions=%#v", result.Actions)
	}
	for uid, kind := range want {
		if got[uid] != kind {
			t.Errorf("%s: got %q want %q", uid, got[uid], kind)
		}
	}
}

func TestPlanDoesNotDeleteFromPartialPull(t *testing.T) {
	pulled := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	task := &models.Task{ID: 1, BackendID: 3, UID: "uid-1", Title: "local", UpdatedAt: ptrTime(pulled)}
	result, err := Plan(ReconcileInput{BackendID: 3, Local: []*models.Task{task}, Mappings: []EntityMapping{{BackendID: 3, TaskID: ptrInt(1), RemoteUID: task.UID, RemoteETag: "old", LastPulledAt: ptrTime(pulled)}}, Pull: PullResult{HasMore: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Actions) != 0 {
		t.Fatalf("partial pull produced destructive action: %#v", result.Actions)
	}
}

func TestPlanHandlesRemoteDeletionAndNewEntities(t *testing.T) {
	baseline := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	local := []*models.Task{
		{ID: 1, BackendID: 4, UID: "deleted", Title: "gone", UpdatedAt: ptrTime(baseline)},
		{ID: 2, BackendID: 4, UID: "new-local", Title: "new", UpdatedAt: ptrTime(baseline)},
	}
	mappings := []EntityMapping{{BackendID: 4, TaskID: ptrInt(1), RemoteUID: "deleted", RemoteETag: "d1", LastPulledAt: ptrTime(baseline)}}
	result, err := Plan(ReconcileInput{BackendID: 4, Local: local, Mappings: mappings, Pull: PullResult{Entities: []RemoteEntity{{RemoteUID: "remote-new", Task: models.Task{UID: "remote-new", Title: "from provider"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	got := actionKindsByUID(result.Actions)
	if got["remote-new"] != ActionImport || got["deleted"] != ActionDeleteLocal || got["new-local"] != ActionPush {
		t.Fatalf("unexpected actions: %#v", result.Actions)
	}
}

func TestPlanResolvesIncrementalDeletionByDurableHref(t *testing.T) {
	baseline := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	task := &models.Task{ID: 1, BackendID: 5, UID: "deleted", Title: "gone", UpdatedAt: ptrTime(baseline)}
	result, err := Plan(ReconcileInput{
		BackendID: 5,
		Local:     []*models.Task{task},
		Mappings:  []EntityMapping{{BackendID: 5, TaskID: ptrInt(1), RemoteUID: "deleted", RemoteHref: "/dav/tasks/deleted.ics", RemoteETag: `"one"`, LastPulledAt: ptrTime(baseline)}},
		Pull:      PullResult{DeletedHrefs: []string{"/dav/tasks/deleted.ics"}, Incremental: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Actions) != 1 || result.Actions[0].Kind != ActionDeleteLocal || result.Actions[0].Remote == nil || result.Actions[0].Remote.RemoteUID != "deleted" {
		t.Fatalf("incremental deletion was not resolved through mapping: %#v", result.Actions)
	}
}

func TestPlanPushesUnmappedLocalTaskDuringIncrementalPull(t *testing.T) {
	task := &models.Task{ID: 1, BackendID: 6, UID: "local-only", Title: "new", UpdatedAt: ptrTime(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))}
	result, err := Plan(ReconcileInput{
		BackendID: 6,
		Local:     []*models.Task{task},
		Pull:      PullResult{NextCursor: "cursor-2", Incremental: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Actions) != 1 || result.Actions[0].Kind != ActionPush || result.Actions[0].Task != task {
		t.Fatalf("incremental pull did not push unmapped local task: %#v", result.Actions)
	}
}

func TestPlanRejectsDuplicateRemoteUID(t *testing.T) {
	_, err := Plan(ReconcileInput{BackendID: 1, Pull: PullResult{Entities: []RemoteEntity{{RemoteUID: "same"}, {RemoteUID: "same"}}}})
	if err == nil {
		t.Fatal("expected duplicate remote UID error")
	}
}

func actionKindsByUID(actions []ReconcileAction) map[string]ActionKind {
	result := make(map[string]ActionKind)
	for _, action := range actions {
		uid := ""
		if action.Remote != nil {
			uid = action.Remote.RemoteUID
		} else if action.Task != nil {
			uid = action.Task.UID
		}
		result[uid] = action.Kind
	}
	return result
}

func ptrInt(value int) *int              { return &value }
func ptrTime(value time.Time) *time.Time { return &value }
