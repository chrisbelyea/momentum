package caldav

import (
	"context"
	"errors"
	"fmt"

	syncengine "github.com/chrisbelyea/momentum/internal/sync"
)

// SyncAdapter connects the provider-neutral sync engine to one VTODO-capable
// CalDAV collection. Collection discovery/configuration remains outside the
// engine so credentials and backend selection stay application-owned.
type SyncAdapter struct {
	client     *Client
	collection string
}

func NewSyncAdapter(client *Client, collectionHref string) (*SyncAdapter, error) {
	if client == nil || collectionHref == "" {
		return nil, fmt.Errorf("CalDAV client and collection are required")
	}
	return &SyncAdapter{client: client, collection: collectionHref}, nil
}

func (a *SyncAdapter) Capabilities() syncengine.Capabilities {
	return syncengine.Capabilities(syncengine.CapabilityPull | syncengine.CapabilityIncrementalPull | syncengine.CapabilityPush | syncengine.CapabilityDelete | syncengine.CapabilityETag)
}

func (a *SyncAdapter) Pull(ctx context.Context, cursor string) (syncengine.PullResult, error) {
	changeSet, err := a.client.ListTodosSince(ctx, a.collection, cursor)
	if err != nil {
		if cursor != "" || !errors.Is(err, ErrIncrementalPullUnsupported) {
			return syncengine.PullResult{}, err
		}
		// Some older servers only implement Depth-1 PROPFIND. It is safe as
		// an initial import, but it deliberately does not claim a cursor; a
		// later run will try REPORT again instead of advancing fake state.
		todos, listErr := a.client.ListTodos(ctx, a.collection)
		if listErr != nil {
			return syncengine.PullResult{}, listErr
		}
		entities, convertErr := entitiesFromTodos(todos)
		if convertErr != nil {
			return syncengine.PullResult{}, convertErr
		}
		return syncengine.PullResult{Entities: entities}, nil
	}
	entities, err := entitiesFromTodos(changeSet.Todos)
	if err != nil {
		return syncengine.PullResult{}, err
	}
	return syncengine.PullResult{
		Entities:     entities,
		DeletedHrefs: changeSet.DeletedHrefs,
		NextCursor:   changeSet.NextToken,
		HasMore:      changeSet.HasMore,
		Incremental:  cursor != "",
	}, nil
}

func entitiesFromTodos(todos []RemoteTodo) ([]syncengine.RemoteEntity, error) {
	entities := make([]syncengine.RemoteEntity, 0, len(todos))
	for _, remote := range todos {
		if remote.Todo == nil || remote.Todo.UID == "" {
			return nil, fmt.Errorf("CalDAV resource %q has no UID", remote.Href)
		}
		task := taskFromTodo(remote.Todo)
		entities = append(entities, syncengine.RemoteEntity{Task: task, RemoteUID: remote.Todo.UID, RemoteHref: remote.Href, ETag: remote.ETag})
	}
	return entities, nil
}

func (a *SyncAdapter) Push(ctx context.Context, entity syncengine.RemoteEntity) (syncengine.PushResult, error) {
	todo := todoFromTask(&entity.Task)
	var remote *RemoteTodo
	var err error
	if entity.RemoteHref != "" {
		remote, err = a.client.UpdateTodo(ctx, entity.RemoteHref, todo, entity.ETag)
	} else {
		remote, err = a.client.CreateTodo(ctx, a.collection, todo)
	}
	if err != nil {
		return syncengine.PushResult{}, err
	}
	return syncengine.PushResult{RemoteUID: todo.UID, RemoteHref: remote.Href, ETag: remote.ETag}, nil
}

// ResumePush recovers the provider result for a completed PUT after the
// caller lost the response before its SQLite transaction committed. This is
// used by the durable sync idempotency store on process restart.
func (a *SyncAdapter) ResumePush(ctx context.Context, entity syncengine.RemoteEntity) (syncengine.PushResult, error) {
	href := entity.RemoteHref
	if href == "" {
		var err error
		href, err = a.client.todoHref(a.collection, entity.Task.UID)
		if err != nil {
			return syncengine.PushResult{}, err
		}
	}
	remote, err := a.client.GetTodo(ctx, href)
	if err != nil {
		return syncengine.PushResult{}, err
	}
	if remote.Todo == nil || remote.Todo.UID != entity.Task.UID {
		return syncengine.PushResult{}, fmt.Errorf("completed CalDAV push returned unexpected UID")
	}
	return syncengine.PushResult{RemoteUID: remote.Todo.UID, RemoteHref: remote.Href, ETag: remote.ETag}, nil
}

func (a *SyncAdapter) Delete(ctx context.Context, entity syncengine.RemoteEntity) error {
	if entity.RemoteHref == "" {
		return fmt.Errorf("cannot delete CalDAV entity without href")
	}
	return a.client.DeleteTodo(ctx, entity.RemoteHref, entity.ETag)
}

var _ syncengine.Adapter = (*SyncAdapter)(nil)
