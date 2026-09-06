package caldav

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/models"
	syncengine "github.com/chrisbelyea/momentum/internal/sync"
	"github.com/chrisbelyea/momentum/pkg/vtodo"
	_ "github.com/mattn/go-sqlite3"
)

// lifecycleCalDAV is deliberately small, but exercises the real external
// client and adapter over HTTP. It models the collection/list/get/put/delete
// lifecycle needed by the Phase 1 two-way sync path.
type lifecycleCalDAV struct {
	mu      sync.Mutex
	todos   map[string]*vtodo.Todo
	etags   map[string]string
	version int
}

func (s *lifecycleCalDAV) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Method {
	case http.MethodOptions:
		w.Header().Set("Allow", "OPTIONS,PROPFIND,GET,PUT,DELETE")
		w.WriteHeader(http.StatusNoContent)
	case "PROPFIND":
		s.handlePropfind(w, r)
	case http.MethodGet:
		s.handleGet(w, r)
	case http.MethodPut:
		s.handlePut(w, r)
	case http.MethodDelete:
		s.handleDelete(w, r)
	default:
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
	}
}

func (s *lifecycleCalDAV) handlePropfind(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/dav/tasks/" {
		http.NotFound(w, r)
		return
	}
	uids := make([]string, 0, len(s.todos))
	for uid := range s.todos {
		uids = append(uids, uid)
	}
	sort.Strings(uids)
	var body strings.Builder
	body.WriteString(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">`)
	body.WriteString(`<d:response><d:href>/dav/tasks/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/><c:calendar/></d:resourcetype></d:prop></d:propstat></d:response>`)
	for _, uid := range uids {
		body.WriteString(fmt.Sprintf(`<d:response><d:href>/dav/tasks/%s.ics</d:href><d:propstat><d:prop><d:getetag>%s</d:getetag><d:getcontenttype>text/calendar; component=VTODO</d:getcontenttype></d:prop></d:propstat></d:response>`, uid, s.etags[uid]))
	}
	body.WriteString(`</d:multistatus>`)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = io.WriteString(w, body.String())
}

func (s *lifecycleCalDAV) handleGet(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimSuffix(path.Base(r.URL.Path), ".ics")
	todo, ok := s.todos[uid]
	if !ok {
		http.NotFound(w, r)
		return
	}
	body, err := todo.Marshal()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("ETag", s.etags[uid])
	w.Header().Set("Content-Type", "text/calendar")
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	_, _ = w.Write(body)
}

