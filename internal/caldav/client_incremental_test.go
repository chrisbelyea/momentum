package caldav

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chrisbelyea/momentum/internal/models"
)

func TestClientListTodosSinceUsesSyncTokenAndReportsDeletion(t *testing.T) {
	var tokens []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("ETag", `"two"`)
			w.Header().Set("Content-Type", "text/calendar")
			_, _ = fmt.Fprint(w, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VTODO\r\nUID:one\r\nDTSTAMP:20260906T120000Z\r\nSUMMARY:Updated\r\nSTATUS:NEEDS-ACTION\r\nEND:VTODO\r\nEND:VCALENDAR\r\n")
			return
		}
		if r.Method != "REPORT" {
			http.Error(w, "REPORT required", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read report body: %v", err)
			return
		}
		text := string(body)
		var token string
		switch {
		case strings.Contains(text, "<d:sync-token></d:sync-token>"):
			token = ""
		case strings.Contains(text, "cursor&amp;v2"):
			token = "cursor&v2"
		default:
			t.Errorf("unexpected sync token request: %s", text)
		}
		tokens = append(tokens, token)
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		if token == "" {
			_, _ = fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:response><d:href>one.ics</d:href><d:propstat><d:prop><d:getetag>"one"</d:getetag><d:getcontenttype>text/calendar; component=VTODO</d:getcontenttype><c:calendar-data>`)
			_, _ = fmt.Fprint(w, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VTODO\r\nUID:one\r\nDTSTAMP:20260906T120000Z\r\nSUMMARY:First\r\nSTATUS:NEEDS-ACTION\r\nEND:VTODO\r\nEND:VCALENDAR\r\n")
			_, _ = fmt.Fprint(w, `</c:calendar-data></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response><d:sync-token>cursor&amp;v2</d:sync-token></d:multistatus>`)
			return
		}
		_, _ = fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:response><d:href>one.ics</d:href><d:propstat><d:prop><d:getetag>"two"</d:getetag><d:getcontenttype>text/calendar; component=VTODO</d:getcontenttype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response><d:response><d:href>gone.ics</d:href><d:status>HTTP/1.1 404 Not Found</d:status></d:response><d:sync-token>cursor-3</d:sync-token></d:multistatus>`)
	}))
	defer server.Close()
	client, err := NewClient(&models.BackendConfig{URL: server.URL + "/dav/tasks/", SkipTLSVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	first, err := client.ListTodosSince(context.Background(), server.URL+"/dav/tasks/", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Todos) != 1 || first.Todos[0].Todo.UID != "one" || first.Todos[0].ETag != `"one"` || first.NextToken != "cursor&v2" {
		t.Fatalf("unexpected initial sync result: %#v", first)
	}
	second, err := client.ListTodosSince(context.Background(), server.URL+"/dav/tasks/", first.NextToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Todos) != 1 || second.Todos[0].Todo.UID != "one" || second.Todos[0].ETag != `"two"` || len(second.DeletedHrefs) != 1 || second.DeletedHrefs[0] != server.URL+"/dav/tasks/gone.ics" {
		t.Fatalf("unexpected incremental sync result: %#v", second)
	}
	if len(tokens) != 2 || tokens[0] != "" || tokens[1] != "cursor&v2" {
		t.Fatalf("sync tokens sent: %#v", tokens)
	}
}

func TestClientListTodosSinceDistinguishesUnsupportedAndInvalidToken(t *testing.T) {
	unsupported := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer unsupported.Close()
	client, err := NewClient(&models.BackendConfig{URL: unsupported.URL + "/dav/tasks/", SkipTLSVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = client.ListTodosSince(context.Background(), unsupported.URL+"/dav/tasks/", "")
	if !errors.Is(err, ErrIncrementalPullUnsupported) {
		t.Fatalf("unsupported report error=%v", err)
	}

	for _, status := range []int{http.StatusForbidden, http.StatusConflict} {
		status := status
		conflict := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		conflictClient, clientErr := NewClient(&models.BackendConfig{URL: conflict.URL + "/dav/tasks/", SkipTLSVerify: true})
		if clientErr != nil {
			conflict.Close()
			t.Fatal(clientErr)
		}
		_, requestErr := conflictClient.ListTodosSince(context.Background(), conflict.URL+"/dav/tasks/", "old-token")
		conflictClient.Close()
		conflict.Close()
		if !errors.Is(requestErr, ErrInvalidSyncToken) {
			t.Fatalf("invalid token status %d error=%v", status, requestErr)
		}
	}
}
