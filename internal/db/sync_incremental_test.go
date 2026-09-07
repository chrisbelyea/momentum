package db

import (
	"context"
	"testing"

	"github.com/chrisbelyea/momentum/internal/models"
	syncengine "github.com/chrisbelyea/momentum/internal/sync"
)

type checkpointPagesAdapter struct {
	cursors []string
}

func (a *checkpointPagesAdapter) Capabilities() syncengine.Capabilities {
	return syncengine.Capabilities(syncengine.CapabilityPull | syncengine.CapabilityIncrementalPull)
}

func (a *checkpointPagesAdapter) Pull(_ context.Context, cursor string) (syncengine.PullResult, error) {
	a.cursors = append(a.cursors, cursor)
	switch cursor {
	case "":
		return syncengine.PullResult{
			Entities: []syncengine.RemoteEntity{{
				RemoteUID:  "page-one",
				RemoteHref: "/dav/tasks/page-one.ics",
				ETag:       `"one"`,
				Task:       models.Task{UID: "page-one", Title: "Page one", Status: models.StatusNeedsAction},
			}},
			NextCursor:  "page-2",
			HasMore:     true,
			Incremental: true,
		}, nil
	case "page-2":
		return syncengine.PullResult{
			Entities: []syncengine.RemoteEntity{{
				RemoteUID:  "page-two",
				RemoteHref: "/dav/tasks/page-two.ics",
				ETag:       `"two"`,
				Task:       models.Task{UID: "page-two", Title: "Page two", Status: models.StatusNeedsAction},
			}},
			NextCursor:  "final-token",
			Incremental: true,
		}, nil
	default:
		return syncengine.PullResult{NextCursor: cursor, Incremental: true}, nil
	}
}

func (*checkpointPagesAdapter) Push(context.Context, syncengine.RemoteEntity) (syncengine.PushResult, error) {
	return syncengine.PushResult{}, nil
}

func (*checkpointPagesAdapter) Delete(context.Context, syncengine.RemoteEntity) error { return nil }

func TestIncrementalPullResumesFromDurableCheckpoint(t *testing.T) {
	database := syncApplyDB(t)
	defer database.Close()
	repo := NewSyncRepository(database)
	adapter := &checkpointPagesAdapter{}
	runner := syncengine.Runner{Policy: syncengine.RetryPolicy{MaxAttempts: 1, Multiplier: 2}}
	ctx := context.Background()

	first, err := syncengine.RunCycle(ctx, adapter, runner, syncengine.CycleInput{BackendID: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Pull.HasMore || first.NextCursor != "page-2" {
		t.Fatalf("first page did not expose resumable cursor: %#v", first)
	}
	if err := repo.ApplyCycle(ctx, SyncCheckpoint{BackendID: 2}, first); err != nil {
		t.Fatal(err)
	}

	checkpoint, err := repo.GetCheckpoint(2)
	if err != nil || checkpoint == nil || checkpoint.Cursor != "page-2" {
		t.Fatalf("checkpoint after first page: %#v err=%v", checkpoint, err)
	}
	tasks, err := NewTaskRepository(database).List(2)
	if err != nil || len(tasks) != 1 || tasks[0].UID != "page-one" {
		t.Fatalf("first page was not applied: tasks=%#v err=%v", tasks, err)
	}
	mappings, err := repo.ListEntities(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	local := make([]*models.Task, len(tasks))
	copy(local, tasks)
	engineMappings := make([]syncengine.EntityMapping, len(mappings))
	for i := range mappings {
		engineMappings[i] = mappings[i].Mapping()
	}

	second, err := syncengine.RunCycle(ctx, adapter, runner, syncengine.CycleInput{
		BackendID: 2,
		Cursor:    checkpoint.Cursor,
		Local:     local,
		Mappings:  engineMappings,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.NextCursor != "final-token" || second.Pull.HasMore {
		t.Fatalf("second page did not complete cursor: %#v", second)
	}
	if err := repo.ApplyCycle(ctx, *checkpoint, second); err != nil {
		t.Fatal(err)
	}
	checkpoint, err = repo.GetCheckpoint(2)
	if err != nil || checkpoint == nil || checkpoint.Cursor != "final-token" {
		t.Fatalf("final checkpoint: %#v err=%v", checkpoint, err)
	}
	tasks, err = NewTaskRepository(database).List(2)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("resumed page was not applied: tasks=%#v err=%v", tasks, err)
	}
	if len(adapter.cursors) != 2 || adapter.cursors[0] != "" || adapter.cursors[1] != "page-2" {
		t.Fatalf("pull did not resume from durable cursor: %#v", adapter.cursors)
	}
}
