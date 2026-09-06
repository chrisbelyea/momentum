package caldav

import (
	"context"
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
	return syncengine.Capabilities(syncengine.CapabilityPull | syncengine.CapabilityPush | syncengine.CapabilityDelete | syncengine.CapabilityETag)
}

func (a *SyncAdapter) Pull(ctx context.Context, _ string) (syncengine.PullResult, error) {
	todos, err := a.client.ListTodos(ctx, a.collection)
	if err != nil {
		return syncengine.PullResult{}, err
	}
	entities := make([]syncengine.RemoteEntity, 0, len(todos))
	for _, remote := range todos {
		if remote.Todo == nil || remote.Todo.UID == "" {
			return syncengine.PullResult{}, fmt.Errorf("CalDAV resource %q has no UID", remote.Href)
		}
		task := taskFromTodo(remote.Todo)
		entities = append(entities, syncengine.RemoteEntity{Task: task, RemoteUID: remote.Todo.UID, RemoteHref: remote.Href, ETag: remote.ETag})
	}
	return syncengine.PullResult{Entities: entities}, nil
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

func (a *SyncAdapter) Delete(ctx context.Context, entity syncengine.RemoteEntity) error {
	if entity.RemoteHref == "" {
		return fmt.Errorf("cannot delete CalDAV entity without href")
	}
	return a.client.DeleteTodo(ctx, entity.RemoteHref, entity.ETag)
}

var _ syncengine.Adapter = (*SyncAdapter)(nil)
