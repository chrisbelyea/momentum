package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/chrisbelyea/momentum/internal/auth"
)

func TestAuthPagesRejectOriginlessAndCrossSchemeForms(t *testing.T) {
	for _, origin := range []string{"", "http://momentum.test", "https://evil.example"} {
		t.Run("origin="+origin, func(t *testing.T) {
			database := newAuthPageTestDB(t)
			service := auth.NewService(database)
			pages := NewAuthPageHandler(service)
			form := url.Values{"email": {"owner@example.test"}, "password": {"correct horse battery staple"}}
			request := func(path string, cookie *http.Cookie) *http.Request {
				r := httptest.NewRequest(http.MethodPost, "https://momentum.test"+path, strings.NewReader(form.Encode()))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if origin != "" {
					r.Header.Set("Origin", origin)
				}
				if cookie != nil {
					r.AddCookie(cookie)
				}
				return r
			}

			// An attacker cannot seize the first account through an originless form.
			register := httptest.NewRecorder()
			pages.HandleRegister(register, request("/register", nil))
			if register.Code != http.StatusForbidden || !service.RegistrationAvailable() {
				t.Fatalf("form registered first account: status=%d", register.Code)
			}

			// Non-browser JSON clients retain their originless registration path.
			jsonRequest := httptest.NewRequest(http.MethodPost, "https://momentum.test/auth/register", strings.NewReader(`{"email":"owner@example.test","password":"correct horse battery staple"}`))
			jsonRequest.Header.Set("Content-Type", "application/json")
			jsonResponse := httptest.NewRecorder()
			service.Routes().ServeHTTP(jsonResponse, jsonRequest)
			if jsonResponse.Code != http.StatusCreated {
				t.Fatalf("originless JSON registration status=%d body=%s", jsonResponse.Code, jsonResponse.Body.String())
			}
			cookies := jsonResponse.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("originless JSON registration session cookies=%d", len(cookies))
			}
			cookie := cookies[0]

			login := httptest.NewRecorder()
			pages.HandleLogin(login, request("/login", nil))
			if login.Code != http.StatusForbidden || len(login.Result().Cookies()) != 0 {
				t.Fatalf("form login changed session: status=%d", login.Code)
			}

			logout := httptest.NewRecorder()
			pages.HandleLogout(logout, request("/logout", cookie))
			if logout.Code != http.StatusForbidden || len(logout.Result().Cookies()) != 0 {
				t.Fatalf("form logout changed session: status=%d", logout.Code)
			}
			protected := httptest.NewRequest(http.MethodGet, "https://momentum.test/list", nil)
			protected.AddCookie(cookie)
			if _, err := service.UserID(protected); err != nil {
				t.Fatalf("form logout revoked session: %v", err)
			}
		})
	}
}
