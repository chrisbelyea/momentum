package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chrisbelyea/momentum/internal/auth"
	"github.com/chrisbelyea/momentum/internal/crypto"
	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/models"
	_ "github.com/mattn/go-sqlite3"
)

func TestRunBackendPushesLocalTaskAndPersistsCheckpoint(t *testing.T) {
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	if err := db.InitializeSchema(database); err != nil {
		t.Fatal(err)
	}
	if err := crypto.InitializeEncryption("runtime-sync-test-encryption-key"); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "REPORT":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:sync-token>token-1</d:sync-token></d:multistatus>`)
		case http.MethodPut:
			w.Header().Set("ETag", `"v1"`)
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	serverURL := strings.TrimRight(server.URL, "/") + "/calendar/"

	authService := auth.NewService(database)
	userID, err := authService.CreateUser("owner@example.test", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	backendRepo := db.NewBackendRepository(database)
	backend := &models.Backend{
		UserID: userID,
		Type:   models.BackendTypeExternalCalDAV,
		Name:   "Test CalDAV",
		Config: &models.BackendConfig{URL: serverURL, Username: "owner", Password: "secret", CalendarPath: serverURL, SkipTLSVerify: true},
	}
	if err := backendRepo.Create(backend); err != nil {
		t.Fatal(err)
	}
	taskRepo := db.NewTaskRepository(database)
	if err := taskRepo.Create(&models.Task{BackendID: backend.ID, Title: "local task", Status: models.StatusNeedsAction}); err != nil {
		t.Fatal(err)
	}
	syncRepo := db.NewSyncRepository(database)
	service := NewService(backendRepo, taskRepo, syncRepo)
	result, err := service.RunBackend(context.Background(), backend.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Collection != serverURL || result.Checkpoint.Status != "complete" || result.Checkpoint.Cursor != "token-1" {
		t.Fatalf("unexpected sync result: %#v", result)
	}
	entities, err := syncRepo.ListEntities(context.Background(), backend.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entities) != 1 || entities[0].RemoteUID == "" || entities[0].RemoteETag != `"v1"` {
		t.Fatalf("local push mapping was not persisted: %#v", entities)
	}
	var operations int
	if err := database.QueryRow("SELECT count(*) FROM sync_operations WHERE backend_id=?", backend.ID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations < 2 {
		t.Fatalf("runtime provider attempts were not persisted (got %d, want at least pull and push)", operations)
	}
}

func TestHandleStatusRequiresOwnership(t *testing.T) {
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := db.InitializeSchema(database); err != nil {
		t.Fatal(err)
	}
	service := NewService(db.NewBackendRepository(database), db.NewTaskRepository(database), db.NewSyncRepository(database))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/sync/status?backend_id=1", nil)
	service.HandleStatus(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status request = %d, want 401", recorder.Code)
	}
}
