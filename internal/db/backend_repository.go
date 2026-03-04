package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chrisbelyea/momentum/internal/crypto"
	"github.com/chrisbelyea/momentum/internal/models"
)

// BackendRepository handles backend configuration database operations
type BackendRepository struct {
	db *sql.DB
}

// NewBackendRepository creates a new BackendRepository
func NewBackendRepository(db *sql.DB) *BackendRepository {
	return &BackendRepository{db: db}
}

// List returns all backends for a given user
func (r *BackendRepository) List(userID string) ([]*models.Backend, error) {
	query := `
		SELECT id, user_id, backend_type, name, config_encrypted, created_at, updated_at
		FROM backends
		WHERE user_id = ?
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query backends: %w", err)
	}
	defer rows.Close()

	var backends []*models.Backend
	for rows.Next() {
		var backend models.Backend
		var configEncrypted sql.NullString
		var createdAt, updatedAt string

		err := rows.Scan(
			&backend.ID,
			&backend.UserID,
			&backend.Type,
			&backend.Name,
			&configEncrypted,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan backend: %w", err)
		}

		// Parse timestamps
		backend.CreatedAt, err = parseTimestamp(createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to parse created_at: %w", err)
		}
		backend.UpdatedAt, err = parseTimestamp(updatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to parse updated_at: %w", err)
		}

		// Decrypt and parse config if present
		if configEncrypted.Valid && configEncrypted.String != "" {
			config, err := r.decryptConfig(configEncrypted.String)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt config: %w", err)
			}
			backend.Config = config
		}

		backends = append(backends, &backend)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating backends: %w", err)
	}

	return backends, nil
}

// Get returns a single backend by ID
func (r *BackendRepository) Get(id string) (*models.Backend, error) {
	query := `
		SELECT id, user_id, backend_type, name, config_encrypted, created_at, updated_at
		FROM backends
		WHERE id = ?
	`

	var backend models.Backend
	var configEncrypted sql.NullString
	var createdAt, updatedAt string

	err := r.db.QueryRow(query, id).Scan(
		&backend.ID,
		&backend.UserID,
		&backend.Type,
		&backend.Name,
		&configEncrypted,
		&createdAt,
		&updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get backend: %w", err)
	}

	// Parse timestamps
	backend.CreatedAt, err = parseTimestamp(createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse created_at: %w", err)
	}
	backend.UpdatedAt, err = parseTimestamp(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse updated_at: %w", err)
	}

	// Decrypt and parse config if present
	if configEncrypted.Valid && configEncrypted.String != "" {
		config, err := r.decryptConfig(configEncrypted.String)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt config: %w", err)
		}
		backend.Config = config
	}

	return &backend, nil
}

// Create creates a new backend
func (r *BackendRepository) Create(backend *models.Backend) error {
	// Validate backend
	if err := backend.Validate(); err != nil {
		return err
	}

	// Set timestamps
	now := time.Now()
	backend.CreatedAt = now
	backend.UpdatedAt = now

	// Encrypt config if present
	var configEncrypted sql.NullString
	if backend.Config != nil {
		encrypted, err := r.encryptConfig(backend.Config)
		if err != nil {
			return fmt.Errorf("failed to encrypt config: %w", err)
		}
		configEncrypted = sql.NullString{String: encrypted, Valid: true}
	}

	query := `
		INSERT INTO backends (
			id, user_id, backend_type, name, config_encrypted, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.Exec(
		query,
		backend.ID,
		backend.UserID,
		backend.Type,
		backend.Name,
		configEncrypted,
		backend.CreatedAt,
		backend.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}

	return nil
}

// Update updates an existing backend
func (r *BackendRepository) Update(backend *models.Backend) error {
	// Validate backend
	if err := backend.Validate(); err != nil {
		return err
	}

	// Update timestamp
	now := time.Now()
	backend.UpdatedAt = now

	// Encrypt config if present
	var configEncrypted sql.NullString
	if backend.Config != nil {
		encrypted, err := r.encryptConfig(backend.Config)
		if err != nil {
			return fmt.Errorf("failed to encrypt config: %w", err)
		}
		configEncrypted = sql.NullString{String: encrypted, Valid: true}
	}

	query := `
		UPDATE backends
		SET backend_type = ?, name = ?, config_encrypted = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := r.db.Exec(
		query,
		backend.Type,
		backend.Name,
		configEncrypted,
		backend.UpdatedAt,
		backend.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update backend: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// Delete deletes a backend by ID
func (r *BackendRepository) Delete(id string) error {
	query := `DELETE FROM backends WHERE id = ?`

	result, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete backend: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// encryptConfig encrypts a BackendConfig
func (r *BackendRepository) encryptConfig(config *models.BackendConfig) (string, error) {
	// Serialize config to JSON
	jsonData, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	// Encrypt the JSON
	encrypted, err := crypto.Encrypt(string(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to encrypt config: %w", err)
	}

	return encrypted, nil
}

// decryptConfig decrypts a BackendConfig
func (r *BackendRepository) decryptConfig(encrypted string) (*models.BackendConfig, error) {
	// Decrypt the data
	decrypted, err := crypto.Decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt config: %w", err)
	}

	// Parse JSON
	var config models.BackendConfig
	if err := json.Unmarshal([]byte(decrypted), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

// parseTimestamp parses a timestamp string
func parseTimestamp(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.999999999Z07:00",
	}

	for _, format := range formats {
		t, err := time.Parse(format, s)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", s)
}
