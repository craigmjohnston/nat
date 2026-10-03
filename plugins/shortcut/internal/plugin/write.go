package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/craigmjohnston/nat/internal/source"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/settings"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/shortcut"
)

// action runs one of the plugin's actions. Whatever happens, the project's
// cache is dropped: nat re-reads the sidebar after every action, and that
// read must see what the action did.
func (a *app) action(ctx context.Context) ([]byte, error) {
	defer a.cache.Drop(a.req.Project.ID)
	msg, err := a.runAction(ctx)
	if err != nil {
		return nil, err
	}
	return marshal(source.ActionResult{Message: msg}), nil
}

func (a *app) runAction(ctx context.Context) (string, error) {
	input := strings.TrimSpace(a.req.Input)
	switch a.req.Action {
	case "refresh":
		a.refCache().Drop(a.refScope())
		return "Refreshed from Shortcut", nil
	case "new-segment":
		return a.editSegments(func(p *settings.Project) (string, error) {
			if input == "" {
				return "", errors.New("shortcut: a new segment needs a name")
			}
			p.Segments = append(p.Segments, settings.Segment{ID: p.NewSegmentID(input), Name: input})
			return fmt.Sprintf("Added segment %s, showing every unstarted story — Filter… to narrow it", input), nil
		})
	case "filter":
		if a.req.Target.Group == "" {
			// The section header's: every search the sidebar makes.
			return a.editSegments(func(p *settings.Project) (string, error) {
				f, err := parseFilter(input)
				if err != nil {
					return "", err
				}
				p.Filter = f
				return "Every Shortcut list now shows " + describeFilter(f, "everything"), nil
			})
		}
		fallthrough
	case "rename", "remove":
		return a.editSegments(func(p *settings.Project) (string, error) {
			return segmentAction(p, a.req.Action, a.req.Target.Group, input)
		})
	case "comment":
		id, err := storyID(a.req.Target.Container)
		if err != nil {
			return "", err
		}
		if input == "" {
			return "", errors.New("shortcut: a comment needs some text")
		}
		if err := notFound(a.sc.CreateComment(ctx, id, input), id); err != nil {
			return "", err
		}
		return fmt.Sprintf("Commented on sc-%d", id), nil
	case "assign":
		return a.addMe(ctx, "owner")
	case "follow":
		return a.addMe(ctx, "follower")
	}
	return "", fmt.Errorf("shortcut: unknown action %q", a.req.Action)
}

// editSegments loads the project's settings, applies edit, and saves them.
func (a *app) editSegments(edit func(p *settings.Project) (string, error)) (string, error) {
	f, p, err := a.settings()
	if err != nil {
		return "", err
	}
	msg, err := edit(p)
	if err != nil {
		return "", err
	}
	return msg, f.Save(a.dirs.ConfigFile())
}

// segmentAction renames, re-filters or removes the segment whose group id is
// group.
func segmentAction(p *settings.Project, action, group, input string) (string, error) {
	i := p.Find(group)
	if i < 0 {
		return "", fmt.Errorf("shortcut: %s needs a segment (got %q)", action, group)
	}
	s := &p.Segments[i]
	switch action {
	case "rename":
		if input == "" {
			return "", errors.New("shortcut: a segment needs a name")
		}
		old := s.Name
		s.Name = input
		return fmt.Sprintf("Renamed %s to %s", old, input), nil
	case "filter":
		f, err := parseFilter(input)
		if err != nil {
			return "", err
		}
		s.Filter = f
		return fmt.Sprintf("%s now shows %s", s.Name, describeFilter(f, "every unstarted story")), nil
	}
	name := s.Name
	p.Segments = slices.Delete(p.Segments, i, i+1)
	return "Removed segment " + name, nil
}

