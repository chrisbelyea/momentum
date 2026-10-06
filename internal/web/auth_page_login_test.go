package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/chrisbelyea/momentum/internal/auth"
)

func TestAuthPagesRenderedLoginWithSameOrigin(t *testing.T) {
	database := newAuthPageTestDB(t)
	service := auth.NewService(database)
	pages := NewAuthPageHandler(service)
	if _, err := service.CreateUser("owner@example.test", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	page := httptest.NewRecorder()
	pages.HandleLogin(page, httptest.NewRequest(http.MethodGet, "https://momentum.test/login?return=%2Flist", nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `action="/login"`) {
		t.Fatalf("rendered login page status=%d body=%s", page.Code, page.Body.String())
	}
	form := url.Values{"email": {"owner@example.test"}, "password": {"correct horse battery staple"}, "return": {"/list"}}
	request := httptest.NewRequest(http.MethodPost, "https://momentum.test/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "https://momentum.test")
	login := httptest.NewRecorder()
	pages.HandleLogin(login, request)
	if login.Code != http.StatusSeeOther || login.Header().Get("Location") != "/list" {
		t.Fatalf("rendered login status=%d location=%s", login.Code, login.Header().Get("Location"))
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("rendered login session cookies=%d", len(cookies))
	}
	protected := httptest.NewRequest(http.MethodGet, "https://momentum.test/list", nil)
	protected.AddCookie(cookies[0])
	if _, err := service.UserID(protected); err != nil {
		t.Fatalf("rendered login session rejected: %v", err)
	}
}