func (s *lifecycleCalDAV) handlePut(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimSuffix(path.Base(r.URL.Path), ".ics")
	old, exists := s.todos[uid]
	if condition := r.Header.Get("If-None-Match"); condition == "*" && exists {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	if condition := r.Header.Get("If-Match"); condition != "" && (!exists || condition != s.etags[uid]) {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	todo, err := vtodo.Parse(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if old != nil && todo.UID != old.UID {
		http.Error(w, "UID changed", http.StatusBadRequest)
		return
	}
	s.version++
	s.todos[uid], s.etags[uid] = todo, fmt.Sprintf(`"%d"`, s.version)
	w.Header().Set("ETag", s.etags[uid])
	w.Header().Set("Content-Length", "0")
	if exists {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
}

func (s *lifecycleCalDAV) handleDelete(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimSuffix(path.Base(r.URL.Path), ".ics")
	if _, ok := s.todos[uid]; !ok {
		http.NotFound(w, r)
		return
	}
	if condition := r.Header.Get("If-Match"); condition != "" && condition != s.etags[uid] {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	delete(s.todos, uid)
	delete(s.etags, uid)
	w.WriteHeader(http.StatusNoContent)
}

func TestSyncAdapterAndCycleCompleteTwoWayLifecycle(t *testing.T) {
	initial := &vtodo.Todo{UID: "remote-1", Summary: "Remote initial", Status: "NEEDS-ACTION", DTStamp: time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)}
	provider := &lifecycleCalDAV{todos: map[string]*vtodo.Todo{"remote-1": initial}, etags: map[string]string{"remote-1": `"1"`}, version: 1}
	server := httptest.NewTLSServer(provider)
	defer server.Close()
	client, err := NewClient(&models.BackendConfig{URL: server.URL + "/dav/", SkipTLSVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	adapter, err := NewSyncAdapter(client, server.URL+"/dav/tasks/")
	if err != nil {
		t.Fatal(err)
	}
	database, err := openLifecycleDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	taskRepo := db.NewTaskRepository(database)
	syncRepo := db.NewSyncRepository(database)
	ctx := context.Background()
	runner := syncengine.Runner{Policy: syncengine.RetryPolicy{MaxAttempts: 1, InitialWait: time.Nanosecond, MaxWait: time.Nanosecond, Multiplier: 2}}

	result, err := syncengine.RunCycle(ctx, adapter, runner, syncengine.CycleInput{BackendID: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncRepo.ApplyCycle(ctx, db.SyncCheckpoint{BackendID: 2}, result); err != nil {
		t.Fatal(err)
	}
	task, err := taskRepo.List(2)
	if err != nil || len(task) != 1 || task[0].Title != "Remote initial" {
		t.Fatalf("initial import failed: tasks=%#v err=%v", task, err)
	}

	task[0].Title = "Local update"
	if err := taskRepo.Update(task[0]); err != nil {
		t.Fatal(err)
	}
	mappings, err := syncRepo.ListEntities(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	result, err = syncengine.RunCycle(ctx, adapter, runner, syncengine.CycleInput{BackendID: 2, Local: task, Mappings: dbMapping(mappings).toEngine()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncRepo.ApplyCycle(ctx, db.SyncCheckpoint{BackendID: 2}, result); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	if provider.todos["remote-1"].Summary != "Local update" {
		provider.mu.Unlock()
		t.Fatal("local update was not pushed to provider")
	}
	provider.todos["remote-1"].Summary = "Remote update"
	provider.version++
	provider.etags["remote-1"] = fmt.Sprintf(`"%d"`, provider.version)
	provider.mu.Unlock()

	task, err = taskRepo.List(2)
	if err != nil {
		t.Fatal(err)
	}
	mappings, err = syncRepo.ListEntities(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	result, err = syncengine.RunCycle(ctx, adapter, runner, syncengine.CycleInput{BackendID: 2, Local: task, Mappings: dbMapping(mappings).toEngine()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncRepo.ApplyCycle(ctx, db.SyncCheckpoint{BackendID: 2}, result); err != nil {
		t.Fatal(err)
	}
	task, err = taskRepo.List(2)
	if err != nil || len(task) != 1 || task[0].Title != "Remote update" {
		t.Fatalf("remote update was not applied: tasks=%#v err=%v", task, err)
	}

	provider.mu.Lock()
	delete(provider.todos, "remote-1")
	delete(provider.etags, "remote-1")
	provider.mu.Unlock()
	task, err = taskRepo.List(2)
	if err != nil {
		t.Fatal(err)
	}
	mappings, err = syncRepo.ListEntities(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	result, err = syncengine.RunCycle(ctx, adapter, runner, syncengine.CycleInput{BackendID: 2, Local: task, Mappings: dbMapping(mappings).toEngine()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncRepo.ApplyCycle(ctx, db.SyncCheckpoint{BackendID: 2}, result); err != nil {
		t.Fatal(err)
	}
	if task, err = taskRepo.List(2); err != nil || len(task) != 0 {
		t.Fatalf("remote deletion was not propagated: tasks=%#v err=%v", task, err)
	}
}

// dbMapping keeps the lifecycle test's dependency on the durable repository
// explicit while converting rows to the provider-neutral planner contract.
type dbMapping []db.SyncEntity

func (m dbMapping) toEngine() []syncengine.EntityMapping {
	result := make([]syncengine.EntityMapping, len(m))
	for i := range m {
		result[i] = m[i].Mapping()
	}
	return result
}

func openLifecycleDB(t *testing.T) (*sql.DB, error) {
	t.Helper()
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, err
	}
	if err := db.InitializeSchema(database); err != nil {
		database.Close()
		return nil, err
	}
	if _, err := database.Exec("INSERT INTO users(id,email) VALUES(2,'lifecycle@example.com'); INSERT INTO backends(id,user_id,backend_type,name) VALUES(2,2,'external_caldav','Lifecycle')"); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}
