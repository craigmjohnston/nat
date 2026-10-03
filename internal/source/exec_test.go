package source

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// fakeRunner records the one call it was given and answers it from canned
// output.
type fakeRunner struct {
	out string
	err error

	dir   string
	name  string
	args  []string
	stdin string
	calls int
}

func (f *fakeRunner) Run(dir, name string, args ...string) (string, error) {
	return f.RunWithStdin(dir, nil, name, args...)
}

func (f *fakeRunner) RunWithStdin(dir string, stdin io.Reader, name string, args ...string) (string, error) {
	f.calls++
	f.dir, f.name, f.args = dir, name, args
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		f.stdin = string(b)
	}
	return f.out, f.err
}

// request decodes what the runner was sent on stdin.
func (f *fakeRunner) request(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(f.stdin), &m); err != nil {
		t.Fatalf("stdin %q is not JSON: %v", f.stdin, err)
	}
	return m
}

var testProject = Project{ID: "p1", Name: "Work", WorkingDir: "/repo"}

// assertSent checks the runner ran the plugin's binary, in the project's
// working directory, with exactly the method as argv, and that stdin carried
// the envelope plus want.
func assertSent(t *testing.T, f *fakeRunner, method string, want map[string]any) {
	t.Helper()
	if f.name != "/bin/nat-source-sc" || f.dir != "/repo" || !reflect.DeepEqual(f.args, []string{method}) {
		t.Errorf("ran %q in %q with %q, want /bin/nat-source-sc in /repo with [%s]", f.name, f.dir, f.args, method)
	}
	want["project"] = map[string]any{"id": "p1", "name": "Work", "working_dir": "/repo"}
	if got := f.request(t); !reflect.DeepEqual(got, want) {
		t.Errorf("sent %v, want %v", got, want)
	}
}

func TestDescribeSendsTheEnvelopeAndDecodes(t *testing.T) {
	f := &fakeRunner{out: `{"protocol":1,"name":"sc","title":"Shortcut","tag":"SC","icon_symbol":"bolt","container_noun":"card","task_noun":"task","menu":[{"id":"refresh","label":"Refresh","input":"none"}]}`}
	d, err := NewWithRunner("sc", "/bin/nat-source-sc", f).Describe(context.Background(), testProject)
	if err != nil {
		t.Fatalf("Describe() = %v", err)
	}
	want := Describe{Protocol: 1, Name: "sc", Title: "Shortcut", Tag: "SC", IconSymbol: "bolt",
		ContainerNoun: "card", TaskNoun: "task", Menu: []Action{{ID: "refresh", Label: "Refresh", Input: InputNone}}}
	if !reflect.DeepEqual(d, want) {
		t.Errorf("Describe() = %+v, want %+v", d, want)
	}
	assertSent(t, f, "describe", map[string]any{})
}

func TestDescribeRefusesAnotherProtocol(t *testing.T) {
	f := &fakeRunner{out: `{"protocol":2,"name":"sc"}`}
	_, err := NewWithRunner("sc", "/bin/nat-source-sc", f).Describe(context.Background(), testProject)
	if err == nil || err.Error() != "source plugin sc speaks protocol 2; this nat speaks protocol 1" {
		t.Errorf("Describe() = %v, want a protocol refusal", err)
	}
}

func TestDescribeReportsAFailedCall(t *testing.T) {
	f := &fakeRunner{err: &ExitError{Code: 1, Stderr: "no token\n"}}
	if _, err := NewWithRunner("sc", "/bin/nat-source-sc", f).Describe(context.Background(), testProject); err == nil {
		t.Error("Describe() = nil, want the plugin's refusal")
	}
}

func TestSidebarSendsExpandAndDecodesGroups(t *testing.T) {
	f := &fakeRunner{out: `{"groups":[{"id":"done","label":"Done","count":3,"lazy":true,"containers":[{"id":"c1","title":"Fix it","badges":[{"text":"WEB","color":"#f00"}]}]}]}`}
	groups, err := NewWithRunner("sc", "/bin/nat-source-sc", f).Sidebar(context.Background(), testProject, []string{"done"})
	if err != nil {
		t.Fatalf("Sidebar() = %v", err)
	}
	three := 3
	want := []Group{{ID: "done", Label: "Done", Count: &three, Lazy: true,
		Containers: []Container{{ID: "c1", Title: "Fix it", Badges: []Badge{{Text: "WEB", Color: "#f00"}}}}}}
	if !reflect.DeepEqual(groups, want) {
		t.Errorf("Sidebar() = %+v, want %+v", groups, want)
	}
	assertSent(t, f, "sidebar", map[string]any{"expand": []any{"done"}})
}

