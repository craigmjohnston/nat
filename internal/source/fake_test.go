package source

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestFakeAnswersFromItsCannedValues(t *testing.T) {
	ctx := context.Background()
	f := &Fake{
		DescribeResult: Describe{Protocol: 1, Name: "sc"},
		Groups:         []Group{{ID: "g"}},
		Details:        map[string]ContainerDetail{"c1": {ID: "c1", Title: "Card"}},
		ActionResult:   ActionResult{Message: "done"},
	}
	if d, err := f.Describe(ctx, testProject); err != nil || d.Name != "sc" {
		t.Errorf("Describe() = %+v, %v", d, err)
	}
	if g, err := f.Sidebar(ctx, testProject, []string{"done"}); err != nil || !reflect.DeepEqual(g, f.Groups) {
		t.Errorf("Sidebar() = %+v, %v", g, err)
	}
	if d, err := f.Container(ctx, testProject, "c1"); err != nil || d.Title != "Card" {
		t.Errorf("Container() = %+v, %v", d, err)
	}
	if r, err := f.Action(ctx, testProject, "move", Target{Container: "c1"}, "Ready"); err != nil || r.Message != "done" {
		t.Errorf("Action() = %+v, %v", r, err)
	}
	task := Task{ID: "t1"}
	if err := f.Event(ctx, testProject, "c1", task, EventCreated); err != nil {
		t.Errorf("Event() = %v", err)
	}

	if !reflect.DeepEqual(f.Expands, [][]string{{"done"}}) || !reflect.DeepEqual(f.ContainerIDs, []string{"c1"}) {
		t.Errorf("recorded expands %v, containers %v", f.Expands, f.ContainerIDs)
	}
	if want := []ActionCall{{Project: testProject, Action: "move", Target: Target{Container: "c1"}, Input: "Ready"}}; !reflect.DeepEqual(f.Actions, want) {
		t.Errorf("recorded actions %+v, want %+v", f.Actions, want)
	}
	if want := []EventCall{{Project: testProject, Container: "c1", Task: task, Event: EventCreated}}; !reflect.DeepEqual(f.Events, want) {
		t.Errorf("recorded events %+v, want %+v", f.Events, want)
	}
}

func TestFakeReturnsItsErrorsAndStillRecords(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")
	f := &Fake{DescribeErr: boom, SidebarErr: boom, ContainerErr: boom, ActionErr: boom, EventErr: boom}
	if _, err := f.Describe(ctx, testProject); err != boom {
		t.Errorf("Describe() = %v", err)
	}
	if _, err := f.Sidebar(ctx, testProject, nil); err != boom {
		t.Errorf("Sidebar() = %v", err)
	}
	if _, err := f.Container(ctx, testProject, "c1"); err != boom {
		t.Errorf("Container() = %v", err)
	}
	if _, err := f.Action(ctx, testProject, "a", Target{}, ""); err != boom {
		t.Errorf("Action() = %v", err)
	}
	if err := f.Event(ctx, testProject, "c1", Task{}, EventDeleted); err != boom {
		t.Errorf("Event() = %v", err)
	}
	if len(f.Expands) != 1 || len(f.ContainerIDs) != 1 || len(f.Actions) != 1 || len(f.Events) != 1 {
		t.Errorf("recorded %+v, want every call", f)
	}
}

// TestFakeZeroValueAnswersAContainerItHasNoDetailFor: a nil Details map is
// the zero detail, not a panic.
func TestFakeZeroValueAnswersAContainerItHasNoDetailFor(t *testing.T) {
	var f Fake
	if d, err := f.Container(context.Background(), testProject, "c9"); err != nil || !reflect.DeepEqual(d, ContainerDetail{}) {
		t.Errorf("Container() = %+v, %v, want the zero detail", d, err)
	}
}
