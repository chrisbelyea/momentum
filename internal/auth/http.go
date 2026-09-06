package auth

import (
	"context"
	"encoding/json"
	"net/http"
)

type contextKey string

const userIDKey contextKey = "momentum.user_id"

func UserIDFromRequest(r *http.Request) (int, bool) {
	id, ok := r.Context().Value(userIDKey).(int)
	return id, ok && id > 0
}

func (s *Service) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
			if origin := r.Header.Get("Origin"); origin != "" && origin != "https://"+r.Host && origin != "http://"+r.Host {
				http.Error(w, "cross-site request rejected", http.StatusForbidden)
				return
			}
		}
		id, err := s.UserID(r)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, contextWithUser(r, id))
	})
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
		var in struct{ Email, Password string }
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "invalid request", 400)
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
		s.Logout(w, r)
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}