// TestSidebarSendsAnEmptyExpandForNil: a plugin reading `expand` should find
// a list, never null.
func TestSidebarSendsAnEmptyExpandForNil(t *testing.T) {
	f := &fakeRunner{out: `{"groups":[]}`}
	if _, err := NewWithRunner("sc", "/bin/nat-source-sc", f).Sidebar(context.Background(), testProject, nil); err != nil {
		t.Fatalf("Sidebar() = %v", err)
	}
	assertSent(t, f, "sidebar", map[string]any{"expand": []any{}})
}

func TestSidebarReportsAFailedCall(t *testing.T) {
	f := &fakeRunner{err: &ExitError{Code: 2, Stderr: "rate limited\nretry later\n"}}
	_, err := NewWithRunner("sc", "/bin/nat-source-sc", f).Sidebar(context.Background(), testProject, nil)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("Sidebar() = %v, want an *ExitError", err)
	}
	if err.Error() != "nat-source-sc sidebar: rate limited" {
		t.Errorf("Sidebar() = %q, want the first stderr line under the plugin and method", err)
	}
}

func TestContainerSendsTheIDAndDecodes(t *testing.T) {
	f := &fakeRunner{out: `{"id":"c1","title":"Fix it","external_url":"https://x/c1","facts":[{"label":"state","value":"Doing","color":"blue"}],"sections":[{"id":"body","title":"Story","kind":"prose","body":"words"},{"id":"talk","title":"Comments","kind":"comments","comments":[{"by":"a","when":"now","text":"hi"}],"composer":{"id":"comment","label":"Comment","input":"text"}},{"id":"links","title":"Links","kind":"links","links":[{"label":"PR","text":"#4","state":"open","url":"https://x/4"}]}],"task_note":"Linked."}`}
	d, err := NewWithRunner("sc", "/bin/nat-source-sc", f).Container(context.Background(), testProject, "c1")
	if err != nil {
		t.Fatalf("Container() = %v", err)
	}
	want := ContainerDetail{ID: "c1", Title: "Fix it", ExternalURL: "https://x/c1",
		Facts: []Fact{{Label: "state", Value: "Doing", Color: "blue"}},
		Sections: []Section{
			{ID: "body", Title: "Story", Kind: KindProse, Body: "words"},
			{ID: "talk", Title: "Comments", Kind: KindComments, Comments: []Comment{{By: "a", When: "now", Text: "hi"}},
				Composer: &Action{ID: "comment", Label: "Comment", Input: InputText}},
			{ID: "links", Title: "Links", Kind: KindLinks, Links: []Link{{Label: "PR", Text: "#4", State: "open", URL: "https://x/4"}}},
		},
		TaskNote: "Linked."}
	if !reflect.DeepEqual(d, want) {
		t.Errorf("Container() = %+v, want %+v", d, want)
	}
	assertSent(t, f, "container", map[string]any{"id": "c1"})
}

func TestContainerReportsAFailedCall(t *testing.T) {
	f := &fakeRunner{err: errors.New("no such file")}
	if _, err := NewWithRunner("sc", "/bin/nat-source-sc", f).Container(context.Background(), testProject, "c1"); err == nil {
		t.Error("Container() = nil, want the runner's error")
	}
}

func TestActionSendsTheActionTargetAndInput(t *testing.T) {
	f := &fakeRunner{out: `{"message":"Moved."}`}
	r, err := NewWithRunner("sc", "/bin/nat-source-sc", f).Action(context.Background(), testProject, "move",
		Target{Container: "c1"}, "Ready")
	if err != nil {
		t.Fatalf("Action() = %v", err)
	}
	if r != (ActionResult{Message: "Moved."}) {
		t.Errorf("Action() = %+v, want the plugin's message", r)
	}
	assertSent(t, f, "action", map[string]any{"action": "move", "target": map[string]any{"container": "c1"}, "input": "Ready"})
}

// TestActionLeavesOutAnEmptyInputAndTarget: the spec's input and target
// fields are optional, and an action with neither sends neither.
func TestActionLeavesOutAnEmptyInputAndTarget(t *testing.T) {
	f := &fakeRunner{out: `{}`}
	if _, err := NewWithRunner("sc", "/bin/nat-source-sc", f).Action(context.Background(), testProject, "refresh", Target{}, ""); err != nil {
		t.Fatalf("Action() = %v", err)
	}
	assertSent(t, f, "action", map[string]any{"action": "refresh", "target": map[string]any{}})
}

