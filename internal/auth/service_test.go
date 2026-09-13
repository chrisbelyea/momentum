package auth

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chrisbelyea/momentum/internal/db"
	_ "github.com/mattn/go-sqlite3"
)

func TestRequireRejectsCrossSiteMutation(t *testing.T) {
	db := testDB(t)
	s := NewService(db)
	r := httptest.NewRequest(http.MethodPost, "/protected", nil)
	r.Host = "localhost:8443"
	r.Header.Set("Origin", "https://evil.example")
	rr := httptest.NewRecorder()
	s.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("handler called") })).ServeHTTP(rr, r)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func testDB(t *testing.T) *sql.DB {
	d, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InitializeSchema(d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestPasswordAndSessionLifecycle(t *testing.T) {
	d := testDB(t)
	defer d.Close()
	s := NewService(d)
	userID, err := s.CreateUser("User@Example.com", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if userID != 1 {
		t.Fatalf("fresh registration should adopt the canonical compatibility user, got %d", userID)
	}
	if _, err := s.Authenticate("user@example.com", "wrong password"); err != ErrInvalidCredentials {
		t.Fatalf("wrong password error=%v", err)
	}
	if got, err := s.Authenticate("USER@example.com", "correct horse battery staple"); err != nil || got != userID {
		t.Fatalf("authenticate got %d/%v", got, err)
	}
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	if err := s.Login(w, userID); err != nil {
		t.Fatal(err)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unsafe cookie: %#v", cookie)
	}
	r.AddCookie(cookie)
	if got, err := s.UserID(r); err != nil || got != userID {
		t.Fatalf("session lookup got %d/%v", got, err)
	}
	s.Logout(w, r)
	if _, err := s.UserID(r); err != ErrUnauthenticated {
		t.Fatalf("revoked session error=%v", err)
	}
}

func TestFirstRegistrationPreservesExistingCompatibilityTasks(t *testing.T) {
	d := testDB(t)
	defer d.Close()
	if _, err := d.Exec(`INSERT INTO tasks(backend_id,uid,title,status,dtstamp,created_at)
		VALUES(1,'before-account','Existing task','NEEDS-ACTION',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	s := NewService(d)
	userID, err := s.CreateUser("owner@example.com", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if userID != 1 {
		t.Fatalf("existing compatibility user was not adopted: %d", userID)
	}
	var email string
	if err := d.QueryRow("SELECT email FROM users WHERE id=1").Scan(&email); err != nil || email != "owner@example.com" {
		t.Fatalf("adopted email=%q err=%v", email, err)
	}
	var tasks int
	if err := d.QueryRow("SELECT COUNT(*) FROM tasks WHERE backend_id=1 AND title='Existing task'").Scan(&tasks); err != nil || tasks != 1 {
		t.Fatalf("existing task was not preserved: count=%d err=%v", tasks, err)
	}
	var backends int
	if err := d.QueryRow("SELECT COUNT(*) FROM backends WHERE user_id=1").Scan(&backends); err != nil || backends != 1 {
		t.Fatalf("existing backend was not preserved: count=%d err=%v", backends, err)
	}
}

func TestCleanupExpiredSessions(t *testing.T) {
	d := testDB(t)
	defer d.Close()
	s := NewService(d)
	id, err := s.CreateUser("cleanup@example.com", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.Exec("INSERT INTO sessions(id,user_id,expires_at) VALUES('expired',?,?)", id, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupExpired(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := d.QueryRow("SELECT count(*) FROM sessions WHERE id='expired'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("expired session remains: %d/%v", count, err)
	}
}
