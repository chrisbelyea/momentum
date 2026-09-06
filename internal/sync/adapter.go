// Package sync defines the provider boundary for synchronization. Adapters
// translate transport-specific records into the canonical models.Task shape;
// orchestration and persistence remain provider-neutral.
package sync

import (
	"context"

	"github.com/chrisbelyea/momentum/internal/models"
)

// Capability describes operations a provider can perform. An adapter must not
// be asked to execute an operation it does not advertise.
type Capability uint32

const (
	CapabilityPull Capability = 1 << iota
	CapabilityIncrementalPull
	CapabilityPush
	CapabilityDelete
	CapabilityETag
	CapabilityRemoteSequence
)

type Capabilities uint32

func (c Capabilities) Has(capability Capability) bool { return uint32(c)&uint32(capability) != 0 }

// RemoteEntity is a provider record plus the opaque identity used for future
// updates and deletes. Task is canonical and never contains transport state.
type RemoteEntity struct {
	Task           models.Task
	RemoteUID      string
	RemoteHref     string
	ETag           string
	RemoteSequence *int
	Deleted        bool
}

type PullResult struct {
	Entities   []RemoteEntity
	NextCursor string
	HasMore    bool
}

type PushResult struct {
	RemoteUID      string
	RemoteHref     string
	ETag           string
	RemoteSequence *int
}

// Adapter is the only transport contract required by the sync engine.
// Implementations should make Pull and Push idempotent using the supplied
// remote identity and conditional metadata where supported.
type Adapter interface {
	Capabilities() Capabilities
	Pull(ctx context.Context, cursor string) (PullResult, error)
	Push(ctx context.Context, entity RemoteEntity) (PushResult, error)
	Delete(ctx context.Context, entity RemoteEntity) error
}
