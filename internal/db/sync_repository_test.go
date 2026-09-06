package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

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
