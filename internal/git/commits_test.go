package git

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// commitLog is a git log answer in [commitLogFormat]'s own shape: three
// commits, NUL-separated fields, newest first — the order git log already
// writes them in.
const commitLog = "aaa111\x00Add the viewer\x00Craig Johnston\x002026-08-28T11:30:00+02:00\n" +
	"bbb222\x00Wire the diff screen\x00Craig Johnston\x002026-08-27T09:15:00+02:00\n" +
	"ccc333\x00Scaffold the package\x00Craig Johnston\x002026-08-26T16:00:00+02:00\n"

func at(t *testing.T, stamp string) time.Time {
	t.Helper()
	when, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		t.Fatalf("could not read the timestamp: %v", err)
	}
	return when
}

// TestCommitsRunsGit pins both invocations: the default branch read off
// origin's HEAD, then git log against it in NUL-delimited fields, in the
// slice's repository.
func TestCommitsRunsGit(t *testing.T) {
	runner := &fakeRunner{outs: []string{"origin/main\n", commitLog}}
	commits, err := NewWithRunner(runner).Commits("/repos/nat", "slice/viewer")
	if err != nil {
		t.Fatalf("Commits() = %v, want a commit list", err)
	}
	want := []Commit{
		{SHA: "aaa111", Subject: "Add the viewer", Author: "Craig Johnston", Date: at(t, "2026-08-28T11:30:00+02:00")},
		{SHA: "bbb222", Subject: "Wire the diff screen", Author: "Craig Johnston", Date: at(t, "2026-08-27T09:15:00+02:00")},
		{SHA: "ccc333", Subject: "Scaffold the package", Author: "Craig Johnston", Date: at(t, "2026-08-26T16:00:00+02:00")},
	}
	if !reflect.DeepEqual(commits, want) {
		t.Errorf("Commits() = %+v, want %+v", commits, want)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("made %d calls, want the base read and the log", len(runner.calls))
	}
	wantLog := []string{"log", "--format=" + commitLogFormat, "origin/main..slice/viewer"}
	if !reflect.DeepEqual(runner.calls[1].args, wantLog) {
		t.Errorf("log args = %v, want %v", runner.calls[1].args, wantLog)
	}
	if runner.calls[1].dir != "/repos/nat" {
		t.Errorf("log ran in %q, want the slice's repository", runner.calls[1].dir)
	}
}

// TestCommitsOfAnEmptyRange covers a branch with nothing new since the base: no
// commits, rather than the one empty entry splitting nothing on a newline
// would leave behind.
func TestCommitsOfAnEmptyRange(t *testing.T) {
	runner := &fakeRunner{outs: []string{"origin/main\n", ""}}
	commits, err := NewWithRunner(runner).Commits("/repos/nat", "slice/nochange")
	if err != nil {
		t.Fatalf("Commits() = %v, want no error", err)
	}
	if len(commits) != 0 {
		t.Errorf("Commits() = %+v, want none", commits)
	}
}

// TestCommitsFailure passes git's own refusal straight back.
func TestCommitsFailure(t *testing.T) {
	refused := &ExitError{Code: 128, Stderr: "fatal: bad revision 'slice/gone'\n"}
	runner := &fakeRunner{outs: []string{"origin/main\n"}, errs: []error{nil, refused}}
	commits, err := NewWithRunner(runner).Commits("/repos/nat", "slice/gone")
	if !errors.Is(err, error(refused)) {
		t.Fatalf("Commits() = %v, want git's own error", err)
	}
	if commits != nil {
		t.Errorf("Commits() = %+v, want nothing read", commits)
	}
}

// TestCommitsRefusesAMalformedLine covers git log printing a line this
// package cannot make sense of: fewer fields than the format asked for, which
// would otherwise silently misread one field as another.
func TestCommitsRefusesAMalformedLine(t *testing.T) {
	runner := &fakeRunner{outs: []string{"origin/main\n", "aaa111\x00only two fields\n"}}
	_, err := NewWithRunner(runner).Commits("/repos/nat", "slice/bad")
	if err == nil || !strings.Contains(err.Error(), "2 fields") {
		t.Errorf("Commits() = %v, want it to name the malformed line", err)
	}
}

