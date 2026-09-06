package vtodo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProviderVariantFixtures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		wantUID    string
		wantStatus string
		wantExtra  string
		dateOnly   bool
	}{
		{"provider-google.vcf", "google-task-1@example.com", "IN-PROCESS", "X-GOOGLE-REFERENCE", true},
		{"provider-apple.vcf", "apple-task-1@example.com", "CANCELLED", "X-APPLE-SORT-ORDER", false},
	} {
		raw, err := os.ReadFile(filepath.Join("testdata", tc.name))
		if err != nil {
			t.Fatal(err)
		}
		todo, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if todo.UID != tc.wantUID || todo.Status != tc.wantStatus || todo.StartDateOnly != tc.dateOnly {
			t.Fatalf("%s: unexpected canonical fields: %#v", tc.name, todo)
		}
		found := false
		for _, property := range todo.Extra {
			found = found || property.Name == tc.wantExtra
		}
		if !found {
			t.Fatalf("%s: provider extension %s was dropped", tc.name, tc.wantExtra)
		}
	}
}

func TestMalformedProviderFixtureIsRejected(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "malformed-missing-uid.vcf"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(raw); err == nil {
		t.Fatal("malformed provider fixture was accepted")
	}
}

func validTodo() *Todo {
	stamp := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	due := stamp.Add(24 * time.Hour)
	p := 3
	return &Todo{UID: "task-123@example.test", Summary: "Plan, then\\ship; it", Description: "line one\nline two", Status: "NEEDS-ACTION", DTStamp: stamp, LastModified: &stamp, Due: &due, Priority: &p, Categories: []string{"work", "urgent"}, Sequence: 2, URL: "https://example.test/t/123"}
}

func TestMarshalParseRoundTrip(t *testing.T) {
	original := validTodo()
	data, err := original.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\r\n") || !strings.Contains(string(data), "Plan\\, then\\\\ship\\; it") {
		t.Fatalf("not RFC escaped/folded: %q", data)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.UID != original.UID || parsed.Summary != original.Summary || parsed.Description != original.Description || parsed.Status != original.Status || parsed.Sequence != original.Sequence {
		t.Fatalf("round trip mismatch: %#v", parsed)
	}
	if parsed.Due == nil || !parsed.Due.Equal(*original.Due) || len(parsed.Categories) != 2 {
		t.Fatalf("optional fields lost: %#v", parsed)
	}
}

func TestDateOnlyAndTZID(t *testing.T) {
	input := "BEGIN:VCALENDAR\r\nBEGIN:VTODO\r\nUID:date-1\r\nDTSTAMP:20260905T120000Z\r\nSUMMARY:Date task\r\nDTSTART;VALUE=DATE:20260905\r\nDUE;TZID=America/New_York:20260906T080000\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	todo, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if !todo.StartDateOnly || todo.Start == nil || todo.Start.Format("2006-01-02") != "2026-09-05" {
		t.Fatalf("date-only DTSTART not retained: %#v", todo)
	}
	if todo.Due == nil || todo.Due.Location().String() != "America/New_York" {
		t.Fatalf("TZID not applied: %#v", todo.Due)
	}
}

func TestUnknownPropertiesAndFolding(t *testing.T) {
	long := strings.Repeat("x", 120)
	input := "BEGIN:VTODO\r\nUID:extra\r\nDTSTAMP:20260905T120000Z\r\nSUMMARY:Long\r\nX-PROVIDER:" + long[:60] + "\r\n " + long[60:] + "\r\nEND:VTODO\r\n"
	todo, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(todo.Extra) != 1 || todo.Extra[0].Value != long {
		t.Fatalf("unknown property not preserved")
	}
}

func TestMarshalFoldsLongUTF8Line(t *testing.T) {
	todo := validTodo()
	todo.Description = strings.Repeat("é", 100)
	data, err := todo.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\r\n ") {
		t.Fatal("long property was not folded")
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Description != todo.Description {
		t.Fatal("folding changed UTF-8 text")
	}
}

func TestRejectsInvalidContract(t *testing.T) {
	base := "BEGIN:VTODO\r\nUID:x\r\nDTSTAMP:20260905T120000Z\r\nSUMMARY:x\r\n"
	for _, tc := range []string{"STATUS:BOGUS", "PRIORITY:10", "PERCENT-COMPLETE:101", "SEQUENCE:-1"} {
		if _, err := Parse([]byte(base + tc + "\r\nEND:VTODO\r\n")); err == nil {
			t.Errorf("expected rejection of %s", tc)
		}
	}
}
