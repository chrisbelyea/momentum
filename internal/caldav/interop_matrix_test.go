package caldav

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chrisbelyea/momentum/internal/models"
	"github.com/chrisbelyea/momentum/pkg/vtodo"
)

// interopProfile describes an observed wire-level compatibility boundary that
// is useful to exercise locally. The profiles are deliberately not named after
// a vendor: passing these fixtures proves protocol behavior, not certification
// against a hosted provider.
type interopProfile struct {
	name              string
	absRootHrefs      bool
	createStatus      int
	updateStatus      int
	putReturnsBody    bool
	responseMediaType string
}

type interopFixtureState struct {
	mu           sync.Mutex
	todo         []byte
	resourcePath string
	etag         string
	putCount     int
	requestCount int
	failures     []string
}

func (s *interopFixtureState) failure(format string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, fmt.Sprintf(format, args...))
}

// TestExternalVTODOInteropMatrix keeps the release-relevant outbound lifecycle
// executable without requiring network access or credentials. The first
// profile models a strict standards server (relative hrefs, explicit
// preconditions, and a representation in PUT responses). The second models a
// common hosted-server shape (absolute same-origin hrefs, media-type
// parameters, and an empty successful PUT response).
func TestExternalVTODOInteropMatrix(t *testing.T) {
	profiles := []interopProfile{
		{
			name:              "strict-rfc-server",
			createStatus:      http.StatusCreated,
			updateStatus:      http.StatusOK,
			putReturnsBody:    true,
			responseMediaType: "text/calendar; component=VTODO",
		},
		{
			name:              "hosted-provider-shaped-server",
			absRootHrefs:      true,
			createStatus:      http.StatusCreated,
			updateStatus:      http.StatusNoContent,
			putReturnsBody:    false,
			responseMediaType: "text/calendar; component=VTODO; charset=utf-8",
		},
	}

	for _, profile := range profiles {
		profile := profile
		t.Run(profile.name, func(t *testing.T) {
			state := &interopFixtureState{}
			server := httptest.NewTLSServer(newInteropFixtureHandler(t, profile, state))
			defer server.Close()

			client, err := NewClient(&models.BackendConfig{
				URL:           server.URL + "/dav/",
				Username:      "alice",
				Password:      "secret",
				SkipTLSVerify: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()

			ctx := context.Background()
			collections, err := client.DiscoverCollections(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(collections) != 1 || collections[0].DisplayName != "Tasks" || len(collections[0].Components) != 1 || collections[0].Components[0] != "VTODO" {
				t.Fatalf("unexpected discovery result: %#v", collections)
			}

			todo := &vtodo.Todo{UID: "matrix-task@example.test", Summary: "Interop create", Status: "NEEDS-ACTION", DTStamp: time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)}
			created, err := client.CreateTodo(ctx, collections[0].Href, todo)
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			if created.ETag != `"v1"` || created.Href == "" {
				t.Fatalf("create metadata: %#v", created)
			}

			listed, err := client.ListTodos(ctx, collections[0].Href)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(listed) != 1 || listed[0].Todo.UID != todo.UID || listed[0].Todo.Summary != todo.Summary || listed[0].ETag != `"v1"` {
				t.Fatalf("list result: %#v", listed)
			}

			fetched, err := client.GetTodo(ctx, created.Href)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if fetched.Todo.UID != todo.UID || fetched.ETag != `"v1"` {
				t.Fatalf("get result: %#v", fetched)
			}

			todo.Summary = "Interop update"
			updated, err := client.UpdateTodo(ctx, created.Href, todo, fetched.ETag)
			if err != nil {
				t.Fatalf("update: %v", err)
			}
			if updated.ETag != `"v2"` {
				t.Fatalf("update metadata: %#v", updated)
			}

			if err := client.DeleteTodo(ctx, created.Href, updated.ETag); err != nil {
				t.Fatalf("delete: %v", err)
			}

			state.mu.Lock()
			failures := append([]string(nil), state.failures...)
			putCount, requestCount := state.putCount, state.requestCount
			state.mu.Unlock()
			if len(failures) > 0 {
				t.Fatalf("fixture protocol failures: %s", strings.Join(failures, "; "))
			}
			if putCount != 2 || requestCount < 7 {
				t.Fatalf("unexpected lifecycle request counts: PUT=%d total=%d", putCount, requestCount)
			}
		})
	}
}

func newInteropFixtureHandler(t *testing.T, profile interopProfile, state *interopFixtureState) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		state.requestCount++
		state.mu.Unlock()

		if user, password, ok := r.BasicAuth(); !ok || user != "alice" || password != "secret" {
			state.failure("%s %s missing configured Basic authentication", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case "PROPFIND":
			handleInteropPropfind(w, r, profile, state)
		case http.MethodPut:
			handleInteropPut(w, r, profile, state)
		case http.MethodGet:
			handleInteropGet(w, r, profile, state)
		case http.MethodDelete:
			handleInteropDelete(w, r, state)
		default:
			state.failure("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func handleInteropPropfind(w http.ResponseWriter, r *http.Request, profile interopProfile, state *interopFixtureState) {
	if r.Header.Get("Depth") != "1" {
		state.failure("PROPFIND Depth = %q, want 1", r.Header.Get("Depth"))
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/xml") {
		state.failure("PROPFIND Content-Type = %q", r.Header.Get("Content-Type"))
	}

	if r.URL.Path == "/dav/" {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:response><d:href>%s</d:href><d:propstat><d:prop><d:displayname>Tasks</d:displayname><d:resourcetype><d:collection/><c:calendar/></d:resourcetype><c:supported-calendar-component-set><c:comp name="VTODO"/></c:supported-calendar-component-set></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`, interopHref(r, "/dav/tasks/", profile, true))
		return
	}

	state.mu.Lock()
	resourcePath, etag := state.resourcePath, state.etag
	state.mu.Unlock()
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:response><d:href>.</d:href><d:propstat><d:prop><d:resourcetype><d:collection/><c:calendar/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
	if resourcePath != "" {
		_, _ = fmt.Fprintf(w, `<d:response><d:href>%s</d:href><d:propstat><d:prop><d:getetag>%s</d:getetag><d:getcontenttype>%s</d:getcontenttype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`, interopHref(r, resourcePath, profile, false), etag, profile.responseMediaType)
	}
	_, _ = io.WriteString(w, `</d:multistatus>`)
}

func handleInteropPut(w http.ResponseWriter, r *http.Request, profile interopProfile, state *interopFixtureState) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "text/calendar") {
		state.failure("PUT Content-Type = %q", r.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxCalDAVResponseBytes))
	if err != nil {
		state.failure("read PUT: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if _, err := vtodo.Parse(body); err != nil {
		state.failure("PUT body is not a VTODO: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	state.putCount++
	if state.resourcePath == "" {
		if r.Header.Get("If-None-Match") != "*" {
			state.failure("create If-None-Match = %q, want *", r.Header.Get("If-None-Match"))
		}
		state.resourcePath = r.URL.Path
		state.etag = `"v1"`
	} else {
		if r.URL.Path != state.resourcePath {
			state.failure("update path = %q, want %q", r.URL.Path, state.resourcePath)
		}
		if r.Header.Get("If-Match") != state.etag {
			state.failure("update If-Match = %q, want %q", r.Header.Get("If-Match"), state.etag)
		}
		state.etag = `"v2"`
	}
	state.todo = append(state.todo[:0], body...)
	w.Header().Set("ETag", state.etag)
	w.Header().Set("Content-Type", profile.responseMediaType)
	status := profile.createStatus
	if state.putCount > 1 {
		status = profile.updateStatus
	}
	w.WriteHeader(status)
	if profile.putReturnsBody {
		_, _ = w.Write(body)
	}
}

func handleInteropGet(w http.ResponseWriter, r *http.Request, profile interopProfile, state *interopFixtureState) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if r.URL.Path != state.resourcePath || len(state.todo) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("ETag", state.etag)
	w.Header().Set("Content-Type", profile.responseMediaType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(state.todo)
}

func handleInteropDelete(w http.ResponseWriter, r *http.Request, state *interopFixtureState) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if r.URL.Path != state.resourcePath || len(state.todo) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Header.Get("If-Match") != state.etag {
		state.failure("delete If-Match = %q, want %q", r.Header.Get("If-Match"), state.etag)
	}
	state.todo = nil
	w.WriteHeader(http.StatusNoContent)
}

func interopHref(r *http.Request, path string, profile interopProfile, root bool) string {
	if profile.absRootHrefs {
		return "https://" + r.Host + path
	}
	if root {
		return "tasks/"
	}
	return strings.TrimPrefix(path, "/dav/tasks/")
}
