package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chrisbelyea/momentum/internal/auth"
	"github.com/chrisbelyea/momentum/internal/db"
)

func TestNativeCapabilities(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	h.HandleNativeAPI(rec, httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("capabilities returned %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != nativeAPIMediaType {
		t.Fatalf("content type=%q", got)
	}
	var payload struct {
		API      struct{ Minimum, Maximum int } `json:"api"`
		Features map[string]bool                `json:"features"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.API.Minimum != 1 || payload.API.Maximum != 1 || !payload.Features["conflict_recovery"] {
		t.Fatalf("unexpected capabilities: %#v", payload)
	}
}

func TestNativeTaskListRequiresAuthenticatedOwner(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()
	authService := auth.NewService(database)
	userID, err := authService.CreateUser("native@example.com", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(db.NewTaskRepository(database), db.NewBackendRepository(database))
	protected := authService.Require(http.HandlerFunc(h.HandleNativeAPI))

	login := httptest.NewRecorder()
	if err := authService.Login(login, userID); err != nil {
		t.Fatal(err)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected session cookie, got %d", len(cookies))
	}

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{"title":"Native task"}`))
	createReq.AddCookie(cookies[0])
	protected.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create returned %d: %s", create.Code, create.Body.String())
	}

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	listReq.AddCookie(cookies[0])
	protected.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Native task") {
		t.Fatalf("list returned %d: %s", list.Code, list.Body.String())
	}

	unauthenticated := httptest.NewRecorder()
	protected.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list returned %d", unauthenticated.Code)
	}
	var task struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &task); err != nil || task.ID == 0 {
		t.Fatalf("created task response: %s (%v)", create.Body.String(), err)
	}
	path := fmt.Sprintf("/api/v1/tasks/%d/status", task.ID)
	withoutVersion := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(`{"status":"COMPLETED"}`))
	req.AddCookie(cookies[0])
	protected.ServeHTTP(withoutVersion, req)
	if withoutVersion.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing version returned %d: %s", withoutVersion.Code, withoutVersion.Body.String())
	}
	stale := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, path, strings.NewReader(`{"status":"COMPLETED"}`))
	req.AddCookie(cookies[0])
	req.Header.Set("If-Match", `"stale"`)
	protected.ServeHTTP(stale, req)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), `"authoritative_task"`) {
		t.Fatalf("stale write returned %d: %s", stale.Code, stale.Body.String())
	}
	valid := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, path, strings.NewReader(`{"status":"COMPLETED"}`))
	req.AddCookie(cookies[0])
	req.Header.Set("If-Match", create.Header().Get("ETag"))
	protected.ServeHTTP(valid, req)
	if valid.Code != http.StatusOK || !strings.Contains(valid.Body.String(), `"COMPLETED"`) {
		t.Fatalf("valid status change returned %d: %s", valid.Code, valid.Body.String())
	}
}
