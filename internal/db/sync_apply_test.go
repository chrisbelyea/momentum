package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/chrisbelyea/momentum/internal/models"
	syncengine "github.com/chrisbelyea/momentum/internal/sync"
	_ "github.com/mattn/go-sqlite3"
)

func syncApplyDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := InitializeSchema(database); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO users(id,email) VALUES(2,'apply@example.com'); INSERT INTO backends(id,user_id,backend_type,name) VALUES(2,2,'external_caldav','Remote')"); err != nil {
		database.Close()
		t.Fatal(err)
	}
	return database
}

func TestApplyCycleImportsAndPersistsCheckpointAtomically(t *testing.T) {
	database := syncApplyDB(t)
	defer database.Close()
	repo := NewSyncRepository(database)
	cycle := syncengine.CycleResult{
		NextCursor: "cursor-1",
		Plan: syncengine.PlanResult{Actions: []syncengine.ReconcileAction{{
			Kind:   syncengine.ActionImport,
			Remote: &syncengine.RemoteEntity{RemoteUID: "remote-1", RemoteHref: "/tasks/1", ETag: `"one"`, Task: models.Task{UID: "remote-1", Title: "Imported", Status: models.StatusNeedsAction}},
		}}},
	}
	if err := repo.ApplyCycle(context.Background(), SyncCheckpoint{BackendID: 2}, cycle); err != nil {
		t.Fatal(err)
	}
	var taskID int
	if err := database.QueryRow("SELECT id FROM tasks WHERE uid='remote-1'").Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	var mapped int
	if err := database.QueryRow("SELECT count(*) FROM sync_entities WHERE task_id=? AND remote_etag=?", taskID, `"one"`).Scan(&mapped); err != nil || mapped != 1 {
		t.Fatalf("mapping missing: count=%d err=%v", mapped, err)
	}
	checkpoint, err := repo.GetCheckpoint(2)
	if err != nil || checkpoint == nil || checkpoint.Cursor != "cursor-1" || checkpoint.Status != "complete" {
		t.Fatalf("checkpoint not persisted: %#v err=%v", checkpoint, err)
	}
}

func TestApplyCycleRollsBackTaskAndCheckpointOnFailure(t *testing.T) {
	database := syncApplyDB(t)
	defer database.Close()
	repo := NewSyncRepository(database)
	cycle := syncengine.CycleResult{NextCursor: "must-not-advance", Plan: syncengine.PlanResult{Actions: []syncengine.ReconcileAction{
		{Kind: syncengine.ActionImport, Remote: &syncengine.RemoteEntity{RemoteUID: "remote-2", Task: models.Task{UID: "remote-2", Title: "Temporary", Status: models.StatusNeedsAction}}},
		{Kind: syncengine.ActionPush, Task: &models.Task{ID: 99, BackendID: 2, UID: "missing"}},
	}}}
	if err := repo.ApplyCycle(context.Background(), SyncCheckpoint{BackendID: 2, Cursor: "old", Status: "complete", LastCompletedAt: ptrTimeForApply(time.Now().UTC())}, cycle); err == nil {
		t.Fatal("expected incomplete push to fail")
	}
	var tasks int
	if err := database.QueryRow("SELECT count(*) FROM tasks WHERE uid='remote-2'").Scan(&tasks); err != nil || tasks != 0 {
		t.Fatalf("import survived rollback: tasks=%d err=%v", tasks, err)
	}
	checkpoint, err := repo.GetCheckpoint(2)
	if err != nil || checkpoint != nil {
		t.Fatalf("checkpoint unexpectedly committed: %#v err=%v", checkpoint, err)
	}
}

func ptrTimeForApply(value time.Time) *time.Time { return &value }
