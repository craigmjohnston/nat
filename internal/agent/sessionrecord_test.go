package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/config"
)

// A session's record sits beside its statusline files, named for it.
func TestSessionRecordPath(t *testing.T) {
	dir := isolatedStatusDir(t)
	got, err := SessionRecordPath("nat-b4463d8f")
	if err != nil || got != filepath.Join(dir, "nat-b4463d8f.session.json") {
		t.Errorf("SessionRecordPath = %q, %v; want nat-b4463d8f.session.json in %s", got, err, dir)
	}
}

// With no state directory there is no record to name, read or remove: the
// launch carries none and goes fresh.
func TestSessionRecordWithNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	if got := sessionRecordEnvArgs("nat-1"); got != nil {
		t.Errorf("sessionRecordEnvArgs = %v, want none with no state directory", got)
	}
	if _, ok := ReadSessionRecord("nat-1"); ok {
		t.Error("ReadSessionRecord found a record with no state directory")
	}
	RemoveSessionRecord("nat-1") // nothing to do, and nothing to fail
}

// The record the mod writes reads back, its time as JavaScript's toISOString
// spells it; a record that is missing, unreadable or names no session is none.
func TestReadSessionRecord(t *testing.T) {
	dir := isolatedStatusDir(t)
	path := filepath.Join(dir, "nat-1.session.json")
	if _, ok := ReadSessionRecord("nat-1"); ok {
		t.Error("ReadSessionRecord found a record that is not there")
	}
	for _, bad := range []string{`not json`, `{"session_id":"  ","cwd":"/w"}`} {
		write(t, path, bad)
		if _, ok := ReadSessionRecord("nat-1"); ok {
			t.Errorf("ReadSessionRecord(%s) found a record", bad)
		}
	}
	write(t, path, `{"session_id":"9c4e357d","cwd":"/work","started_at":"2026-10-10T11:22:33.456Z"}`)
	rec, ok := ReadSessionRecord("nat-1")
	want := time.Date(2026, 10, 10, 11, 22, 33, 456e6, time.UTC)
	if !ok || rec.SessionID != "9c4e357d" || rec.Cwd != "/work" || !rec.StartedAt.Equal(want) {
		t.Errorf("ReadSessionRecord = %+v, %v; want the record as written", rec, ok)
	}
}

// Removing a record removes that one file; none there is no failure, and one
// that cannot be removed is logged, never returned.
func TestRemoveSessionRecord(t *testing.T) {
	dir := isolatedStatusDir(t)
	write(t, filepath.Join(dir, "nat-1.session.json"), `{}`)
	write(t, filepath.Join(dir, "nat-1.json"), `{}`)
	RemoveSessionRecord("nat-1")
	RemoveSessionRecord("nat-1")
	if _, err := os.Stat(filepath.Join(dir, "nat-1.session.json")); !os.IsNotExist(err) {
		t.Errorf("record still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "nat-1.json")); err != nil {
		t.Errorf("the statusline payload went with the record: %v", err)
	}
	// A directory with something in it is not removed.
	write(t, filepath.Join(dir, "nat-2.session.json", "x"), `{}`)
	RemoveSessionRecord("nat-2")
	if _, err := os.Stat(filepath.Join(dir, "nat-2.session.json")); err != nil {
		t.Errorf("Stat = %v, want the unremovable path left", err)
	}
}

// A slice's session record is found by the slice's session name.
func TestForgetSliceSession(t *testing.T) {
	dir := isolatedStatusDir(t)
	id := "3b738308-f654-8170-8c99-eccab4463d8f"
	path := filepath.Join(dir, SessionName(id)+".session.json")
	write(t, path, `{}`)
	ForgetSliceSession(id)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("record still there: %v", err)
	}
}

