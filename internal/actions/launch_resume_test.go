package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// isolatedState points nat's state directory at a fresh one for one test.
func isolatedState(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
}

// writeSessionRecord leaves the record the mod would have written for slice
// s5's session, started at started in cwd.
func writeSessionRecord(t *testing.T, cwd string, started time.Time) string {
	t.Helper()
	path, err := agent.SessionRecordPath(agent.SessionName("s5"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	rec := `{"session_id":"9c4e357d","cwd":"` + cwd + `","started_at":"` + started.Format(time.RFC3339) + `"}`
	if err := os.WriteFile(path, []byte(rec), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// launchWithBody launches slice s5, whose brief is body, and answers the
// launcher it was started through.
func launchWithBody(t *testing.T, body ...notion.Block) *fakeLauncher {
	t.Helper()
	l := &fakeLauncher{}
	client := &fakeClient{
		getPage: func(id string) (*notion.Page, error) { return todoPage(id, true), nil },
		blocks: func(id string) ([]notion.Block, error) {
			if id == "s5" {
				return body, nil
			}
			return nil, nil
		},
	}
	_, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{}, client.store(), nil, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceClaimed}, ProjectID: "p1", WorkingDir: t.TempDir()},
		config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if len(l.launches) != 1 {
		t.Fatalf("launches = %+v, want one", l.launches)
	}
	return l
}

// A relaunch whose earlier session left a record resumes that session, with a
// short prompt carrying what the task log recorded since it started; the full
// brief is still written, for a compaction and for the fallback.
func TestLaunchResumesARelaunchedSession(t *testing.T) {
	isolatedState(t)
	writeSessionRecord(t, t.TempDir(), time.Date(2026, 10, 10, 11, 0, 0, 0, time.UTC))

	l := launchWithBody(t,
		block(t, "heading_3", "Handed back"),
		block(t, "paragraph", "At 2026-10-10T10:00:00Z"),
		block(t, "paragraph", "Before the session: not news."),
		block(t, "heading_3", "Sent back"),
		block(t, "paragraph", "At 2026-10-10T12:00:00Z"),
		block(t, "paragraph", "Rename the column."))

	if len(l.resumes) != 1 || l.resumes[0].SessionID != "9c4e357d" {
		t.Fatalf("resumes = %+v, want the recorded session resumed", l.resumes)
	}
	data, err := os.ReadFile(l.resumes[0].PromptFile)
	if err != nil {
		t.Fatal(err)
	}
	prompt := string(data)
	if !strings.Contains(prompt, `You are continuing the slice "Info view"`) ||
		!strings.Contains(prompt, "### Sent back, at 2026-10-10T12:00:00Z\n\nRename the column.") ||
		strings.Contains(prompt, "Before the session") {
		t.Errorf("resume prompt:\n%s", prompt)
	}
	if brief, err := os.ReadFile(l.launches[0].promptFile); err != nil || !strings.Contains(string(brief), "working exactly one slice") {
		t.Errorf("brief file = %q, %v; want the full brief written for the fallback", brief, err)
	}
}

// With no record, or a fresh launch, or a record whose directory is gone, the
// launch is today's fresh one.
func TestLaunchGoesFreshWithoutAResumableRecord(t *testing.T) {
	history := []notion.Block{block(t, "heading_3", "Handed back"), block(t, "paragraph", "Wrote it.")}
	for name, tt := range map[string]struct {
		cwd  func(t *testing.T) string
		body []notion.Block
	}{
		"no record":      {nil, history},
		"no history":     {func(t *testing.T) string { return t.TempDir() }, nil},
		"directory gone": {func(t *testing.T) string { return filepath.Join(t.TempDir(), "gone") }, history},
		"cwd not a dir": {func(t *testing.T) string {
			p := filepath.Join(t.TempDir(), "f")
			_ = os.WriteFile(p, nil, 0o600)
			return p
		}, history},
	} {
		t.Run(name, func(t *testing.T) {
			isolatedState(t)
			if tt.cwd != nil {
				writeSessionRecord(t, tt.cwd(t), time.Now())
			}
			if l := launchWithBody(t, tt.body...); len(l.resumes) != 0 {
				t.Errorf("resumes = %+v, want a fresh launch", l.resumes)
			}
		})
	}
}

// A resume prompt that cannot be written launches fresh rather than not at all.
func TestLaunchGoesFreshWhenTheResumePromptCannotBeWritten(t *testing.T) {
	isolatedState(t)
	writeSessionRecord(t, t.TempDir(), time.Now())
	dir, err := agent.BriefDir()
	if err != nil {
		t.Fatal(err)
	}
	// A directory where the temp file would go makes the write fail.
	if err := os.MkdirAll(filepath.Join(dir, agent.SessionName("s5")+".resume.md.tmp", "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if l := launchWithBody(t, block(t, "heading_3", "Handed back"), block(t, "paragraph", "Wrote it.")); len(l.resumes) != 0 {
		t.Errorf("resumes = %+v, want a fresh launch", l.resumes)
	}
}

// What changed since the session started: every stamped event after it under
// its heading, time and author, then every follow-up decided after it. The
// launch lines and anything older, or unstamped, are left out.
func TestEventsSince(t *testing.T) {
	since := time.Date(2026, 10, 10, 11, 0, 0, 0, time.UTC)
	before, after := since.Add(-time.Hour), since.Add(time.Hour)
	got := eventsSince([]store.TaskEvent{
		{Kind: "summary", Note: "unstamped"},
		{Kind: store.HandedBackKind, Note: "old", At: before},
		{Kind: "relaunched", At: after},
		{Kind: "note", Note: "Mind the cache.", By: "Craig", At: after},
		{Kind: "follow_ups", At: before, FollowUps: []store.TaskFollowUp{
			{Title: "Old", Decision: "dropped", DecidedAt: before},
			{Title: "Pending"},
			{Title: "Queue it", Decision: "queued", DecidedAt: after},
		}},
		{Kind: store.ChecksFailedKind, At: after},
	}, since)
	want := "### Note, at 2026-10-10T12:00:00Z, by Craig\n\nMind the cache.\n\n" +
		"### Checks failed, at 2026-10-10T12:00:00Z\n\n" +
		"### Follow-ups the user decided\n\n- \"Queue it\": queued"
	if got != want {
		t.Errorf("eventsSince =\n%s\nwant\n%s", got, want)
	}
	if got := eventsSince(nil, since); got != "" {
		t.Errorf("eventsSince(nil) = %q, want nothing", got)
	}
}
