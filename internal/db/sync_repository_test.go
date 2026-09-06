package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/chrisbelyea/momentum/internal/models"
	_ "github.com/mattn/go-sqlite3"
)

func TestSyncRepositoryApplyBatchIsTransactionalAndResumable(t *testing.T) {
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err = InitializeSchema(database); err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec("INSERT INTO users(id,email) VALUES(2,'sync@example.com'); INSERT INTO backends(id,user_id,backend_type,name) VALUES(2,2,'external_caldav','Remote'); INSERT INTO tasks(id,backend_id,uid,title,status,dtstamp,created_at) VALUES(2,2,'local-2','Task','NEEDS-ACTION',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	repo := NewSyncRepository(database)
	now := time.Now().UTC()
	taskID, entityID := 2, 1
	err = repo.ApplyBatch(context.Background(), SyncCheckpoint{BackendID: 2, Cursor: "cursor-1", Status: "complete", LastCompletedAt: &now}, []SyncEntity{{BackendID: 2, TaskID: &taskID, RemoteUID: "remote-2", RemoteHref: "/2", RemoteETag: "etag"}}, []SyncOperation{{BackendID: 2, EntityID: &entityID, Direction: "push", Operation: "upsert", Outcome: "success", Attempts: 1, CompletedAt: &now}}, []SyncConflict{{BackendID: 2, EntityID: &entityID, TaskID: &taskID, LocalSnapshot: `{"title":"local"}`, RemoteSnapshot: `{"title":"remote"}`}})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := repo.GetCheckpoint(2)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint == nil || checkpoint.Cursor != "cursor-1" || checkpoint.Status != "complete" {
		t.Fatalf("unexpected checkpoint: %#v", checkpoint)
	}
	var entities, operations, conflicts int
	for query, dest := range map[string]*int{"SELECT count(*) FROM sync_entities": &entities, "SELECT count(*) FROM sync_operations": &operations, "SELECT count(*) FROM sync_conflicts": &conflicts} {
		if err := database.QueryRow(query).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	if entities != 1 || operations != 1 || conflicts != 1 {
		t.Fatalf("state counts: entities=%d operations=%d conflicts=%d", entities, operations, conflicts)
	}
	rows, err := repo.ListEntities(context.Background(), 2)
	if err != nil || len(rows) != 1 {
		t.Fatalf("list entities: err=%v rows=%d", err, len(rows))
	}
	mapping := rows[0].Mapping()
	if mapping.RemoteUID != "remote-2" || mapping.TaskID == nil || *mapping.TaskID != 2 || mapping.RemoteETag != "etag" {
		t.Fatalf("unexpected mapping: %#v", mapping)
	}
	// Invalid records must roll back all writes, including the checkpoint.
	err = repo.ApplyBatch(context.Background(), SyncCheckpoint{BackendID: 2, Cursor: "bad", Status: "failed"}, nil, []SyncOperation{{BackendID: 999, Direction: "pull", Operation: "pull", Outcome: "failed"}}, nil)
	if err == nil {
		t.Fatal("expected invalid batch to fail")
	}
	checkpoint, err = repo.GetCheckpoint(2)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Cursor != "cursor-1" {
		t.Fatalf("partial batch advanced cursor: %q", checkpoint.Cursor)
	}
}

func TestSyncRepositoryConflictRecoveryIsOwnedTransactionalAndRetainsEvidence(t *testing.T) {
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err = InitializeSchema(database); err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec("INSERT INTO users(id,email) VALUES(2,'conflict@example.com'); INSERT INTO backends(id,user_id,backend_type,name) VALUES(2,2,'external_caldav','Remote'); INSERT INTO tasks(id,backend_id,uid,title,status,dtstamp,created_at) VALUES(2,2,'task-2','Local title','NEEDS-ACTION',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	remote := models.Task{ID: 2, BackendID: 2, UID: "task-2", Title: "Remote title", Status: models.StatusInProcess, DTStamp: time.Now().UTC()}
	remoteSnapshot, err := json.Marshal(remote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO sync_entities(id,backend_id,task_id,remote_uid,remote_href,remote_etag,state,last_pulled_at,updated_at) VALUES(1,2,2,'task-2','/tasks/task-2.ics','"old"','active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO sync_conflicts(id,backend_id,entity_id,task_id,local_snapshot,remote_snapshot,policy,status) VALUES(1,2,1,2,'{"title":"Local title"}',?,'manual','open')`, string(remoteSnapshot)); err != nil {
		t.Fatal(err)
	}
	repo := NewSyncRepository(database)
	conflicts, err := repo.ListConflicts(context.Background(), 2, "open", 2)
	if err != nil || len(conflicts) != 1 || conflicts[0].ID != 1 || conflicts[0].RemoteSnapshot == "" {
		t.Fatalf("list conflicts: %#v err=%v", conflicts, err)
	}
	if got, err := repo.ListConflicts(context.Background(), 1, "", 0); err != nil || len(got) != 0 {
		t.Fatalf("cross-user conflict disclosure: %#v err=%v", got, err)
	}
	resolved, err := repo.ResolveConflict(context.Background(), 2, 1, "remote")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "resolved" || resolved.Resolution != "remote" || resolved.ResolvedAt == nil {
		t.Fatalf("unexpected resolved conflict: %#v", resolved)
	}
	var title, status string
	if err := database.QueryRow("SELECT title,status FROM tasks WHERE id=2").Scan(&title, &status); err != nil {
		t.Fatal(err)
	}
	if title != "Remote title" || status != models.StatusInProcess {
		t.Fatalf("remote resolution was not applied: title=%q status=%q", title, status)
	}
	var localSnapshot, remoteSnapshotAfter, resolution string
	if err := database.QueryRow("SELECT local_snapshot,remote_snapshot,resolution FROM sync_conflicts WHERE id=1").Scan(&localSnapshot, &remoteSnapshotAfter, &resolution); err != nil {
		t.Fatal(err)
	}
	if localSnapshot == "" || remoteSnapshotAfter == "" || resolution != "remote" {
		t.Fatalf("conflict evidence was not retained: local=%q remote=%q resolution=%q", localSnapshot, remoteSnapshotAfter, resolution)
	}
	if _, err := repo.ResolveConflict(context.Background(), 2, 1, "local"); err == nil {
		t.Fatal("expected already-resolved conflict to reject a second resolution")
	}
}
