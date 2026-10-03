package source

import "context"

// Unavailable is a task source that could not be reached at all — its plugin
// uninstalled, say — answering every call with Err. A source project opens
// over one all the same: its plan is nat's own file, so its tasks still read,
// launch and merge, while every plugin read fails the way a broken plugin's
// would and every event is logged as not sent.
type Unavailable struct {
	Err error
}

var _ Client = Unavailable{}

// Describe answers Err.
func (u Unavailable) Describe(context.Context, Project) (Describe, error) { return Describe{}, u.Err }

// Sidebar answers Err.
func (u Unavailable) Sidebar(context.Context, Project, []string) ([]Group, error) { return nil, u.Err }

// Container answers Err.
func (u Unavailable) Container(context.Context, Project, string) (ContainerDetail, error) {
	return ContainerDetail{}, u.Err
}

// Action answers Err.
func (u Unavailable) Action(context.Context, Project, string, Target, string) (ActionResult, error) {
	return ActionResult{}, u.Err
}

// Event answers Err.
func (u Unavailable) Event(context.Context, Project, string, Task, string) error { return u.Err }
