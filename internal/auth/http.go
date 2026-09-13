package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

type contextKey string

const userIDKey contextKey = "momentum.user_id"

func UserIDFromRequest(r *http.Request) (int, bool) {
	id, ok := r.Context().Value(userIDKey).(int)
	return id, ok && id > 0
}

func (s *Service) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isMutation(r.Method) && !SameOrigin(r) {
			http.Error(w, "cross-site request rejected", http.StatusForbidden)
			return
		}
		id, err := s.UserID(r)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, contextWithUser(r, id))
	})
}

// RequirePage protects an HTML navigation while preserving API clients'
// machine-readable 401 response from Require. The original request is passed
// through a validated relative return URL so login cannot become an open
// redirect.
func (s *Service) RequirePage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := s.UserID(r)
		if err != nil {
			returnURL := SafeReturnURL(r.URL.RequestURI())
			location := "/login?return=" + url.QueryEscape(returnURL)
			http.Redirect(w, r, location, http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, contextWithUser(r, id))
	})
}

func isMutation(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

// SameOrigin permits non-browser API clients that omit Origin, but rejects an
// explicitly supplied cross-site Origin on every state-changing route.
func SameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	return origin == "" || origin == "https://"+r.Host || origin == "http://"+r.Host
}

// SafeReturnURL returns only an in-origin relative request target.
func SafeReturnURL(raw string) string {
	if raw == "" {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return "/"
	}
	if u.Path == "" {
		return "/"
	}
	return u.RequestURI()
}

func contextWithUser(r *http.Request, id int) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userIDKey, id))
}

func (s *Service) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		if !SameOrigin(r) {
			http.Error(w, "cross-site request rejected", http.StatusForbidden)
			return
		}
		var in struct{ Email, Password string }
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		if !s.RegistrationAvailable() {
			http.Error(w, ErrRegistrationClosed.Error(), http.StatusForbidden)
			return
		}
		id, err := s.CreateUser(in.Email, in.Password)
		if err != nil {
			http.Error(w, "registration failed", 400)
			return
		}
		_ = s.Login(w, id)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int{"user_id": id})
	})
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		if !SameOrigin(r) {
			http.Error(w, "cross-site request rejected", http.StatusForbidden)
			return
		}
		var in struct{ Email, Password string }
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		id, err := s.Authenticate(in.Email, in.Password)
		if err != nil {
			http.Error(w, "invalid credentials", 401)
			return
		}
		if err := s.Login(w, id); err != nil {
			http.Error(w, "login failed", 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]int{"user_id": id})
	})
	mux.HandleFunc("/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		if !SameOrigin(r) {
			http.Error(w, "cross-site request rejected", http.StatusForbidden)
			return
		}
		s.Logout(w, r)
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}