func TestActionReportsAFailedCall(t *testing.T) {
	f := &fakeRunner{err: &ExitError{Code: 1}}
	_, err := NewWithRunner("sc", "/bin/nat-source-sc", f).Action(context.Background(), testProject, "move", Target{Group: "g"}, "")
	if err == nil || err.Error() != "nat-source-sc action: plugin exited 1" {
		t.Errorf("Action() = %v, want the exit code", err)
	}
}

func TestEventSendsTheTaskAndIgnoresStdout(t *testing.T) {
	f := &fakeRunner{out: "not json at all"}
	task := Task{ID: "t1", Title: "Do it", Status: "In progress", Branch: "slice/do-it", PR: "https://x/4"}
	if err := NewWithRunner("sc", "/bin/nat-source-sc", f).Event(context.Background(), testProject, "c1", task, EventHandedBack); err != nil {
		t.Fatalf("Event() = %v", err)
	}
	assertSent(t, f, "event", map[string]any{"container": "c1", "event": "handed_back",
		"task": map[string]any{"id": "t1", "title": "Do it", "status": "In progress", "branch": "slice/do-it", "pr": "https://x/4"}})
}

func TestEventReportsAFailedCall(t *testing.T) {
	f := &fakeRunner{err: &ExitError{Code: 3, Stderr: "down"}}
	if err := NewWithRunner("sc", "/bin/nat-source-sc", f).Event(context.Background(), testProject, "c1", Task{}, EventMerged); err == nil {
		t.Error("Event() = nil, want the plugin's refusal")
	}
}

func TestSetupSendsTheValueOnStdinAndDecodesTheMessage(t *testing.T) {
	f := &fakeRunner{out: `{"message":"Logged in to scratch as Craig"}`}
	msg, err := NewWithRunner("sc", "/bin/nat-source-sc", f).Setup(context.Background(), "token", "s3cret-value")
	if err != nil || msg != "Logged in to scratch as Craig" {
		t.Fatalf("Setup() = %q, %v", msg, err)
	}
	if f.name != "/bin/nat-source-sc" || f.dir != "" || !reflect.DeepEqual(f.args, []string{"setup"}) {
		t.Errorf("ran %q in %q with %q, want the binary with [setup] alone", f.name, f.dir, f.args)
	}
	want := map[string]any{"project": map[string]any{"id": "", "name": "", "working_dir": ""}, "id": "token", "input": "s3cret-value"}
	if got := f.request(t); !reflect.DeepEqual(got, want) {
		t.Errorf("sent %v, want %v", got, want)
	}
}

// TestSetupNeverLogsTheInput: a refused setup is logged by plugin, method and
// id, and the value — a credential — reaches neither the log nor the error.
func TestSetupNeverLogsTheInput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", home)
	path, err := logging.Open()
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeRunner{err: &ExitError{Code: 1, Stderr: "shortcut: Shortcut refused the token\n"}}
	_, err = NewWithRunner("sc", "/bin/nat-source-sc", f).Setup(context.Background(), "token", "s3cret-value")
	if cerr := logging.Close(); cerr != nil {
		t.Fatal(cerr)
	}
	if err == nil || err.Error() != "nat-source-sc setup: shortcut: Shortcut refused the token" {
		t.Errorf("Setup() = %v, want the plugin's line", err)
	}
	b, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	log := string(b)
	if strings.Contains(log, "s3cret") || !strings.Contains(log, "method=setup") || !strings.Contains(log, "id=token") {
		t.Errorf("log = %q, want method and id, never the input", log)
	}
}

func TestSetupMalformedResponse(t *testing.T) {
	_, err := NewWithRunner("sc", "/bin/nat-source-sc", &fakeRunner{out: "s3cret"}).Setup(context.Background(), "token", "s3cret")
	if err == nil || err.Error() != "nat-source-sc setup: malformed response" {
		t.Errorf("Setup() = %v, want a malformed-response error", err)
	}
}

