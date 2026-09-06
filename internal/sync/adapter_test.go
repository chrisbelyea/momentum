package sync

import (
	"context"
	"testing"

	"github.com/chrisbelyea/momentum/internal/models"
)

type contractAdapter struct{}

func (contractAdapter) Capabilities() Capabilities {
	return Capabilities(CapabilityPull | CapabilityIncrementalPull | CapabilityPush | CapabilityDelete | CapabilityETag)
}
func (contractAdapter) Pull(context.Context, string) (PullResult, error) { return PullResult{}, nil }
func (contractAdapter) Push(context.Context, RemoteEntity) (PushResult, error) {
	return PushResult{}, nil
}
func (contractAdapter) Delete(context.Context, RemoteEntity) error { return nil }

func TestAdapterContractSeparatesCanonicalTaskAndTransportIdentity(t *testing.T) {
	var adapter Adapter = contractAdapter{}
	if !adapter.Capabilities().Has(CapabilityIncrementalPull) || !adapter.Capabilities().Has(CapabilityETag) {
		t.Fatal("adapter capability contract lost advertised features")
	}
	entity := RemoteEntity{Task: models.Task{UID: "canonical-uid", Title: "Task"}, RemoteUID: "provider-uid", RemoteHref: "/calendar/provider-uid", ETag: "etag-1"}
	if entity.Task.UID == entity.RemoteUID {
		t.Fatal("canonical and provider identities must remain distinct")
	}
}
