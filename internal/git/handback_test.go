package git

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDirtyPathsNamesEveryUncommittedPath(t *testing.T) {
	runner := &fakeRunner{outs: []string{" M internal/a.go\nA  b.go\n?? notes.txt\nR  old.go -> new.go\n\n"}}
	paths, err := NewWithRunner(runner).DirtyPaths("/wt")
	if err != nil {
		t.Fatalf("DirtyPaths: %v", err)
	}
	want := []string{"internal/a.go", "b.go", "notes.txt", "old.go -> new.go"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("paths = %q, want %q", paths, want)
	}
	wantArgs := []string{"status", "--porcelain", "--untracked-files=normal"}
	if c := runner.calls[0]; c.dir != "/wt" || !reflect.DeepEqual(c.args, wantArgs) {
		t.Errorf("ran %+v, want %v in /wt", c, wantArgs)
	}
}

func TestDirtyPathsOfACleanTreeIsNone(t *testing.T) {
	paths, err := NewWithRunner(&fakeRunner{outs: []string{""}}).DirtyPaths("/wt")
	if err != nil || len(paths) != 0 {
		t.Errorf("DirtyPaths = %q, %v, want none", paths, err)
	}
}

func TestDirtyPathsReportsAFailedStatus(t *testing.T) {
	boom := errors.New("not a git repository")
	if _, err := NewWithRunner(&fakeRunner{errs: []error{boom}}).DirtyPaths("/wt"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestPushPushesWithALease(t *testing.T) {
	runner := &fakeRunner{}
	if err := NewWithRunner(runner).Push("/wt", "slice/x"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	want := []string{"push", "--force-with-lease", "-u", "origin", "slice/x"}
	if c := runner.calls[0]; c.dir != "/wt" || !reflect.DeepEqual(c.args, want) {
		t.Errorf("ran %+v, want %v in /wt", c, want)
	}
}

func TestPushRefusedCarriesGitsWholeOutput(t *testing.T) {
	stderr := "To github.com:x/y.git\n ! [rejected]        slice/x -> slice/x (stale info)\nerror: failed to push some refs\n"
	runner := &fakeRunner{errs: []error{&ExitError{Code: 1, Stderr: stderr}}}
	err := NewWithRunner(runner).Push("/wt", "slice/x")
	if err == nil {
		t.Fatal("Push: want an error")
	}
	if !strings.Contains(err.Error(), "(stale info)") || !strings.Contains(err.Error(), "failed to push some refs") {
		t.Errorf("err = %q, want all of git's output", err)
	}
}

func TestPushFailedOtherwiseWrapsTheError(t *testing.T) {
	boom := errors.New("exec: git not found")
	err := NewWithRunner(&fakeRunner{errs: []error{boom}}).Push("/wt", "slice/x")
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want it wrapping %v", err, boom)
	}
	silent := &ExitError{Code: 128}
	err = NewWithRunner(&fakeRunner{errs: []error{silent}}).Push("/wt", "slice/x")
	if !errors.Is(err, silent) {
		t.Errorf("err = %v, want it wrapping %v", err, silent)
	}
}