// parseFilter reads the filter editor's answer: a JSON object of field id —
// team, project, epic, labels — to the option ids chosen, an empty list (or a
// field left out) being "any". Team, project and epic take one choice each.
func parseFilter(input string) (settings.Filter, error) {
	var chosen map[string][]string
	if json.Unmarshal([]byte(input), &chosen) != nil {
		return settings.Filter{}, errors.New("shortcut: a filter is a JSON object of field ids to lists of choices")
	}
	var f settings.Filter
	for field, ids := range chosen {
		ids = slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return strings.TrimSpace(id) == "" })
		one := func(dst *string) error {
			if len(ids) > 1 {
				return fmt.Errorf("shortcut: a segment's %s is one choice, not %d", field, len(ids))
			}
			if len(ids) == 1 {
				*dst = ids[0]
			}
			return nil
		}
		var err error
		switch field {
		case "team":
			err = one(&f.Team)
		case "project":
			err = one(&f.Project)
		case "epic":
			err = one(&f.Epic)
		case "labels":
			f.Labels = ids
		default:
			err = fmt.Errorf("shortcut: a segment has no filter field %q", field)
		}
		if err != nil {
			return settings.Filter{}, err
		}
	}
	if len(f.Labels) == 0 {
		f.Labels = nil
	}
	return f, nil
}

// describeFilter is a filter as a toast says it: what it narrows to, field by
// field, or none where it narrows nothing.
func describeFilter(f settings.Filter, none string) string {
	var parts []string
	if f.Team != "" {
		parts = append(parts, "team "+f.Team)
	}
	if f.Project != "" {
		parts = append(parts, "project "+f.Project)
	}
	if f.Epic != "" {
		parts = append(parts, "epic "+f.Epic)
	}
	if len(f.Labels) > 0 {
		parts = append(parts, "labels "+strings.Join(f.Labels, ", "))
	}
	if len(parts) == 0 {
		return none
	}
	return strings.Join(parts, "; ")
}

// addMe adds the token's member to the target story's owners or followers.
func (a *app) addMe(ctx context.Context, role string) (string, error) {
	id, err := storyID(a.req.Target.Container)
	if err != nil {
		return "", err
	}
	var me shortcut.MemberInfo
	var st shortcut.Story
	if err := parallel(
		func() (err error) { me, err = a.sc.Me(ctx); return err },
		func() (err error) { st, err = a.story(ctx, id); return err },
	); err != nil {
		return "", err
	}
	ids, u := st.OwnerIDs, shortcut.StoryUpdate{}
	if role == "follower" {
		ids = st.FollowerIDs
	}
	if slices.Contains(ids, me.ID) {
		if role == "owner" {
			return fmt.Sprintf("You already own sc-%d", id), nil
		}
		return fmt.Sprintf("Already following sc-%d", id), nil
	}
	ids = append(slices.Clone(ids), me.ID)
	if role == "owner" {
		u.OwnerIDs = ids
	} else {
		u.FollowerIDs = ids
	}
	if err := a.sc.UpdateStory(ctx, id, u); err != nil {
		return "", err
	}
	if role == "owner" {
		return fmt.Sprintf("Assigned sc-%d to you", id), nil
	}
	return fmt.Sprintf("Following sc-%d", id), nil
}

// notFound words a 404 as a missing story; any other error passes through.
func notFound(err error, id int64) error {
	var se *shortcut.StatusError
	if errors.As(err, &se) && se.Status == 404 {
		return fmt.Errorf("shortcut: no story sc-%d", id)
	}
	return err
}

// event keeps the story in step with what nat just did to one of its tasks.
// Every branch reads the story first and writes only what isn't already so,
// which makes a duplicate a no-op and lets an out-of-order event find the
// story as it really is rather than as the event's order implies.
func (a *app) event(ctx context.Context) ([]byte, error) {
	if err := a.runEvent(ctx); err != nil {
		return nil, err
	}
	return []byte("{}"), nil
}

