package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SyncCheckpoint is the durable cursor and retry state for one backend.
type SyncCheckpoint struct {
	BackendID       int
	Cursor          string
	Status          string
	LastStartedAt   *time.Time
	LastCompletedAt *time.Time
	LastError       string
	RetryCount      int
	NextRetryAt     *time.Time
	UpdatedAt       time.Time
}

// SyncEntity maps a canonical task to a provider entity. TaskID is nil while
// an imported remote entity is being staged for reconciliation.
type SyncEntity struct {
	ID             int
	BackendID      int
	TaskID         *int
	RemoteUID      string
	RemoteHref     string
	RemoteETag     string
	RemoteSequence *int
	State          string
	LastPulledAt   *time.Time
	LastPushedAt   *time.Time
	DeletedAt      *time.Time
}

// SyncOperation records an attempted provider operation, including failures
// that can be retried after a process restart.
type SyncOperation struct {
	BackendID     int
	EntityID      *int
	Direction     string
	Operation     string
	Outcome       string
	Error         string
	Attempts      int
	NextAttemptAt *time.Time
	CompletedAt   *time.Time
}

// SyncConflict retains both snapshots so conflict resolution never destroys
// evidence. Phase 1 uses the manual policy until a resolver is implemented.
type SyncConflict struct {
	BackendID      int
	EntityID       *int
	TaskID         *int
	LocalSnapshot  string
	RemoteSnapshot string
	Policy         string
	Status         string
	Resolution     string
}

// SyncRepository persists synchronization state.
type SyncRepository struct{ db *sql.DB }

func NewSyncRepository(db *sql.DB) *SyncRepository { return &SyncRepository{db: db} }

func (r *SyncRepository) GetCheckpoint(backendID int) (*SyncCheckpoint, error) {
	var c SyncCheckpoint
	var cursor, status, lastError sql.NullString
	var started, completed, retryAt, updated sql.NullString
	err := r.db.QueryRow(`SELECT backend_id,cursor,status,last_started_at,last_completed_at,last_error,retry_count,next_retry_at,updated_at FROM sync_checkpoints WHERE backend_id=?`, backendID).
		Scan(&c.BackendID, &cursor, &status, &started, &completed, &lastError, &c.RetryCount, &retryAt, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get sync checkpoint: %w", err)
	}
	c.Cursor, c.Status, c.LastError = cursor.String, status.String, lastError.String
	for _, item := range []struct {
		value  sql.NullString
		target **time.Time
	}{{started, &c.LastStartedAt}, {completed, &c.LastCompletedAt}, {retryAt, &c.NextRetryAt}} {
		if item.value.Valid {
			parsed, e := parseTimestamp(item.value.String)
			if e != nil {
				return nil, e
			}
			*item.target = &parsed
		}
	}
	parsed, err := parseTimestamp(updated.String)
	if err != nil {
		return nil, err
	}
	c.UpdatedAt = parsed
	return &c, nil
}

// ApplyBatch atomically records mappings, operation outcomes, conflicts, and
// the new checkpoint. A crash before commit leaves the previous checkpoint.
func (r *SyncRepository) ApplyBatch(ctx context.Context, checkpoint SyncCheckpoint, entities []SyncEntity, operations []SyncOperation, conflicts []SyncConflict) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sync transaction: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	for _, e := range entities {
		if e.BackendID != checkpoint.BackendID || e.RemoteUID == "" {
			return fmt.Errorf("invalid sync entity")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO sync_entities(backend_id,task_id,remote_uid,remote_href,remote_etag,remote_sequence,state,last_pulled_at,last_pushed_at,deleted_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(backend_id,remote_uid) DO UPDATE SET task_id=excluded.task_id,remote_href=excluded.remote_href,remote_etag=excluded.remote_etag,remote_sequence=excluded.remote_sequence,state=excluded.state,last_pulled_at=excluded.last_pulled_at,last_pushed_at=excluded.last_pushed_at,deleted_at=excluded.deleted_at,updated_at=excluded.updated_at`, e.BackendID, e.TaskID, e.RemoteUID, e.RemoteHref, e.RemoteETag, e.RemoteSequence, e.State, e.LastPulledAt, e.LastPushedAt, e.DeletedAt, now)
		if err != nil {
			return fmt.Errorf("record sync entity: %w", err)
		}
	}
	for _, o := range operations {
		if o.BackendID != checkpoint.BackendID {
			return fmt.Errorf("invalid sync operation")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO sync_operations(backend_id,entity_id,direction,operation,outcome,error,attempts,next_attempt_at,completed_at) VALUES(?,?,?,?,?,?,?,?,?)`, o.BackendID, o.EntityID, o.Direction, o.Operation, o.Outcome, o.Error, o.Attempts, o.NextAttemptAt, o.CompletedAt)
		if err != nil {
			return fmt.Errorf("record sync operation: %w", err)
		}
	}
	for _, c := range conflicts {
		if c.BackendID != checkpoint.BackendID || c.LocalSnapshot == "" || c.RemoteSnapshot == "" {
			return fmt.Errorf("invalid sync conflict")
		}
		if c.Policy == "" {
			c.Policy = "manual"
		}
		if c.Status == "" {
			c.Status = "open"
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO sync_conflicts(backend_id,entity_id,task_id,local_snapshot,remote_snapshot,policy,status,resolution) VALUES(?,?,?,?,?,?,?,?)`, c.BackendID, c.EntityID, c.TaskID, c.LocalSnapshot, c.RemoteSnapshot, c.Policy, c.Status, c.Resolution)
		if err != nil {
			return fmt.Errorf("record sync conflict: %w", err)
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sync_checkpoints(backend_id,cursor,status,last_started_at,last_completed_at,last_error,retry_count,next_retry_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(backend_id) DO UPDATE SET cursor=excluded.cursor,status=excluded.status,last_started_at=excluded.last_started_at,last_completed_at=excluded.last_completed_at,last_error=excluded.last_error,retry_count=excluded.retry_count,next_retry_at=excluded.next_retry_at,updated_at=excluded.updated_at`, checkpoint.BackendID, checkpoint.Cursor, checkpoint.Status, checkpoint.LastStartedAt, checkpoint.LastCompletedAt, checkpoint.LastError, checkpoint.RetryCount, checkpoint.NextRetryAt, now)
	if err != nil {
		return fmt.Errorf("record sync checkpoint: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit sync transaction: %w", err)
	}
	return nil
}
