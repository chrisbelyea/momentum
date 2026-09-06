package caldav

import (
	"testing"

	"github.com/chrisbelyea/momentum/internal/models"
	"github.com/chrisbelyea/momentum/pkg/vtodo"
)

func TestTodoTaskConversionPreservesProviderFields(t *testing.T) {
	todo := &vtodo.Todo{
		UID: "uid-1", Summary: "Task", Status: models.StatusNeedsAction,
		Categories: []string{"work", "urgent"}, RelatedTo: []string{"parent-1"},
		Extra: []vtodo.Property{{Name: "X-PROVIDER-STATE", Value: "open"}},
	}
	task := taskFromTodo(todo)
	if task.TagsJSON == nil || task.RelatedToJSON == nil || task.ExtraJSON == nil {
		t.Fatal("provider fields were not persisted")
	}
	roundTrip := todoFromTask(&task)
	if len(roundTrip.Categories) != 2 || roundTrip.Categories[1] != "urgent" {
		t.Fatalf("categories lost: %#v", roundTrip.Categories)
	}
	if len(roundTrip.RelatedTo) != 1 || roundTrip.RelatedTo[0] != "parent-1" {
		t.Fatalf("relationships lost: %#v", roundTrip.RelatedTo)
	}
	if len(roundTrip.Extra) != 1 || roundTrip.Extra[0].Name != "X-PROVIDER-STATE" {
		t.Fatalf("extensions lost: %#v", roundTrip.Extra)
	}
}

func TestTodoTaskConversionPreservesDateOnlySemantics(t *testing.T) {
	todo := &vtodo.Todo{UID: "date-only", Summary: "All day", Status: models.StatusNeedsAction,
		DueDateOnly: true, StartDateOnly: true}
	task := taskFromTodo(todo)
	if !task.DueDateOnly || !task.StartDateOnly {
		t.Fatal("date-only flags were not copied into the canonical task")
	}
	roundTrip := todoFromTask(&task)
	if !roundTrip.DueDateOnly || !roundTrip.StartDateOnly {
		t.Fatal("date-only flags were not restored for VTODO serialization")
	}
}
