package caldav

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/models"
	syncengine "github.com/chrisbelyea/momentum/internal/sync"
	"github.com/chrisbelyea/momentum/pkg/vtodo"
	_ "github.com/mattn/go-sqlite3"
)

// TestLiveCalDAVInterop runs the outbound client against a real CalDAV server
// when MOMENTUM_CALDAV_INTEROP_URL is provided. The CI workflow supplies a
// pinned Radicale instance; local developers can point it at a provider or
// self-hosted server without putting credentials in the repository.
func TestLiveCalDAVInterop(t *testing.T) {
	baseURL := os.Getenv("MOMENTUM_CALDAV_INTEROP_URL")
	if baseURL == "" {
		t.Skip("MOMENTUM_CALDAV_INTEROP_URL is not set")
	}
	username := os.Getenv("MOMENTUM_CALDAV_INTEROP_USER")
	password := os.Getenv("MOMENTUM_CALDAV_INTEROP_PASSWORD")
	client, err := NewClient(&models.BackendConfig{
		URL:           baseURL,
		Username:      username,
		Password:      password,
		SkipTLSVerify: os.Getenv("MOMENTUM_CALDAV_INTEROP_SKIP_TLS_VERIFY") == "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	collections, err := client.DiscoverCollections(ctx)
	if err != nil {
		t.Fatalf("discover collections: %v", err)
	}
	var collection Collection
	for _, candidate := range collections {
		for _, component := range candidate.Components {
			if strings.EqualFold(component, "VTODO") {
				collection = candidate
				break
			}
		}
		if collection.Href != "" {
			break
		}
	}
	if collection.Href == "" {
		t.Fatalf("discovery returned no VTODO collection: %#v", collections)
	}

	todo := &vtodo.Todo{
		UID:     "momentum-live-interop@example.invalid",
		Summary: "Momentum live interoperability",
		Status:  "NEEDS-ACTION",
		DTStamp: time.Now().UTC().Truncate(time.Second),
	}
	created, err := client.CreateTodo(ctx, collection.Href, todo)
	if err != nil {
		t.Fatalf("create VTODO: %v", err)
	}
	if created.Href == "" || created.ETag == "" {
		t.Fatalf("create returned incomplete metadata: %#v", created)
	}
	currentETag := created.ETag

	defer func() {
		if err := client.DeleteTodo(context.Background(), created.Href, currentETag); err != nil {
			t.Errorf("cleanup VTODO: %v", err)
		}
	}()

	fetched, err := client.GetTodo(ctx, created.Href)
	if err != nil {
		t.Fatalf("read VTODO: %v", err)
	}
	if fetched.Todo.UID != todo.UID || fetched.Todo.Summary != todo.Summary {
		t.Fatalf("read VTODO = %#v, want UID %q and summary %q", fetched.Todo, todo.UID, todo.Summary)
	}
	if fetched.ETag == "" {
		t.Fatal("read VTODO did not return an ETag")
	}

	todo.Summary = "Momentum live interoperability updated"
	updated, err := client.UpdateTodo(ctx, created.Href, todo, fetched.ETag)
	if err != nil {
		t.Fatalf("update VTODO: %v", err)
	}
	if updated.ETag == "" {
		t.Fatal("update VTODO did not return an ETag")
	}
	currentETag = updated.ETag
	verified, err := client.GetTodo(ctx, created.Href)
	if err != nil {
		t.Fatalf("read updated VTODO: %v", err)
	}
	if verified.Todo.Summary != todo.Summary {
		t.Fatalf("updated summary = %q, want %q", verified.Todo.Summary, todo.Summary)
	}

	listed, err := client.ListTodos(ctx, collection.Href)
	if err != nil {
		t.Fatalf("list VTODO collection: %v", err)
	}
	var found bool
	for _, item := range listed {
		if item.Todo.UID == todo.UID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("list did not include created VTODO: %#v", listed)
	}

	t.Run("incremental resume after interrupted pull", func(t *testing.T) {
		testLiveIncrementalResume(t, client, collection.Href)
	})
}

// testLiveIncrementalResume exercises the complete provider-to-checkpoint
// path. The first cycle commits an opaque provider cursor, the next pull is
// intentionally discarded to model a process interruption, and the retry
// starts from the durable cursor before applying the provider change.
func testLiveIncrementalResume(t *testing.T, client *Client, collectionHref string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	todo := &vtodo.Todo{
		UID:     "momentum-live-resume@example.invalid",
		Summary: "Momentum interrupted sync before restart",
		Status:  "NEEDS-ACTION",
		DTStamp: time.Now().UTC().Truncate(time.Second),
	}
	created, err := client.CreateTodo(ctx, collectionHref, todo)
	if err != nil {
		t.Fatalf("create resume VTODO: %v", err)
	}
	currentETag := created.ETag
	defer func() {
		if err := client.DeleteTodo(context.Background(), created.Href, currentETag); err != nil {
			t.Errorf("cleanup resume VTODO: %v", err)
		}
	}()

	dbPath := filepath.Join(t.TempDir(), "resume.db")
	openDatabase := func() *sql.DB {
		database, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			t.Fatalf("open resume database: %v", err)
		}
		if err := db.InitializeSchema(database); err != nil {
			database.Close()
			t.Fatalf("initialize resume database: %v", err)
		}
		return database
	}
	database := openDatabase()
	defer database.Close()
	if _, err := database.Exec("INSERT INTO users(id,email) VALUES(2,'live-resume@example.invalid') ON CONFLICT(id) DO NOTHING; INSERT INTO backends(id,user_id,backend_type,name) VALUES(2,2,'external_caldav','Live resume') ON CONFLICT(id) DO NOTHING"); err != nil {
		t.Fatalf("create resume backend: %v", err)
	}

	adapter, err := NewSyncAdapter(client, collectionHref)
	if err != nil {
		t.Fatalf("create resume adapter: %v", err)
	}
	runner := syncengine.Runner{Policy: syncengine.RetryPolicy{MaxAttempts: 1, InitialWait: time.Nanosecond, MaxWait: time.Nanosecond, Multiplier: 2}}
	repo := db.NewSyncRepository(database)
	first, err := syncengine.RunCycle(ctx, adapter, runner, syncengine.CycleInput{BackendID: 2}, nil)
	if err != nil {
		t.Fatalf("initial live sync: %v", err)
	}
	if first.NextCursor == "" {
		t.Fatal("provider did not return an initial sync cursor")
	}
	if err := repo.ApplyCycle(ctx, db.SyncCheckpoint{BackendID: 2}, first); err != nil {
		t.Fatalf("commit initial live sync: %v", err)
	}
	checkpoint, err := repo.GetCheckpoint(2)
	if err != nil || checkpoint == nil || checkpoint.Cursor != first.NextCursor {
		t.Fatalf("initial checkpoint = %#v, err=%v", checkpoint, err)
	}

	todo.Summary = "Momentum interrupted sync after restart"
	updated, err := client.UpdateTodo(ctx, created.Href, todo, currentETag)
	if err != nil {
		t.Fatalf("update resume VTODO: %v", err)
	}
	currentETag = updated.ETag
	tasks, mappings, err := loadLiveSyncState(database, repo, 2)
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := syncengine.RunCycle(ctx, adapter, runner, syncengine.CycleInput{BackendID: 2, Cursor: checkpoint.Cursor, Local: tasks, Mappings: mappings}, nil)
	if err != nil {
		t.Fatalf("interrupted live pull: %v", err)
	}
	if !containsUpdatedTask(interrupted.Plan.Actions, todo.UID, todo.Summary) {
		t.Fatalf("interrupted pull did not observe provider update: %#v", interrupted.Plan.Actions)
	}

	// Drop the uncommitted result and reopen the SQLite file to model a process
	// restart. The retry must use the same durable cursor and apply the change.
	database.Close()
	database = openDatabase()
	repo = db.NewSyncRepository(database)
	checkpoint, err = repo.GetCheckpoint(2)
	if err != nil || checkpoint == nil {
		t.Fatalf("checkpoint after restart = %#v, err=%v", checkpoint, err)
	}
	tasks, mappings, err = loadLiveSyncState(database, repo, 2)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := syncengine.RunCycle(ctx, adapter, runner, syncengine.CycleInput{BackendID: 2, Cursor: checkpoint.Cursor, Local: tasks, Mappings: mappings}, nil)
	if err != nil {
		t.Fatalf("retry live pull: %v", err)
	}
	if !containsUpdatedTask(retried.Plan.Actions, todo.UID, todo.Summary) {
		t.Fatalf("retry did not replay provider update: %#v", retried.Plan.Actions)
	}
	if err := repo.ApplyCycle(ctx, *checkpoint, retried); err != nil {
		t.Fatalf("commit retried live sync: %v", err)
	}
	finalTasks, err := db.NewTaskRepository(database).List(2)
	if err != nil {
		t.Fatalf("load applied live task: %v", err)
	}
	for _, task := range finalTasks {
		if task.UID == todo.UID && task.Title == todo.Summary {
			return
		}
	}
	t.Fatalf("retried provider update was not applied to canonical task: %#v", finalTasks)
}

func loadLiveSyncState(database *sql.DB, repo *db.SyncRepository, backendID int) ([]*models.Task, []syncengine.EntityMapping, error) {
	tasks, err := db.NewTaskRepository(database).List(backendID)
	if err != nil {
		return nil, nil, fmt.Errorf("load live tasks: %w", err)
	}
	entities, err := repo.ListEntities(context.Background(), backendID)
	if err != nil {
		return nil, nil, fmt.Errorf("load live mappings: %w", err)
	}
	mappings := make([]syncengine.EntityMapping, len(entities))
	for i := range entities {
		mappings[i] = entities[i].Mapping()
	}
	return tasks, mappings, nil
}

func containsUpdatedTask(actions []syncengine.ReconcileAction, uid, title string) bool {
	for _, action := range actions {
		if action.Remote != nil && action.Remote.RemoteUID == uid && action.Remote.Task.Title == title && action.Kind == syncengine.ActionUpdateLocal {
			return true
		}
	}
	return false
}
