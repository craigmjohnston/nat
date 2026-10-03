package source

import "context"

// Fake is a Client answering from canned values and recording what it was
// asked, for the tests of every package that drives a task source. Its zero
// value is ready to use: no groups, no details, no errors.
type Fake struct {
	DescribeResult Describe
	Groups         []Group
	Details        map[string]ContainerDetail
	ActionResult   ActionResult

	DescribeErr  error
	SidebarErr   error
	ContainerErr error
	ActionErr    error
	EventErr     error

	Expands      [][]string
	ContainerIDs []string
	Actions      []ActionCall
	Events       []EventCall
}

// ActionCall is one action request a [Fake] was sent.
type ActionCall struct {
	Project Project
	Action  string
	Target  Target
	Input   string
}

// EventCall is one event a [Fake] was sent.
type EventCall struct {
	Project   Project
	Container string
	Task      Task
	Event     string
}

var _ Client = (*Fake)(nil)

// Describe returns DescribeResult, or DescribeErr.
func (f *Fake) Describe(_ context.Context, _ Project) (Describe, error) {
	if f.DescribeErr != nil {
		return Describe{}, f.DescribeErr
	}
	return f.DescribeResult, nil
}

// Sidebar records expand and returns Groups, or SidebarErr.
func (f *Fake) Sidebar(_ context.Context, _ Project, expand []string) ([]Group, error) {
	f.Expands = append(f.Expands, expand)
	if f.SidebarErr != nil {
		return nil, f.SidebarErr
	}
	return f.Groups, nil
}

// Container records id and returns Details[id] — the zero detail where there
// is none — or ContainerErr.
func (f *Fake) Container(_ context.Context, _ Project, id string) (ContainerDetail, error) {
	f.ContainerIDs = append(f.ContainerIDs, id)
	if f.ContainerErr != nil {
		return ContainerDetail{}, f.ContainerErr
	}
	return f.Details[id], nil
}

// Action records the call and returns ActionResult, or ActionErr.
func (f *Fake) Action(_ context.Context, p Project, action string, target Target, input string) (ActionResult, error) {
	f.Actions = append(f.Actions, ActionCall{Project: p, Action: action, Target: target, Input: input})
	if f.ActionErr != nil {
		return ActionResult{}, f.ActionErr
	}
	return f.ActionResult, nil
}

// Event records the call and returns EventErr.
func (f *Fake) Event(_ context.Context, p Project, container string, task Task, event string) error {
	f.Events = append(f.Events, EventCall{Project: p, Container: container, Task: task, Event: event})
	return f.EventErr
}
