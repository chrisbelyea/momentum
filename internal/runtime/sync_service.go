// Package runtime wires the provider-neutral sync engine into the running
// Momentum server. The engine itself remains transport and storage agnostic;
// this package owns backend configuration, durable checkpoints, and lifecycle.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chrisbelyea/momentum/internal/auth"
	"github.com/chrisbelyea/momentum/internal/caldav"
	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/models"
	syncengine "github.com/chrisbelyea/momentum/internal/sync"
)

var ErrSyncInProgress = errors.New("sync is already running for this backend")

// Result is the user-visible outcome of a synchronization pass.
type Result struct {
	BackendID  int               `json:"backend_id"`
	Collection string            `json:"collection"`
	Checkpoint db.SyncCheckpoint `json:"checkpoint"`
}

// Service coordinates one bounded sync pass per backend and optionally runs a
// periodic scheduler. A backend cannot be run concurrently by a manual call
// and the scheduler, preventing duplicate provider mutations.
type Service struct {
	backendRepo *db.BackendRepository
	taskRepo    *db.TaskRepository
	syncRepo    *db.SyncRepository
	runner      syncengine.Runner
	mu          sync.Mutex
	running     map[int]bool
}

func NewService(backendRepo *db.BackendRepository, taskRepo *db.TaskRepository, syncRepo *db.SyncRepository) *Service {
	return &Service{
		backendRepo: backendRepo,
		taskRepo:    taskRepo,
		syncRepo:    syncRepo,
		runner: syncengine.Runner{
			Policy: syncengine.DefaultRetryPolicy,
		},
		running: make(map[int]bool),
	}
}

// RunBackend executes a complete bounded cycle for one external CalDAV
// backend. CalendarPath is used when configured; otherwise the first
// VTODO-capable discovered collection is selected for this pass.
func (s *Service) RunBackend(ctx context.Context, backendID int) (Result, error) {
	if backendID <= 0 {
		return Result{}, fmt.Errorf("backend ID must be positive")
	}
	if !s.begin(backendID) {
		return Result{}, ErrSyncInProgress
	}
	defer s.end(backendID)

	backend, err := s.backendRepo.Get(backendID)
	if err != nil {
		return Result{}, err
	}
	if backend == nil {
		return Result{}, db.ErrNotFound
	}
	if backend.Type != models.BackendTypeExternalCalDAV || backend.Config == nil {
		return Result{}, fmt.Errorf("backend %d is not an external CalDAV backend", backendID)
	}
	client, err := caldav.NewClient(backend.Config)
	if err != nil {
		return Result{}, fmt.Errorf("create CalDAV client: %w", err)
	}
	defer client.Close()
	collection := strings.TrimSpace(backend.Config.CalendarPath)
	if collection == "" {
		collections, discoverErr := client.DiscoverCollections(ctx)
		if discoverErr != nil {
			return s.failed(ctx, backendID, discoverErr)
		}
		for _, candidate := range collections {
			for _, component := range candidate.Components {
				if strings.EqualFold(component, "VTODO") {
					collection = candidate.Href
					break
				}
			}
			if collection != "" {
				break
			}
		}
		if collection == "" && len(collections) > 0 {
			collection = collections[0].Href
		}
	}
	if collection == "" {
		return s.failed(ctx, backendID, fmt.Errorf("no CalDAV calendar collection was found"))
	}

	tasks, err := s.taskRepo.List(backendID)
	if err != nil {
		return Result{}, err
	}
	entities, err := s.syncRepo.ListEntities(ctx, backendID)
	if err != nil {
		return Result{}, err
	}
	checkpoint, err := s.syncRepo.GetCheckpoint(backendID)
	if err != nil {
		return Result{}, err
	}
	inputCheckpoint := db.SyncCheckpoint{BackendID: backendID}
	if checkpoint != nil {
		inputCheckpoint = *checkpoint
	}
	now := time.Now().UTC()
	started := inputCheckpoint
	started.Status, started.LastStartedAt, started.LastError = "running", &now, ""
	if err := s.syncRepo.ApplyBatch(ctx, started, nil, nil, nil); err != nil {
		return Result{}, err
	}

	mappings := make([]syncengine.EntityMapping, 0, len(entities))
	for _, entity := range entities {
		mappings = append(mappings, entity.Mapping())
	}
	adapter, err := caldav.NewSyncAdapter(client, collection)
	if err != nil {
		return Result{}, err
	}
	// Persist each provider attempt as it happens. These rows survive a
	// process interruption even when the cycle never reaches ApplyCycle, so a
	// restart can explain and account for partial work rather than presenting
	// an unexplained checkpoint transition.
	runner := s.runner
	observerContext := context.WithoutCancel(ctx)
	runner.Observer = func(event syncengine.SyncEvent) {
		operation := db.SyncOperation{
			BackendID: event.BackendID,
			Direction: event.Operation,
			Operation: event.Operation,
			Outcome:   event.Outcome,
			Error:     event.Error,
			Attempts:  event.Attempt,
		}
		if event.Outcome == "success" || event.Outcome == "failure" {
			now := time.Now().UTC()
			operation.CompletedAt = &now
		}
		_ = s.syncRepo.RecordOperation(observerContext, operation)
	}
	cycle, err := syncengine.RunCycle(ctx, adapter, runner, syncengine.CycleInput{
		BackendID: backendID,
		Cursor:    inputCheckpoint.Cursor,
		Local:     tasks,
		Mappings:  mappings,
	}, nil)
	if err != nil {
		return s.failed(ctx, backendID, err)
	}
	complete := started
	complete.Cursor = cycle.NextCursor
	complete.Status, complete.LastCompletedAt, complete.LastError = "complete", &now, ""
	complete.RetryCount, complete.NextRetryAt = 0, nil
	if err := s.syncRepo.ApplyCycle(ctx, complete, cycle); err != nil {
		return Result{}, err
	}
	return Result{BackendID: backendID, Collection: collection, Checkpoint: complete}, nil
}

