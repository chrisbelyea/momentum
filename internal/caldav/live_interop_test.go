package caldav

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chrisbelyea/momentum/internal/models"
	"github.com/chrisbelyea/momentum/pkg/vtodo"
)

// TestLiveCalDAVInterop runs the outbound client against a real CalDAV server
// when MOMENTUM_CALDAV_INTEROP_URL is provided. The CI workflow supplies a
// pinned Radicale instance; local developers can point it at a provider or
// self-hosted server without putting credentials in the repository.
func TestLiveCalDAVInterop(t *testing.T) {
	baseURL := os.Getenv("MOMENTUM_CALDAV_INTEROP_URL")
	if baseURL == "" {
		t.Skip("MOMENTUM_CALDAV_INTEROP_URL is not set")
	}
	username := os.Getenv("MOMENTUM_CALDAV_INTEROP_USER")
	password := os.Getenv("MOMENTUM_CALDAV_INTEROP_PASSWORD")
	client, err := NewClient(&models.BackendConfig{
		URL:           baseURL,
		Username:      username,
		Password:      password,
		SkipTLSVerify: os.Getenv("MOMENTUM_CALDAV_INTEROP_SKIP_TLS_VERIFY") == "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	collections, err := client.DiscoverCollections(ctx)
	if err != nil {
		t.Fatalf("discover collections: %v", err)
	}
	var collection Collection
	for _, candidate := range collections {
		for _, component := range candidate.Components {
			if strings.EqualFold(component, "VTODO") {
				collection = candidate
				break
			}
		}
		if collection.Href != "" {
			break
		}
	}
	if collection.Href == "" {
		t.Fatalf("discovery returned no VTODO collection: %#v", collections)
	}

	todo := &vtodo.Todo{
		UID:     "momentum-live-interop@example.invalid",
		Summary: "Momentum live interoperability",
		Status:  "NEEDS-ACTION",
		DTStamp: time.Now().UTC().Truncate(time.Second),
	}
	created, err := client.CreateTodo(ctx, collection.Href, todo)
	if err != nil {
		t.Fatalf("create VTODO: %v", err)
	}
	if created.Href == "" || created.ETag == "" {
		t.Fatalf("create returned incomplete metadata: %#v", created)
	}
	currentETag := created.ETag

	defer func() {
		if err := client.DeleteTodo(context.Background(), created.Href, currentETag); err != nil {
			t.Errorf("cleanup VTODO: %v", err)
		}
	}()

	fetched, err := client.GetTodo(ctx, created.Href)
	if err != nil {
		t.Fatalf("read VTODO: %v", err)
	}
	if fetched.Todo.UID != todo.UID || fetched.Todo.Summary != todo.Summary {
		t.Fatalf("read VTODO = %#v, want UID %q and summary %q", fetched.Todo, todo.UID, todo.Summary)
	}
	if fetched.ETag == "" {
		t.Fatal("read VTODO did not return an ETag")
	}

	todo.Summary = "Momentum live interoperability updated"
	updated, err := client.UpdateTodo(ctx, created.Href, todo, fetched.ETag)
	if err != nil {
		t.Fatalf("update VTODO: %v", err)
	}
	if updated.ETag == "" {
		t.Fatal("update VTODO did not return an ETag")
	}
	currentETag = updated.ETag
	verified, err := client.GetTodo(ctx, created.Href)
	if err != nil {
		t.Fatalf("read updated VTODO: %v", err)
	}
	if verified.Todo.Summary != todo.Summary {
		t.Fatalf("updated summary = %q, want %q", verified.Todo.Summary, todo.Summary)
	}

	listed, err := client.ListTodos(ctx, collection.Href)
	if err != nil {
		t.Fatalf("list VTODO collection: %v", err)
	}
	var found bool
	for _, item := range listed {
		if item.Todo.UID == todo.UID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("list did not include created VTODO: %#v", listed)
	}
}