func (a *app) runEvent(ctx context.Context) error {
	switch a.req.Event {
	case source.EventCreated, source.EventClaimed, source.EventHandedBack,
		source.EventApproved, source.EventMerged, source.EventDeleted:
	default:
		// released leaves the task filed, so the story task stays; an event
		// this build doesn't know is ignored, as the protocol requires.
		return nil
	}
	task := a.req.Task
	if task.ID == "" {
		return errors.New("shortcut: event has no task")
	}
	id, err := storyID(a.req.Container)
	if err != nil {
		return err
	}
	st, err := a.story(ctx, id)
	if err != nil {
		return err
	}
	mine := slices.IndexFunc(st.Tasks, func(t shortcut.Task) bool {
		return strings.HasSuffix(strings.TrimSpace(t.Description), natMark(task.ID))
	})
	switch a.req.Event {
	case source.EventCreated:
		if mine >= 0 {
			return nil
		}
		return a.sc.CreateTask(ctx, id, taskDescription(task), false)
	case source.EventDeleted:
		if mine < 0 {
			return nil
		}
		return a.sc.DeleteTask(ctx, id, st.Tasks[mine].ID)
	case source.EventHandedBack:
		if task.Branch == "" {
			return nil
		}
		text := fmt.Sprintf("Branch `%s` is ready for review.", task.Branch)
		if live := liveComments(st); len(live) > 0 && strings.TrimSpace(live[len(live)-1].Text) == text {
			return nil
		}
		return a.sc.CreateComment(ctx, id, text)
	case source.EventApproved:
		if task.PR == "" || slices.ContainsFunc(liveComments(st), func(c shortcut.Comment) bool {
			return strings.Contains(c.Text, task.PR)
		}) {
			return nil
		}
		return a.sc.CreateComment(ctx, id, "Pull request opened: "+task.PR)
	case source.EventClaimed:
		return a.claim(ctx, st)
	}
	return a.merge(ctx, st, mine, task)
}

// natMark is the suffix that ties a story task to a nat task.
func natMark(taskID string) string { return "(nat:" + taskID + ")" }

func taskDescription(t source.Task) string {
	title := strings.TrimSpace(t.Title)
	if title == "" {
		title = "nat task"
	}
	return title + " " + natMark(t.ID)
}

// liveComments are the story's comments not deleted, oldest first.
func liveComments(st shortcut.Story) []shortcut.Comment {
	cs := slices.DeleteFunc(slices.Clone(st.Comments), func(c shortcut.Comment) bool { return c.Deleted })
	slices.SortStableFunc(cs, func(x, y shortcut.Comment) int { return x.CreatedAt.Compare(y.CreatedAt.Time) })
	return cs
}

// claim moves a story not yet started to its workflow's started state and
// adds the claimant as an owner. A story already started or done is left
// alone: either someone moved it on, or this claim arrived after a later
// event.
func (a *app) claim(ctx context.Context, st shortcut.Story) error {
	_, proj, err := a.settings()
	if err != nil {
		return err
	}
	var r refs
	var me shortcut.MemberInfo
	if err := parallel(
		into(ctx, &r.workflows, a.sc.Workflows),
		func() (err error) { me, err = a.sc.Me(ctx); return err },
	); err != nil {
		return err
	}
	cur, w, ok := r.state(st.WorkflowStateID)
	if !ok || (cur.Type != shortcut.StateUnstarted && cur.Type != shortcut.StateBacklog) {
		return nil
	}
	to, err := pickState(w, shortcut.StateStarted, proj.StartedState)
	if err != nil {
		return err
	}
	u := shortcut.StoryUpdate{WorkflowStateID: &to.ID}
	if !slices.Contains(st.OwnerIDs, me.ID) {
		u.OwnerIDs = append(slices.Clone(st.OwnerIDs), me.ID)
	}
	return a.sc.UpdateStory(ctx, st.ID, u)
}

// merge completes the task's story task — making one, already complete, if
// its created event never landed — and then, if every story task is
// complete, moves the story to its workflow's done state.
func (a *app) merge(ctx context.Context, st shortcut.Story, mine int, task source.Task) error {
	tasks := slices.Clone(st.Tasks)
	switch {
	case mine < 0:
		if err := a.sc.CreateTask(ctx, st.ID, taskDescription(task), true); err != nil {
			return err
		}
	case !tasks[mine].Complete:
		if err := a.sc.CompleteTask(ctx, st.ID, tasks[mine].ID); err != nil {
			return err
		}
		tasks[mine].Complete = true
	}
	if slices.ContainsFunc(tasks, func(t shortcut.Task) bool { return !t.Complete }) {
		return nil
	}
	_, proj, err := a.settings()
	if err != nil {
		return err
	}
	var r refs
	if r.workflows, err = a.sc.Workflows(ctx); err != nil {
		return err
	}
	cur, w, ok := r.state(st.WorkflowStateID)
	if !ok || cur.Type == shortcut.StateDone {
		return nil
	}
	to, err := pickState(w, shortcut.StateDone, proj.DoneState)
	if err != nil {
		return err
	}
	return a.sc.UpdateStory(ctx, st.ID, shortcut.StoryUpdate{WorkflowStateID: &to.ID})
}
