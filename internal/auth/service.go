// Package auth implements Momentum's cookie-based account and session service.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated    = errors.New("authentication required")
	ErrRegistrationClosed = errors.New("registration is closed")
)

const (
	credentialType = "password"
	cookieName     = "momentum_session"
	sessionTTL     = 24 * time.Hour
)

type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }

func (s *Service) CreateUser(email, password string) (int, error) {
	if strings.TrimSpace(email) == "" || len(password) < 12 {
		return 0, ErrInvalidCredentials
	}
	hash, err := hashPassword(password)
	if err != nil {
		return 0, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var credentialCount int
	if err := tx.QueryRow("SELECT COUNT(*) FROM credentials WHERE type=?", credentialType).Scan(&credentialCount); err != nil {
		return 0, err
	}
	if credentialCount > 0 {
		return 0, ErrRegistrationClosed
	}
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	var uncredentialedUsers int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM users u WHERE NOT EXISTS (
		SELECT 1 FROM credentials c WHERE c.user_id=u.id AND c.type=?
	)`, credentialType).Scan(&uncredentialedUsers); err != nil {
		return 0, err
	}
	var id int64
	switch uncredentialedUsers {
	case 0:
		result, insertErr := tx.Exec("INSERT INTO users(email) VALUES(?)", normalizedEmail)
		if insertErr != nil {
			return 0, fmt.Errorf("create user: %w", insertErr)
		}
		id, err = result.LastInsertId()
		if err != nil {
			return 0, err
		}
	case 1:
		// The canonical schema and pre-authentication releases may already
		// contain one password-less compatibility user with local tasks. Adopt
		// that identity instead of stranding its backend and task rows.
		if err := tx.QueryRow(`SELECT u.id FROM users u WHERE NOT EXISTS (
			SELECT 1 FROM credentials c WHERE c.user_id=u.id AND c.type=?
		) ORDER BY u.id LIMIT 1`, credentialType).Scan(&id); err != nil {
			return 0, err
		}
		if _, err := tx.Exec("UPDATE users SET email=?, updated_at=CURRENT_TIMESTAMP WHERE id=?", normalizedEmail, id); err != nil {
			return 0, fmt.Errorf("adopt existing user: %w", err)
		}
	default:
		// Multiple password-less identities cannot be safely assigned to the
		// first account without an operator choice; fail closed rather than
		// exposing another user's legacy tasks.
		return 0, ErrRegistrationClosed
	}
	if _, err = tx.Exec("INSERT INTO credentials(user_id,type,secret_hash) VALUES(?,?,?)", id, credentialType, hash); err != nil {
		return 0, fmt.Errorf("create credential: %w", err)
	}
	var backendCount int
	if err := tx.QueryRow("SELECT COUNT(*) FROM backends WHERE user_id=?", id).Scan(&backendCount); err != nil {
		return 0, err
	}
	if backendCount == 0 {
		if _, err = tx.Exec("INSERT INTO backends(user_id,backend_type,name) VALUES(?,?,?)", id, "internal", "Local tasks"); err != nil {
			return 0, fmt.Errorf("create default backend: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(id), nil
}

// RegistrationAvailable reports whether this self-hosted instance still needs
// its first password-backed account. Momentum's Phase 1 identity policy allows
// one initial account; additional account/invitation administration is a later
// feature rather than silently exposing open registration.
func (s *Service) RegistrationAvailable() bool {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM credentials WHERE type=?", credentialType).Scan(&count); err != nil {
		return false
	}
	return count == 0
}

func (s *Service) Authenticate(email, password string) (int, error) {
	var id int
	var encoded string
	err := s.db.QueryRow(`SELECT u.id,c.secret_hash FROM users u JOIN credentials c ON c.user_id=u.id AND c.type=? WHERE u.email=?`, credentialType, strings.ToLower(strings.TrimSpace(email))).Scan(&id, &encoded)
	if err != nil || !verifyPassword(encoded, password) {
		return 0, ErrInvalidCredentials
	}
	return id, nil
}

func (s *Service) Login(w http.ResponseWriter, userID int) error {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return err
	}
	expires := time.Now().Add(sessionTTL)
	digest := sha256.Sum256(token)
	if _, err := s.db.Exec("INSERT INTO sessions(id,user_id,expires_at) VALUES(?,?,?)", base64.RawURLEncoding.EncodeToString(digest[:]), userID, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: base64.RawURLEncoding.EncodeToString(token), Path: "/", Expires: expires, MaxAge: int(sessionTTL.Seconds()), Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	return nil
}

func (s *Service) UserID(r *http.Request) (int, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return 0, ErrUnauthenticated
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return 0, ErrUnauthenticated
	}
	digest := sha256.Sum256(raw)
	var id int
	err = s.db.QueryRow("SELECT user_id FROM sessions WHERE id=? AND expires_at > CURRENT_TIMESTAMP", base64.RawURLEncoding.EncodeToString(digest[:])).Scan(&id)
	if err != nil {
		return 0, ErrUnauthenticated
	}
	return id, nil
}

func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(cookieName); err == nil {
		if raw, err := base64.RawURLEncoding.DecodeString(cookie.Value); err == nil {
			digest := sha256.Sum256(raw)
			_, _ = s.db.Exec("DELETE FROM sessions WHERE id=?", base64.RawURLEncoding.EncodeToString(digest[:]))
		}
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0), Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func (s *Service) CleanupExpired() error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE expires_at <= CURRENT_TIMESTAMP")
	return err
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	const memory, iterations, threads, keyLen = 64 * 1024, 3, 4, 32
	key := argon2.IDKey([]byte(password), salt, iterations, memory, threads, keyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, threads, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory, iterations, threads uint32
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, uint8(threads), uint32(len(expected)))
	if len(actual) != len(expected) {
		return false
	}
	var diff byte
	for i := range actual {
		diff |= actual[i] ^ expected[i]
	}
	return diff == 0
}
