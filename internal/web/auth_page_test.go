package web

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/chrisbelyea/momentum/internal/auth"
	"github.com/chrisbelyea/momentum/internal/db"
	_ "github.com/mattn/go-sqlite3"
)

func newAuthPageTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	if err := db.InitializeSchema(database); err != nil {
		database.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestAuthPagesFirstRunWorkflowAndRouteProtection(t *testing.T) {
	database := newAuthPageTestDB(t)
	service := auth.NewService(database)
	pages := NewAuthPageHandler(service)
	protected := service.RequirePage(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "protected")
	}))

	// A fresh browser navigation receives an onboarding redirect, not a raw API
	// 401, and the original relative URL is retained.
	request := httptest.NewRequest(http.MethodGet, "https://momentum.test/?return=/list", nil)
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login?return=%2F%3Freturn%3D%2Flist" {
		t.Fatalf("unauthenticated navigation: status=%d location=%q", response.Code, response.Header().Get("Location"))
	}

	login := httptest.NewRecorder()
	pages.HandleLogin(login, httptest.NewRequest(http.MethodGet, "https://momentum.test/login", nil))
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), "Create your account") {
		t.Fatalf("fresh login page did not offer onboarding: status=%d body=%s", login.Code, login.Body.String())
	}

	registerForm := url.Values{
		"email":    {"owner@example.test"},
		"password": {"correct horse battery staple"},
		"return":   {"/list"},
	}
	registerRequest := httptest.NewRequest(http.MethodPost, "https://momentum.test/register", strings.NewReader(registerForm.Encode()))
	registerRequest.Host = "momentum.test"
	registerRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	registerRequest.Header.Set("Origin", "https://momentum.test")
	register := httptest.NewRecorder()
	pages.HandleRegister(register, registerRequest)
	if register.Code != http.StatusSeeOther || register.Header().Get("Location") != "/list" {
		t.Fatalf("registration redirect: status=%d location=%q body=%s", register.Code, register.Header().Get("Location"), register.Body.String())
	}
	cookies := register.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("registration session cookie is unsafe: %#v", cookies)
	}

	// Registration is first-account-only; the JSON API and HTML page enforce the
	// same policy rather than leaving an open self-registration endpoint.
	second := httptest.NewRecorder()
	secondRequest := httptest.NewRequest(http.MethodPost, "https://momentum.test/auth/register", strings.NewReader(`{"email":"second@example.test","password":"correct horse battery staple"}`))
	secondRequest.Host = "momentum.test"
	secondRequest.Header.Set("Content-Type", "application/json")
	secondRequest.Header.Set("Origin", "https://momentum.test")
	service.Routes().ServeHTTP(second, secondRequest)
	if second.Code != http.StatusForbidden {
		t.Fatalf("second registration status=%d body=%s", second.Code, second.Body.String())
	}

	// Failed sign-in does not reveal whether an account exists.
	badLoginForm := url.Values{"email": {"owner@example.test"}, "password": {"wrong password"}}
	badLoginRequest := httptest.NewRequest(http.MethodPost, "https://momentum.test/login", strings.NewReader(badLoginForm.Encode()))
	badLoginRequest.Host = "momentum.test"
	badLoginRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badLoginRequest.Header.Set("Origin", "https://momentum.test")
	badLogin := httptest.NewRecorder()
	pages.HandleLogin(badLogin, badLoginRequest)
	if badLogin.Code != http.StatusUnauthorized || !strings.Contains(badLogin.Body.String(), "Invalid email or password") {
		t.Fatalf("bad sign-in response: status=%d body=%s", badLogin.Code, badLogin.Body.String())
	}

	// The session created by registration reaches protected pages.
	authenticatedRequest := httptest.NewRequest(http.MethodGet, "https://momentum.test/list", nil)
	authenticatedRequest.AddCookie(cookies[0])
	authenticated := httptest.NewRecorder()
	protected.ServeHTTP(authenticated, authenticatedRequest)
	if authenticated.Code != http.StatusOK || authenticated.Body.String() != "protected" {
		t.Fatalf("authenticated navigation: status=%d body=%q", authenticated.Code, authenticated.Body.String())
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "https://momentum.test/logout", nil)
	logoutRequest.Host = "momentum.test"
	logoutRequest.Header.Set("Origin", "https://momentum.test")
	logoutRequest.AddCookie(cookies[0])
	logout := httptest.NewRecorder()
	pages.HandleLogout(logout, logoutRequest)
	if logout.Code != http.StatusSeeOther || logout.Header().Get("Location") != "/login" {
		t.Fatalf("logout response: status=%d location=%q", logout.Code, logout.Header().Get("Location"))
	}
	revoked := httptest.NewRecorder()
	revokedRequest := httptest.NewRequest(http.MethodGet, "https://momentum.test/list", nil)
	revokedRequest.AddCookie(cookies[0])
	protected.ServeHTTP(revoked, revokedRequest)
	if revoked.Code != http.StatusSeeOther {
		t.Fatalf("revoked session was accepted: status=%d", revoked.Code)
	}
}

func TestAuthPagesRejectCrossSiteMutationAndUnsafeReturnURL(t *testing.T) {
	database := newAuthPageTestDB(t)
	service := auth.NewService(database)
	pages := NewAuthPageHandler(service)

	form := url.Values{"email": {"owner@example.test"}, "password": {"correct horse battery staple"}}
	crossSite := httptest.NewRequest(http.MethodPost, "https://momentum.test/register", strings.NewReader(form.Encode()))
	crossSite.Host = "momentum.test"
	crossSite.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	crossSite.Header.Set("Origin", "https://evil.example")
	crossResponse := httptest.NewRecorder()
	pages.HandleRegister(crossResponse, crossSite)
	if crossResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-site registration status=%d body=%s", crossResponse.Code, crossResponse.Body.String())
	}

	unsafe := httptest.NewRecorder()
	pages.HandleLogin(unsafe, httptest.NewRequest(http.MethodGet, "https://momentum.test/login?return=https%3A%2F%2Fevil.example", nil))
	if unsafe.Code != http.StatusOK || strings.Contains(unsafe.Body.String(), "evil.example") {
		t.Fatalf("unsafe return URL appeared in page: status=%d body=%s", unsafe.Code, unsafe.Body.String())
	}
}
