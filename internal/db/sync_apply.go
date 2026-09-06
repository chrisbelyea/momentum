package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chrisbelyea/momentum/internal/models"
	syncengine "github.com/chrisbelyea/momentum/internal/sync"
)

// ApplyCycle atomically applies a completed sync cycle to the canonical task
// store and durable sync ledger. A crash before commit leaves both the tasks
// and checkpoint at their previous state, so a subsequent cycle can safely
// retry the provider operations.
func (r *SyncRepository) ApplyCycle(ctx context.Context, checkpoint SyncCheckpoint, cycle syncengine.CycleResult) error {
	if checkpoint.BackendID <= 0 {
		return fmt.Errorf("backend ID must be positive")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sync cycle transaction: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	for _, action := range cycle.Plan.Actions {
		if err := applyAction(ctx, tx, checkpoint.BackendID, action, cycle.PushResults, now); err != nil {
			return err
		}
	}
	checkpoint.Cursor = cycle.NextCursor
	checkpoint.Status = "complete"
	checkpoint.LastCompletedAt = &now
	checkpoint.LastError = ""
	if err := upsertCheckpoint(ctx, tx, checkpoint, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sync cycle transaction: %w", err)
	}
	return nil
}

func applyAction(ctx context.Context, tx *sql.Tx, backendID int, action syncengine.ReconcileAction, pushes map[string]syncengine.PushResult, now time.Time) error {
	if action.Task != nil && action.Task.BackendID != 0 && action.Task.BackendID != backendID {
		return fmt.Errorf("sync action belongs to backend %d, want %d", action.Task.BackendID, backendID)
	}
	switch action.Kind {
	case syncengine.ActionImport:
		task := actionTask(action)
		task.BackendID = backendID
		if err := insertTask(ctx, tx, task, now); err != nil {
			return err
		}
		remote := action.Remote
		if remote == nil {
			return fmt.Errorf("import action has no remote entity")
		}
		return upsertEntity(ctx, tx, backendID, &task.ID, remote.RemoteUID, remote.RemoteHref, remote.ETag, remote.RemoteSequence, "active", now, nil)
	case syncengine.ActionUpdateLocal:
		if action.Task == nil || action.Remote == nil {
			return fmt.Errorf("update-local action is incomplete")
		}
		updated := action.Remote.Task
		updated.ID, updated.BackendID = action.Task.ID, backendID
		if err := updateTask(ctx, tx, &updated, now); err != nil {
			return err
		}
		return upsertEntity(ctx, tx, backendID, &updated.ID, action.Remote.RemoteUID, action.Remote.RemoteHref, action.Remote.ETag, action.Remote.RemoteSequence, "active", now, nil)
	case syncengine.ActionDeleteLocal:
		if action.Task == nil {
			return fmt.Errorf("delete-local action has no task")
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM tasks WHERE id=? AND backend_id=?", action.Task.ID, backendID); err != nil {
			return fmt.Errorf("delete local synced task: %w", err)
		}
	case syncengine.ActionPush:
		if action.Task == nil {
			return fmt.Errorf("push action has no task")
		}
		remoteUID := action.Task.UID
		if action.Mapping != nil && action.Mapping.RemoteUID != "" {
			remoteUID = action.Mapping.RemoteUID
		}
		pushed, ok := pushes[remoteUID]
		if !ok {
			return fmt.Errorf("push result missing for %q", remoteUID)
		}
		return upsertEntity(ctx, tx, backendID, &action.Task.ID, pushed.RemoteUID, pushed.RemoteHref, pushed.ETag, pushed.RemoteSequence, "active", now, nil)
	case syncengine.ActionDeleteRemote:
		if action.Mapping == nil {
			return fmt.Errorf("delete-remote action has no mapping")
		}
		if _, err := tx.ExecContext(ctx, "UPDATE sync_entities SET state='deleted', deleted_at=?, updated_at=? WHERE backend_id=? AND remote_uid=?", now, now, backendID, action.Mapping.RemoteUID); err != nil {
			return fmt.Errorf("mark remote entity deleted: %w", err)
		}
	case syncengine.ActionConflict:
		if action.Task == nil || action.Remote == nil {
			return fmt.Errorf("conflict action is incomplete")
		}
		local, _ := json.Marshal(action.Task)
		remote, _ := json.Marshal(action.Remote.Task)
		var taskID *int
		if action.Task.ID > 0 {
			taskID = &action.Task.ID
		}
		var entityID *int
		if action.Mapping != nil && action.Mapping.RemoteUID != "" {
			var id int
			if err := tx.QueryRowContext(ctx, "SELECT id FROM sync_entities WHERE backend_id=? AND remote_uid=?", backendID, action.Mapping.RemoteUID).Scan(&id); err == nil {
				entityID = &id
			}
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO sync_conflicts(backend_id,entity_id,task_id,local_snapshot,remote_snapshot,policy,status) VALUES(?,?,?,?,?,?,?)", backendID, entityID, taskID, string(local), string(remote), "manual", "open"); err != nil {
			return fmt.Errorf("record sync conflict: %w", err)
		}
	}
	return nil
}

func actionTask(action syncengine.ReconcileAction) *models.Task {
	if action.Task != nil {
		copy := *action.Task
		return &copy
	}
	if action.Remote != nil {
		copy := action.Remote.Task
		return &copy
	}
	return &models.Task{}
}

func insertTask(ctx context.Context, tx *sql.Tx, task *models.Task, now time.Time) error {
	if task.UID == "" || task.Title == "" {
		return fmt.Errorf("cannot import task without UID and title")
	}
	if task.Status == "" {
		task.Status = models.StatusNeedsAction
	}
	if task.DTStamp.IsZero() {
		task.DTStamp = now
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt == nil {
		task.UpdatedAt = &now
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO tasks(backend_id,uid,external_id,title,description,status,priority,due_at,due_date_only,start_at,start_date_only,completed_at,dtstamp,last_modified,sequence,percent_complete,tags_json,related_to_json,url,location,extra_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, task.BackendID, task.UID, task.ExternalID, task.Title, task.Description, task.Status, task.Priority, task.DueAt, task.DueDateOnly, task.StartAt, task.StartDateOnly, task.CompletedAt, task.DTStamp, task.LastModified, task.Sequence, task.PercentComplete, task.TagsJSON, task.RelatedToJSON, task.URL, task.Location, task.ExtraJSON, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return fmt.Errorf("import task: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read imported task ID: %w", err)
	}
	task.ID = int(id)
	return nil
}

func updateTask(ctx context.Context, tx *sql.Tx, task *models.Task, now time.Time) error {
	task.UpdatedAt = &now
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET uid=?,external_id=?,title=?,description=?,status=?,priority=?,due_at=?,due_date_only=?,start_at=?,start_date_only=?,completed_at=?,dtstamp=?,last_modified=?,sequence=?,percent_complete=?,tags_json=?,related_to_json=?,url=?,location=?,extra_json=?,updated_at=? WHERE id=? AND backend_id=?`, task.UID, task.ExternalID, task.Title, task.Description, task.Status, task.Priority, task.DueAt, task.DueDateOnly, task.StartAt, task.StartDateOnly, task.CompletedAt, task.DTStamp, task.LastModified, task.Sequence, task.PercentComplete, task.TagsJSON, task.RelatedToJSON, task.URL, task.Location, task.ExtraJSON, task.UpdatedAt, task.ID, task.BackendID)
	if err != nil {
		return fmt.Errorf("apply remote task update: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return fmt.Errorf("remote task %d was not found", task.ID)
	}
	return nil
}

func upsertEntity(ctx context.Context, tx *sql.Tx, backendID int, taskID *int, uid, href, etag string, sequence *int, state string, now time.Time, deletedAt *time.Time) error {
	if uid == "" {
		return fmt.Errorf("sync entity UID is required")
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO sync_entities(backend_id,task_id,remote_uid,remote_href,remote_etag,remote_sequence,state,last_pulled_at,last_pushed_at,deleted_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(backend_id,remote_uid) DO UPDATE SET task_id=excluded.task_id,remote_href=excluded.remote_href,remote_etag=excluded.remote_etag,remote_sequence=excluded.remote_sequence,state=excluded.state,last_pulled_at=excluded.last_pulled_at,last_pushed_at=excluded.last_pushed_at,deleted_at=excluded.deleted_at,updated_at=excluded.updated_at`, backendID, taskID, uid, href, etag, sequence, state, now, now, deletedAt, now)
	if err != nil {
		return fmt.Errorf("persist sync entity %q: %w", uid, err)
	}
	return nil
}

func upsertCheckpoint(ctx context.Context, tx *sql.Tx, checkpoint SyncCheckpoint, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sync_checkpoints(backend_id,cursor,status,last_started_at,last_completed_at,last_error,retry_count,next_retry_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(backend_id) DO UPDATE SET cursor=excluded.cursor,status=excluded.status,last_started_at=excluded.last_started_at,last_completed_at=excluded.last_completed_at,last_error=excluded.last_error,retry_count=excluded.retry_count,next_retry_at=excluded.next_retry_at,updated_at=excluded.updated_at`, checkpoint.BackendID, checkpoint.Cursor, checkpoint.Status, checkpoint.LastStartedAt, checkpoint.LastCompletedAt, checkpoint.LastError, checkpoint.RetryCount, checkpoint.NextRetryAt, now)
	if err != nil {
		return fmt.Errorf("persist sync checkpoint: %w", err)
	}
	return nil
}
