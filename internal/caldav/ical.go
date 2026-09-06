package caldav

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/chrisbelyea/momentum/internal/models"
	"github.com/chrisbelyea/momentum/pkg/vtodo"
)

func isCalendarRequest(r *http.Request) bool {
	return len(r.Header.Values("Content-Type")) > 0 && (r.Header.Get("Content-Type") == "text/calendar" || bytes.HasPrefix([]byte(r.Header.Get("Content-Type")), []byte("text/calendar;")))
}

func wantsCalendar(r *http.Request) bool {
	return isCalendarRequest(r) || bytes.Contains([]byte(r.Header.Get("Accept")), []byte("text/calendar"))
}

func readBody(r *http.Request) []byte {
	b, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	return b
}

func taskFromTodo(t *vtodo.Todo) models.Task {
	tagsJSON := jsonString(t.Categories)
	relatedJSON := jsonString(t.RelatedTo)
	extraJSON := jsonString(t.Extra)
	return models.Task{UID: t.UID, Title: t.Summary, Description: stringPtr(t.Description), Status: t.Status,
		Priority: t.Priority, DueAt: t.Due, DueDateOnly: t.DueDateOnly, StartAt: t.Start, StartDateOnly: t.StartDateOnly, CompletedAt: t.Completed,
		DTStamp: t.DTStamp, LastModified: t.LastModified, Sequence: t.Sequence,
		PercentComplete: t.PercentComplete, TagsJSON: tagsJSON, RelatedToJSON: relatedJSON,
		ExtraJSON: extraJSON, URL: stringPtr(t.URL), Location: stringPtr(t.Location)}
}

func todoFromTask(t *models.Task) *vtodo.Todo {
	d := ""
	if t.Description != nil {
		d = *t.Description
	}
	u := ""
	if t.URL != nil {
		u = *t.URL
	}
	l := ""
	if t.Location != nil {
		l = *t.Location
	}
	stamp := t.DTStamp
	if stamp.IsZero() {
		stamp = time.Now().UTC()
	}
	return &vtodo.Todo{UID: t.UID, Summary: t.Title, Description: d, Status: t.Status, Due: t.DueAt,
		Start: t.StartAt, StartDateOnly: t.StartDateOnly, Completed: t.CompletedAt, DTStamp: stamp, LastModified: t.LastModified,
		DueDateOnly: t.DueDateOnly,
		Priority:    t.Priority, PercentComplete: t.PercentComplete, Sequence: t.Sequence,
		Categories: decodeStrings(t.TagsJSON), RelatedTo: decodeStrings(t.RelatedToJSON),
		Extra: decodeProperties(t.ExtraJSON), URL: u, Location: l}
}

func jsonString(value any) *string {
	if lenValue, ok := value.([]string); ok && len(lenValue) == 0 {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	s := string(data)
	return &s
}

func decodeStrings(value *string) []string {
	if value == nil {
		return nil
	}
	var result []string
	if json.Unmarshal([]byte(*value), &result) != nil {
		return nil
	}
	return result
}

func decodeProperties(value *string) []vtodo.Property {
	if value == nil {
		return nil
	}
	var result []vtodo.Property
	if json.Unmarshal([]byte(*value), &result) != nil {
		return nil
	}
	return result
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func taskETag(t *models.Task) string {
	data, _ := todoFromTask(t).Marshal()
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func (h *Handler) writeCalendarTask(w http.ResponseWriter, r *http.Request, task *models.Task) {
	etag := taskETag(task)
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag || r.Header.Get("If-None-Match") == "*" {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	data, err := todoFromTask(task).Marshal()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}