// The sweep of files belonging to no live session passes over a record, however
// old: it is for the relaunch after the session has gone.
func TestReadStatusesNeverSweepsASessionRecord(t *testing.T) {
	dir := isolatedStatusDir(t)
	for _, p := range []string{payloadPath(dir, "nat-dead"), filepath.Join(dir, "nat-dead.session.json")} {
		write(t, p, `{}`)
		age(t, p, 24*time.Hour)
	}
	ReadStatuses(map[string]string{})
	if _, err := os.Stat(payloadPath(dir, "nat-dead")); !os.IsNotExist(err) {
		t.Errorf("stale payload not swept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "nat-dead.session.json")); err != nil {
		t.Errorf("session record swept: %v", err)
	}
}

// Every agent and ad hoc session names its record where -e is taken.
func TestLaunchesCarryTheSessionRecord(t *testing.T) {
	isolatedStatusDir(t)
	path, err := SessionRecordPath("nat-1")
	if err != nil {
		t.Fatal(err)
	}
	want := sessionRecordEnv + "=" + path
	for name, launch := range map[string]func(*Tmux) error{
		"Launch": func(tm *Tmux) error {
			return tm.Launch("nat-1", "/tmp", "/tmp/p.md", "go", "3b73", "", config.AgentModel{})
		},
		"LaunchResumed": func(tm *Tmux) error {
			return tm.LaunchResumed("nat-1", "/tmp", "/tmp/p.md", "go", "3b73", "", Resumption{SessionID: "s", PromptFile: "/tmp/r.md"}, config.AgentModel{})
		},
		"LaunchBare": func(tm *Tmux) error { return tm.LaunchBare("nat-1", "/tmp", "session:p:s", config.AgentModel{}) },
	} {
		r := &fakeRunner{outs: map[string]string{"-V": "tmux 3.5a\n", "new-session": "%7\n"}}
		if err := launch(NewTmuxWithRunner(r)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !slices.Contains(r.calls[1].args, want) {
			t.Errorf("%s args = %v, want %q", name, r.calls[1].args, want)
		}
	}
}

// A resumed launch runs `claude --resume` with the short prompt and, with the
// mod, NAT_BRIEF naming the full brief for a compaction to find; it falls
// back to the fresh launch's own command. Its pane is tagged as any launch's.
func TestLaunchResumed(t *testing.T) {
	isolatedStatusDir(t)
	r := &fakeRunner{outs: map[string]string{"-V": "tmux 3.5a\n", "new-session": "%7\n"}}
	id := "3b738308-f654-8170-8c99-eccab4463d8f"
	err := NewTmuxWithRunner(r).LaunchResumed("nat-b4463d8f", "/Users/craig/Projects/x", "/tmp/prompt.md", "Continue the slice.",
		id, "p1", Resumption{SessionID: "9c4e357d", PromptFile: "/tmp/resume.md"}, config.AgentModel{Model: "opus"})
	if err != nil {
		t.Fatalf("LaunchResumed: %v", err)
	}
	args := r.calls[1].args
	command := args[slices.Index(args, "sh")+2]
	for _, want := range []string{
		`cd '/Users/craig/Projects/x' && { `,
		`NAT_BRIEF='/tmp/prompt.md' claude --resume '9c4e357d' --model 'opus' --settings `,
		`"$(cat '/tmp/resume.md')" || `,
		`NAT_BRIEF='/tmp/prompt.md' claude --model 'opus' --settings `,
		` 'Continue the slice.'; }; }`,
	} {
		if !strings.Contains(command, want) {
			t.Errorf("command = %s\nwant it to hold %s", command, want)
		}
	}
	if !slices.Contains(args, "NAT_SLICE="+id) {
		t.Errorf("args = %v, want the slice named", args)
	}
	if tag := r.calls[2].args; !slices.Equal(tag, []string{"-u", "set-option", "-p", "-t", "%7", "@nat_slice", id}) {
		t.Errorf("tag call = %v", tag)
	}
}

// A resumed launch tmux refuses is the error, as a fresh one's is.
func TestLaunchResumedFails(t *testing.T) {
	isolatedStatusDir(t)
	r := &fakeRunner{errs: map[string]error{"new-session": &ExitError{Stderr: "boom"}}}
	if err := NewTmuxWithRunner(r).LaunchResumed("nat-1", "/tmp", "/tmp/p.md", "go", "3b73", "", Resumption{SessionID: "s"}, config.AgentModel{}); err == nil {
		t.Error("LaunchResumed = nil, want tmux's refusal")
	}
}

// With no mod the fallback is the brief as the positional prompt, and the
// resume sets no NAT_BRIEF: nothing would read it.
func TestResumeCommandWithoutTheMod(t *testing.T) {
	got := resumeCommand("/w", "/b.md", "go", Resumption{SessionID: "id", PromptFile: "/r.md"}, config.AgentModel{}, "", "")
	settings := shellQuote(statuslineSettings(""))
	want := `cd '/w' && { s=$(date +%s); claude --resume 'id' --settings ` + settings + ` "$(cat '/r.md')" || ` +
		`{ [ $(($(date +%s) - s)) -lt 15 ] && claude --settings ` + settings + ` "$(cat '/b.md')"; }; }`
	if got != want {
		t.Errorf("resumeCommand =\n%s\nwant\n%s", got, want)
	}
}

// The command as sh runs it, against a stand-in claude that logs its argv and
// exits as told: a resume Claude Code refuses at once starts the fresh agent;
// one that ran and ended — well, or badly once past the window — does not.
func TestResumeCommandFallsBackOnlyOnARefusal(t *testing.T) {
	for _, tt := range []struct {
		name     string
		exit     string
		window   int
		wantRuns int
	}{
		{"refused at once", "1", 15, 2},
		{"resumed and ended", "0", 15, 1},
		{"failed after the window", "1", 0, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bin, work := t.TempDir(), t.TempDir()
			log := filepath.Join(work, "argv")
			stub := "#!/bin/sh\necho \"$*\" >> " + shellQuote(log) + "\ncase \"$1\" in --resume) exit " + tt.exit + ";; esac\n"
			if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(stub), 0o700); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(work, "r.md"), "resume prompt")
			write(t, filepath.Join(work, "b.md"), "brief")
			old := resumeFailWindow
			resumeFailWindow = tt.window
			t.Cleanup(func() { resumeFailWindow = old })

			command := resumeCommand(work, filepath.Join(work, "b.md"), "go",
				Resumption{SessionID: "id", PromptFile: filepath.Join(work, "r.md")}, config.AgentModel{}, "", "/mod")
			cmd := exec.Command("sh", "-c", command)
			cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin")
			_ = cmd.Run()

			data, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			runs := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(runs) != tt.wantRuns || !strings.HasPrefix(runs[0], "--resume id ") || !strings.HasSuffix(runs[0], " resume prompt") {
				t.Fatalf("claude ran %q, want the resume first and %d run(s)", runs, tt.wantRuns)
			}
			if tt.wantRuns == 2 && (strings.Contains(runs[1], "--resume") || !strings.HasSuffix(runs[1], " go")) {
				t.Errorf("fallback ran %q, want a fresh claude with the opening line", runs[1])
			}
		})
	}
}

// The resume prompt is written beside the session's brief, under a name of its
// own so the brief a compaction re-reads is left alone.
func TestWriteResumePromptFile(t *testing.T) {
	isolatedStatusDir(t)
	brief, err := WritePromptFile("nat-1", "brief")
	if err != nil {
		t.Fatal(err)
	}
	path, err := WriteResumePromptFile("nat-1", "resume")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Dir(brief) || filepath.Base(path) != "nat-1.resume.md" {
		t.Errorf("path = %s, want nat-1.resume.md beside %s", path, brief)
	}
	for p, want := range map[string]string{path: "resume", brief: "brief"} {
		if data, _ := os.ReadFile(p); string(data) != want {
			t.Errorf("%s = %q, want %q", p, data, want)
		}
	}
}
