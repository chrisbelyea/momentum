package caldav

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chrisbelyea/momentum/internal/models"
	syncengine "github.com/chrisbelyea/momentum/internal/sync"
)

func TestSyncAdapterPullListsVTODOResources(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = fmt.Fprint(w, `<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:response><d:href>/dav/tasks/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/><c:calendar/></d:resourcetype></d:prop></d:propstat></d:response><d:response><d:href>/dav/tasks/one.ics</d:href><d:propstat><d:prop><d:getetag>"one"</d:getetag><d:getcontenttype>text/calendar; component=VTODO</d:getcontenttype></d:prop></d:propstat></d:response></d:multistatus>`)
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/calendar")
			w.Header().Set("ETag", `"one"`)
			_, _ = fmt.Fprint(w, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VTODO\r\nUID:one\r\nDTSTAMP:20260906T120000Z\r\nSUMMARY:Remote\r\nSTATUS:NEEDS-ACTION\r\nEND:VTODO\r\nEND:VCALENDAR\r\n")
			return
		}
		http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
	}))
	defer server.Close()
	client, err := NewClient(&models.BackendConfig{URL: server.URL + "/dav/", SkipTLSVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	adapter, err := NewSyncAdapter(client, server.URL+"/dav/tasks/")
	if err != nil {
		t.Fatal(err)
	}
	pull, err := adapter.Pull(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(pull.Entities) != 1 || pull.Entities[0].RemoteUID != "one" || pull.Entities[0].ETag != `"one"` || pull.Entities[0].Task.Title != "Remote" {
		t.Fatalf("unexpected pull: %#v", pull)
	}
	if !adapter.Capabilities().Has(syncengine.CapabilityPull) || !adapter.Capabilities().Has(syncengine.CapabilityPush) || !adapter.Capabilities().Has(syncengine.CapabilityDelete) {
		t.Fatalf("adapter capabilities incomplete: %v", adapter.Capabilities())
	}
}
