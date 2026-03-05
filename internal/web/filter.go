package web

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/chrisbelyea/momentum/internal/models"
)

// TaskFilter holds filter and sort parameters for task lists.
type TaskFilter struct {
	Status string // filter by exact status value; empty means all
	Tag    string // filter by tag (case-insensitive exact match against tags_json array elements); empty means all
	Sort   string // sort field: "due_at", "title", "status", "created_at" (default)
	Order  string // "asc" or "desc" (default: "desc" for created_at, "asc" otherwise)
}

// ParseTaskFilter extracts filter/sort parameters from an HTTP request's query string.
func ParseTaskFilter(r *http.Request) TaskFilter {
	q := r.URL.Query()
	f := TaskFilter{
		Status: q.Get("status"),
		Tag:    q.Get("tag"),
		Sort:   q.Get("sort"),
		Order:  q.Get("order"),
	}
	// Validate sort field; fall back to default
	validSorts := map[string]bool{
		"due_at": true, "title": true, "status": true, "created_at": true,
	}
	if !validSorts[f.Sort] {
		f.Sort = "created_at"
	}
	// Validate order
	if f.Order != "asc" && f.Order != "desc" {
		if f.Sort == "created_at" {
			f.Order = "desc"
		} else {
			f.Order = "asc"
		}
	}
	return f
}

// ApplyFilter filters and sorts a slice of tasks according to the given TaskFilter.
func ApplyFilter(tasks []*models.Task, f TaskFilter) []*models.Task {
	// Filter
	filtered := make([]*models.Task, 0, len(tasks))
	for _, t := range tasks {
		if f.Status != "" && t.Status != f.Status {
			continue
		}
		if f.Tag != "" && !taskHasTag(t, f.Tag) {
			continue
		}
		filtered = append(filtered, t)
	}

	// Sort
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		switch f.Sort {
		case "title":
			ta, tb := strings.ToLower(a.Title), strings.ToLower(b.Title)
			if f.Order == "desc" {
				return tb < ta
			}
			return ta < tb
		case "status":
			if f.Order == "desc" {
				return b.Status < a.Status
			}
			return a.Status < b.Status
		case "due_at":
			// nil due dates sort last regardless of order
			if a.DueAt == nil && b.DueAt == nil {
				return false
			} else if a.DueAt == nil {
				return false // nil always last
			} else if b.DueAt == nil {
				return true // non-nil before nil
			}
			if f.Order == "desc" {
				return b.DueAt.Before(*a.DueAt)
			}
			return a.DueAt.Before(*b.DueAt)
		default: // created_at
			if f.Order == "desc" {
				return b.CreatedAt.Before(a.CreatedAt)
			}
			return a.CreatedAt.Before(b.CreatedAt)
		}
	})

	return filtered
}

// taskHasTag returns true if the task's tags_json array contains the given tag (case-insensitive).
func taskHasTag(t *models.Task, tag string) bool {
	if t.TagsJSON == nil {
		return false
	}
	var tags []string
	if err := json.Unmarshal([]byte(*t.TagsJSON), &tags); err != nil {
		return false
	}
	tagLower := strings.ToLower(tag)
	for _, tg := range tags {
		if strings.ToLower(tg) == tagLower {
			return true
		}
	}
	return false
}