func (s *Service) failed(ctx context.Context, backendID int, syncErr error) (Result, error) {
	now := time.Now().UTC()
	checkpoint := db.SyncCheckpoint{BackendID: backendID, Status: "failed", LastStartedAt: &now, LastError: syncErr.Error(), RetryCount: 1}
	if prior, err := s.syncRepo.GetCheckpoint(backendID); err == nil && prior != nil {
		checkpoint = *prior
		checkpoint.Status, checkpoint.LastError, checkpoint.LastStartedAt = "failed", syncErr.Error(), &now
		checkpoint.RetryCount++
	}
	if persistErr := s.syncRepo.ApplyBatch(ctx, checkpoint, nil, nil, nil); persistErr != nil {
		return Result{}, fmt.Errorf("sync failed: %v (record failure: %w)", syncErr, persistErr)
	}
	return Result{BackendID: backendID, Checkpoint: checkpoint}, syncErr
}

func (s *Service) begin(backendID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[backendID] {
		return false
	}
	s.running[backendID] = true
	return true
}

func (s *Service) end(backendID int) {
	s.mu.Lock()
	delete(s.running, backendID)
	s.mu.Unlock()
}

// StartScheduler starts a periodic all-backend worker. A non-positive interval
// disables scheduling, which is the default for installations that prefer an
// explicit POST /api/sync/run trigger.
func (s *Service) StartScheduler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				backends, err := s.backendRepo.ListAll()
				if err != nil {
					continue
				}
				for _, backend := range backends {
					if backend.Type == models.BackendTypeExternalCalDAV {
						_, _ = s.RunBackend(ctx, backend.ID)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// HandleRun is the authenticated manual trigger endpoint.
func (s *Service) HandleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	backendID, err := backendIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	backend, err := s.backendRepo.Get(backendID)
	if err != nil || backend == nil || backend.UserID != userID {
		http.Error(w, "backend not found", http.StatusNotFound)
		return
	}
	result, err := s.RunBackend(r.Context(), backendID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleStatus returns the durable checkpoint for an authenticated backend.
func (s *Service) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	backendID, err := backendIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	backend, err := s.backendRepo.Get(backendID)
	if err != nil || backend == nil || backend.UserID != userID {
		http.Error(w, "backend not found", http.StatusNotFound)
		return
	}
	checkpoint, err := s.syncRepo.GetCheckpoint(backendID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if checkpoint == nil {
		checkpoint = &db.SyncCheckpoint{BackendID: backendID, Status: "never"}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(checkpoint)
}

func backendIDFromRequest(r *http.Request) (int, error) {
	value := r.URL.Query().Get("backend_id")
	if value == "" {
		return 0, fmt.Errorf("backend_id is required")
	}
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("backend_id must be a positive integer")
	}
	return id, nil
}