// TestCommitsRefusesAnUnreadableDate covers a date git log did not write in
// its own %aI format, which this package has no other way to read.
func TestCommitsRefusesAnUnreadableDate(t *testing.T) {
	runner := &fakeRunner{outs: []string{"origin/main\n", "aaa111\x00subject\x00author\x00not-a-date\n"}}
	_, err := NewWithRunner(runner).Commits("/repos/nat", "slice/bad")
	if err == nil || !strings.Contains(err.Error(), "read a commit's date") {
		t.Errorf("Commits() = %v, want it to name the unreadable date", err)
	}
}

// TestCommitDiffRunsGit pins both invocations: the parent verified before
// anything is diffed, then the diff itself against it, with the same pinned
// prefixes and no external diff driver [CLI.Diff] refuses.
func TestCommitDiffRunsGit(t *testing.T) {
	runner := &fakeRunner{outs: []string{"", "diff --git a/x b/x\n"}}
	diff, err := NewWithRunner(runner).CommitDiff("/repos/nat", "aaa111")
	if err != nil {
		t.Fatalf("CommitDiff() = %v, want a diff", err)
	}
	if diff != "diff --git a/x b/x\n" {
		t.Errorf("CommitDiff() = %q, want git's own diff", diff)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("made %d calls, want the parent check and the diff", len(runner.calls))
	}
	wantVerify := []string{"rev-parse", "--verify", "--quiet", "aaa111^"}
	if !reflect.DeepEqual(runner.calls[0].args, wantVerify) {
		t.Errorf("verify args = %v, want %v", runner.calls[0].args, wantVerify)
	}
	wantDiff := []string{"diff", "--no-color", "--no-ext-diff", "--src-prefix=a/", "--dst-prefix=b/",
		"aaa111^", "aaa111"}
	if !reflect.DeepEqual(runner.calls[1].args, wantDiff) {
		t.Errorf("diff args = %v, want %v", runner.calls[1].args, wantDiff)
	}
	for i, c := range runner.calls {
		if c.dir != "/repos/nat" {
			t.Errorf("call %d ran in %q, want the slice's repository", i, c.dir)
		}
	}
}

// TestCommitDiffRefusesARootCommit covers the one commit a first-parent diff
// has nothing to diff against: refused rather than diffed against the empty
// tree, which would answer a different question than the one asked.
func TestCommitDiffRefusesARootCommit(t *testing.T) {
	runner := &fakeRunner{errs: []error{&ExitError{Code: 128}}}
	diff, err := NewWithRunner(runner).CommitDiff("/repos/nat", "root111")
	if !errors.Is(err, ErrNoParent) {
		t.Fatalf("CommitDiff() = %v, want %v", err, ErrNoParent)
	}
	if diff != "" {
		t.Errorf("CommitDiff() = %q, want nothing", diff)
	}
	if len(runner.calls) != 1 {
		t.Errorf("made %d calls, want the parent check alone: a root commit is never diffed", len(runner.calls))
	}
}

// TestCommitDiffFailure covers the diff itself refusing once the parent check
// has passed: git's own reason comes back rather than [ErrNoParent].
func TestCommitDiffFailure(t *testing.T) {
	refused := &ExitError{Code: 128, Stderr: "fatal: bad revision 'aaa111'\n"}
	runner := &fakeRunner{outs: []string{""}, errs: []error{nil, refused}}
	diff, err := NewWithRunner(runner).CommitDiff("/repos/nat", "aaa111")
	if !errors.Is(err, error(refused)) {
		t.Fatalf("CommitDiff() = %v, want git's own error", err)
	}
	if diff != "" {
		t.Errorf("CommitDiff() = %q, want nothing", diff)
	}
}
