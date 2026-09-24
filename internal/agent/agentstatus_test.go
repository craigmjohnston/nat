package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/config"
)

// TestMain pins HOME and XDG_STATE_HOME to a directory this test binary made
// for itself: every launch now writes its status files under nat's state
// directory, and a test that launches must never write into the real one.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "nat-agent-test-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("HOME", dir)
	_ = os.Setenv("XDG_STATE_HOME", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// isolatedStatusDir points the state directory at a fresh temp dir for one
// test and answers the status directory inside it.
func isolatedStatusDir(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir, err := AgentStatusDir()
	if err != nil {
		t.Fatalf("AgentStatusDir: %v", err)
	}
	return dir
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func age(t *testing.T, path string, by time.Duration) {
	t.Helper()
	old := time.Now().Add(-by)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func TestAgentStatusDirError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	if _, err := AgentStatusDir(); err == nil {
		t.Fatal("AgentStatusDir: want an error with no resolvable home directory")
	}
	if got := prepareStatusSink("nat-x", config.AgentModel{}); got != "" {
		t.Errorf("sink = %q, want none when the state dir cannot be resolved", got)
	}
	if got := ReadStatuses(map[string]string{"a": "nat-a"}); got != nil {
		t.Errorf("ReadStatuses = %v, want nil when the state dir cannot be resolved", got)
	}
}

func TestStatuslineSettings(t *testing.T) {
	if got, want := statuslineSettings(""), `{"theme":"auto"}`; got != want {
		t.Errorf("no sink: %s, want %s", got, want)
	}
	got := statuslineSettings("/s/it's.json")
	var parsed struct {
		Theme      string
		StatusLine struct{ Type, Command string }
	}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("settings %q are not JSON: %v", got, err)
	}
	if parsed.Theme != "auto" || parsed.StatusLine.Type != "command" {
		t.Errorf("parsed = %+v", parsed)
	}
	want := `cat > '/s/it'\''s.json.tmp' && mv '/s/it'\''s.json.tmp' '/s/it'\''s.json'`
	if parsed.StatusLine.Command != want {
		t.Errorf("command = %q, want %q", parsed.StatusLine.Command, want)
	}
	if strings.Contains(got, "\\u0026") {
		t.Errorf("settings %q are HTML-escaped", got)
	}
}

func TestPrepareStatusSink(t *testing.T) {
	dir := isolatedStatusDir(t)
	write(t, payloadPath(dir, "nat-x"), `{"model":{"id":"stale"}}`)

	sink := prepareStatusSink("nat-x", config.AgentModel{Model: "opus", Effort: "high"})
	if sink != payloadPath(dir, "nat-x") {
		t.Fatalf("sink = %q", sink)
	}
	if _, err := os.Stat(sink); !os.IsNotExist(err) {
		t.Errorf("a previous session's payload survived the launch: %v", err)
	}
	st := readStatus(dir, "nat-x")
	if st.Model != "opus" || st.Effort != "high" || st.Context != nil {
		t.Errorf("status = %+v, want the launch's pair and no context", st)
	}
}

func TestPrepareStatusSinkFailures(t *testing.T) {
	dir := isolatedStatusDir(t)
	// The status directory's own place is a file: nothing can be made there.
	write(t, dir, "in the way")
	if got := prepareStatusSink("nat-x", config.AgentModel{}); got != "" {
		t.Errorf("sink = %q, want none when the directory cannot be made", got)
	}

	dir = isolatedStatusDir(t)
	// The launch record's place is a directory: it cannot be written.
	if err := os.MkdirAll(metaPath(dir, "nat-x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := prepareStatusSink("nat-x", config.AgentModel{}); got != "" {
		t.Errorf("sink = %q, want none when the record cannot be written", got)
	}
}

func TestReadStatusPayload(t *testing.T) {
	dir := isolatedStatusDir(t)
	write(t, metaPath(dir, "nat-a"), `{"model":"opus","effort":"low"}`)
	write(t, payloadPath(dir, "nat-a"),
		`{"model":{"id":"claude-sonnet-5","display_name":"Sonnet 5"},"effort":{"level":"high"},"context_window":{"used_percentage":42.5}}`)
	st := readStatus(dir, "nat-a")
	if st.Model != "Sonnet 5" || st.Effort != "high" || st.Context == nil || *st.Context != 42.5 {
		t.Errorf("status = %+v, want the payload to win over the launch record", st)
	}

	// No display name: the ID stands in.
	write(t, payloadPath(dir, "nat-a"), `{"model":{"id":"claude-sonnet-5"}}`)
	if st := readStatus(dir, "nat-a"); st.Model != "claude-sonnet-5" || st.Effort != "low" {
		t.Errorf("status = %+v, want the ID, and effort from the launch record", st)
	}
}

// A context of null (before the first response) and 0 both must stay
// distinguishable: null is absent, 0 is a real reading.
func TestReadStatusContextAbsentNotZero(t *testing.T) {
	dir := isolatedStatusDir(t)
	write(t, payloadPath(dir, "nat-a"), `{"context_window":{"used_percentage":null}}`)
	if st := readStatus(dir, "nat-a"); st.Context != nil {
		t.Errorf("context = %v, want nil for null", *st.Context)
	}
	write(t, payloadPath(dir, "nat-a"), `{"context_window":{"used_percentage":0}}`)
	if st := readStatus(dir, "nat-a"); st.Context == nil || *st.Context != 0 {
		t.Errorf("context = %v, want a real 0", st.Context)
	}
}

func TestReadStatusNothingReadable(t *testing.T) {
	dir := isolatedStatusDir(t)
	if st := readStatus(dir, "nat-a"); st != (AgentStatus{}) {
		t.Errorf("status = %+v, want all absent with no files", st)
	}
	write(t, metaPath(dir, "nat-a"), `not json`)
	write(t, payloadPath(dir, "nat-a"), `{"model":`)
	if st := readStatus(dir, "nat-a"); st != (AgentStatus{}) {
		t.Errorf("status = %+v, want all absent with garbage files", st)
	}
}

func TestReadStatusesSweepsStale(t *testing.T) {
	dir := isolatedStatusDir(t)
	write(t, payloadPath(dir, "nat-live"), `{"effort":{"level":"max"}}`)
	age(t, payloadPath(dir, "nat-live"), time.Hour)
	write(t, metaPath(dir, "nat-live"), `{}`)
	age(t, metaPath(dir, "nat-live"), time.Hour)
	for _, p := range []string{payloadPath(dir, "nat-dead"), metaPath(dir, "nat-dead"), payloadPath(dir, "nat-dead") + ".tmp"} {
		write(t, p, `{}`)
		age(t, p, time.Hour)
	}
	// Too new to sweep: a launch that tmux has not tagged yet.
	write(t, metaPath(dir, "nat-new"), `{}`)

	got := ReadStatuses(map[string]string{"slice-1": "nat-live"})
	if len(got) != 1 || got["nat-live"].Effort != "max" {
		t.Errorf("statuses = %+v", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := "nat-live.json nat-live.launch.json nat-new.launch.json"
	if strings.Join(names, " ") != want {
		t.Errorf("files = %v, want %s", names, want)
	}
}

func TestReadStatusesNoDirectory(t *testing.T) {
	isolatedStatusDir(t)
	got := ReadStatuses(map[string]string{"slice-1": "nat-a"})
	if got["nat-a"] != (AgentStatus{}) {
		t.Errorf("statuses = %+v, want an absent status for a session with no files", got)
	}
}
