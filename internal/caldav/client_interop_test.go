package caldav

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chrisbelyea/momentum/internal/models"
	"github.com/chrisbelyea/momentum/pkg/vtodo"
)

func TestClientDiscoverAndVTODOCRUD(t *testing.T) {
	var requests []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if user, pass, ok := r.BasicAuth(); !ok || user != "alice" || pass != "secret" {
			t.Error("request did not carry configured Basic authentication")
		}
		switch r.Method {
		case "PROPFIND":
			if r.Header.Get("Depth") != "1" {
				t.Errorf("Depth = %q, want 1", r.Header.Get("Depth"))
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
<d:response><d:href>/dav/tasks/</d:href><d:propstat><d:prop><d:displayname>Tasks</d:displayname><d:resourcetype><d:collection/><c:calendar/></d:resourcetype><c:supported-calendar-component-set><c:comp name="VTODO"/></c:supported-calendar-component-set></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
<d:response><d:href>/dav/addressbook/</d:href><d:propstat><d:prop><d:displayname>Contacts</d:displayname><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
</d:multistatus>`)
		case http.MethodPut:
			if got := r.Header.Get("If-None-Match"); r.Header.Get("If-Match") == "" && got != "*" && strings.HasSuffix(r.URL.Path, "new.ics") {
				t.Errorf("create If-None-Match = %q, want *", got)
			}
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/calendar") {
				t.Errorf("PUT Content-Type = %q", got)
			}
			w.Header().Set("ETag", `"v2"`)
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			w.Header().Set("ETag", `"v2"`)
			w.Header().Set("Content-Type", "text/calendar")
			_, _ = w.Write([]byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VTODO\r\nUID:new\r\nDTSTAMP:20260905T120000Z\r\nSUMMARY:Remote task\r\nSTATUS:NEEDS-ACTION\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"))
		case http.MethodDelete:
			if got := r.Header.Get("If-Match"); got != `"v2"` {
				t.Errorf("delete If-Match = %q", got)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	client, err := NewClient(&models.BackendConfig{URL: server.URL + "/dav/", Username: "alice", Password: "secret", SkipTLSVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	collections, err := client.DiscoverCollections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(collections) != 1 || collections[0].DisplayName != "Tasks" || collections[0].Href != server.URL+"/dav/tasks/" {
		t.Fatalf("unexpected collections: %#v", collections)
	}
	if len(collections[0].Components) != 1 || collections[0].Components[0] != "VTODO" {
		t.Fatalf("unexpected components: %#v", collections[0].Components)
	}

	todo := &vtodo.Todo{UID: "new", Summary: "New task", DTStamp: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)}
	created, err := client.CreateTodo(context.Background(), collections[0].Href, todo)
	if err != nil {
		t.Fatal(err)
	}
	if created.Href != server.URL+"/dav/tasks/new.ics" || created.ETag != `"v2"` {
		t.Fatalf("unexpected create result: %#v", created)
	}
	fetched, err := client.GetTodo(context.Background(), created.Href)
	if err != nil || fetched.Todo.Summary != "Remote task" || fetched.ETag != `"v2"` {
		t.Fatalf("unexpected fetched result: %#v, err %v", fetched, err)
	}
	updated, err := client.UpdateTodo(context.Background(), created.Href, todo, `"v2"`)
	if err != nil || updated.ETag != `"v2"` {
		t.Fatalf("unexpected update result: %#v, err %v", updated, err)
	}
	if err := client.DeleteTodo(context.Background(), created.Href, `"v2"`); err != nil {
		t.Fatal(err)
	}
}

func TestClientRejectsCrossOriginRemoteHref(t *testing.T) {
	client, err := NewClient(&models.BackendConfig{URL: "https://caldav.example.com/dav/", Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.GetTodo(context.Background(), "https://attacker.example.com/tasks/x.ics"); err == nil {
		t.Fatal("cross-origin href was accepted")
	}
}