// TestMalformedResponsesNameTheCallNotTheBody covers every decoding method:
// the error says which plugin and method, and never quotes what came back.
func TestMalformedResponsesNameTheCallNotTheBody(t *testing.T) {
	const body = `{"secret ticket text": 5`
	ctx := context.Background()
	for method, call := range map[string]func(*Exec) error{
		"describe": func(e *Exec) error { _, err := e.Describe(ctx, testProject); return err },
		"sidebar":  func(e *Exec) error { _, err := e.Sidebar(ctx, testProject, nil); return err },
		"container": func(e *Exec) error {
			_, err := e.Container(ctx, testProject, "c1")
			return err
		},
		"action": func(e *Exec) error { _, err := e.Action(ctx, testProject, "a", Target{}, ""); return err },
	} {
		err := call(NewWithRunner("sc", "/bin/nat-source-sc", &fakeRunner{out: body}))
		if err == nil || err.Error() != "nat-source-sc "+method+": malformed response" {
			t.Errorf("%s: err = %v, want a malformed-response error", method, err)
		}
		if err != nil && strings.Contains(err.Error(), "secret") {
			t.Errorf("%s: err %q quotes the body", method, err)
		}
	}
}

func TestACancelledContextRunsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &fakeRunner{}
	if _, err := NewWithRunner("sc", "/bin/nat-source-sc", f).Describe(ctx, testProject); !errors.Is(err, context.Canceled) {
		t.Errorf("Describe() = %v, want context.Canceled", err)
	}
	if f.calls != 0 {
		t.Errorf("ran the plugin %d times, want none", f.calls)
	}
}

func TestNewRunsRealSubprocessesOnBothTimeouts(t *testing.T) {
	e := New("sc", "/bin/nat-source-sc")
	if e.Name != "sc" || e.Path != "/bin/nat-source-sc" {
		t.Errorf("New() = %+v", e)
	}
	if e.runner != (ExecRunner{Timeout: callTimeout}) || e.eventRunner != (ExecRunner{Timeout: eventTimeout}) {
		t.Errorf("New() runners = %+v, %+v, want the call and event timeouts", e.runner, e.eventRunner)
	}
}

func TestExitErrorFirstLine(t *testing.T) {
	if got := (&ExitError{Code: 1, Stderr: "\n  \n  bad token  \nusage\n"}).Error(); got != "bad token" {
		t.Errorf("Error() = %q, want the first non-empty line", got)
	}
	if got := (&ExitError{Code: 4, Stderr: " \n"}).Error(); got != "plugin exited 4" {
		t.Errorf("Error() = %q, want the exit code", got)
	}
}

// script writes an executable shell script into a temp dir and returns it.
func script(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nat-source-x")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExecRunnerCarriesStdinAndArgs(t *testing.T) {
	path := script(t, `echo "$1"; cat`)
	out, err := ExecRunner{}.RunWithStdin(t.TempDir(), strings.NewReader("hello"), path, "sidebar")
	if err != nil || out != "sidebar\nhello" {
		t.Errorf("RunWithStdin() = %q, %v, want the method then stdin echoed", out, err)
	}
	out, err = ExecRunner{}.Run(t.TempDir(), path, "describe")
	if err != nil || out != "describe\n" {
		t.Errorf("Run() = %q, %v, want the method echoed", out, err)
	}
}

func TestExecRunnerReportsANonZeroExit(t *testing.T) {
	path := script(t, `echo partial; echo "no token" >&2; exit 3`)
	out, err := ExecRunner{Timeout: time.Minute}.Run(t.TempDir(), path)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 3 || exitErr.Error() != "no token" {
		t.Fatalf("Run() = %v, want an *ExitError of 3 saying no token", err)
	}
	if out != "partial\n" {
		t.Errorf("Run() out = %q, want what it printed", out)
	}
}

func TestExecRunnerReportsAMissingBinary(t *testing.T) {
	_, err := ExecRunner{}.Run(t.TempDir(), filepath.Join(t.TempDir(), "absent"))
	var exitErr *ExitError
	if err == nil || errors.As(err, &exitErr) {
		t.Errorf("Run() = %v, want os/exec's own error", err)
	}
}

func TestExecRunnerTimesOut(t *testing.T) {
	old := callTimeout
	callTimeout = 50 * time.Millisecond
	t.Cleanup(func() { callTimeout = old })
	path := script(t, `exec sleep 5`)
	start := time.Now()
	_, err := ExecRunner{}.Run(t.TempDir(), path)
	if err == nil || !strings.Contains(err.Error(), "timed out after 50ms") {
		t.Errorf("Run() = %v, want a timeout", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("Run() took %s, want it cut short", time.Since(start))
	}
}
