package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	syncengine "github.com/chrisbelyea/momentum/internal/sync"
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

// Mapping converts durable provider state into the provider-neutral planner
// input. Keeping this adapter here prevents reconciliation code from knowing
// about SQLite row layouts while preserving the exact checkpoint baseline.
func (e SyncEntity) Mapping() syncengine.EntityMapping {
	return syncengine.EntityMapping{
		BackendID:      e.BackendID,
		TaskID:         e.TaskID,
		RemoteUID:      e.RemoteUID,
		RemoteETag:     e.RemoteETag,
		RemoteSequence: e.RemoteSequence,
		LastPulledAt:   e.LastPulledAt,
	}
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

// ListEntities returns the durable provider mappings used as the baseline for
// reconciliation. A missing baseline is represented by nil timestamps rather
// than being guessed as a local edit.
func (r *SyncRepository) ListEntities(ctx context.Context, backendID int) ([]SyncEntity, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,backend_id,task_id,remote_uid,remote_href,remote_etag,remote_sequence,state,last_pulled_at,last_pushed_at,deleted_at FROM sync_entities WHERE backend_id=? ORDER BY id`, backendID)
	if err != nil {
		return nil, fmt.Errorf("list sync entities: %w", err)
	}
	defer rows.Close()
	var entities []SyncEntity
	for rows.Next() {
		var entity SyncEntity
		var taskID, remoteSequence sql.NullInt64
		var href, etag, state sql.NullString
		var pulled, pushed, deleted sql.NullString
		if err := rows.Scan(&entity.ID, &entity.BackendID, &taskID, &entity.RemoteUID, &href, &etag, &remoteSequence, &state, &pulled, &pushed, &deleted); err != nil {
			return nil, fmt.Errorf("scan sync entity: %w", err)
		}
		if taskID.Valid {
			value := int(taskID.Int64)
			entity.TaskID = &value
		}
		entity.RemoteHref, entity.RemoteETag, entity.State = href.String, etag.String, state.String
		if remoteSequence.Valid {
			value := int(remoteSequence.Int64)
			entity.RemoteSequence = &value
		}
		for _, item := range []struct {
			value  sql.NullString
			target **time.Time
			name   string
		}{{pulled, &entity.LastPulledAt, "last_pulled_at"}, {pushed, &entity.LastPushedAt, "last_pushed_at"}, {deleted, &entity.DeletedAt, "deleted_at"}} {
			if item.value.Valid {
				parsed, parseErr := parseTimestamp(item.value.String)
				if parseErr != nil {
					return nil, fmt.Errorf("parse sync entity %s: %w", item.name, parseErr)
				}
				*item.target = &parsed
			}
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sync entities: %w", err)
	}
	return entities, nil
}

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
