package gh

import (
	"errors"
	"strings"
	"testing"
)

// TestCommentPRRunsGh pins the invocation: gh, in the slice's repository, told
// which pull request to comment on and to read the body off its own stdin
// rather than an argument.
func TestCommentPRRunsGh(t *testing.T) {
	runner := &fakeRunner{out: "https://github.test/craig/nat/pull/7#issuecomment-9\n"}
	url, err := NewWithRunner(runner).CommentPR("/repos/nat", "7", "Looks good, one nit inline.")
	if err != nil {
		t.Fatalf("CommentPR() = %v, want a comment posted", err)
	}
	if want := "https://github.test/craig/nat/pull/7#issuecomment-9"; url != want {
		t.Errorf("CommentPR() = %q, want %q", url, want)
	}
	if runner.dir != "/repos/nat" {
		t.Errorf("ran in %q, want the slice's repository", runner.dir)
	}
	if runner.name != Binary {
		t.Errorf("ran %q, want %q", runner.name, Binary)
	}
	want := []string{"pr", "comment", "7", "--body-file", "-"}
	if len(runner.args) != len(want) {
		t.Fatalf("args = %v, want %v", runner.args, want)
	}
	for i, a := range want {
		if runner.args[i] != a {
			t.Errorf("args = %v, want %v", runner.args, want)
		}
	}
	if runner.stdin != "Looks good, one nit inline." {
		t.Errorf("stdin = %q, want the comment body", runner.stdin)
	}
}

// TestCommentPRNeedsARef refuses before gh is run at all: gh with no pull
// request named comments on whatever branch the directory happens to be on,
// which for a shared checkout is nobody's slice in particular.
func TestCommentPRNeedsARef(t *testing.T) {
	runner := &fakeRunner{}
	_, err := NewWithRunner(runner).CommentPR("/repos/nat", "", "a comment")
	if err == nil || !strings.Contains(err.Error(), "needs a pull request") {
		t.Errorf("CommentPR() = %v, want it to refuse an unnamed pull request", err)
	}
	if runner.runs != 0 {
		t.Errorf("ran gh %d times, want it not run at all", runner.runs)
	}
}

// TestCommentPRNeedsABody refuses an empty comment before gh is run: there is
// nothing here worth a round trip.
func TestCommentPRNeedsABody(t *testing.T) {
	runner := &fakeRunner{}
	_, err := NewWithRunner(runner).CommentPR("/repos/nat", "7", "   ")
	if err == nil || !strings.Contains(err.Error(), "needs a comment") {
		t.Errorf("CommentPR() = %v, want it to refuse an empty comment", err)
	}
	if runner.runs != 0 {
		t.Errorf("ran gh %d times, want it not run at all", runner.runs)
	}
}

// noStdinRunner is a Runner that answers Run alone, standing in for a runner
// that cannot carry a comment on its own stdin — the case [CLI.CommentPR]
// refuses rather than silently dropping the body.
type noStdinRunner struct{}

func (noStdinRunner) Run(dir, name string, args ...string) (string, error) { return "", nil }

var _ Runner = noStdinRunner{}

// TestCommentPRNeedsAStdinRunner covers a Runner that cannot carry the
// comment at all: refused rather than posted with no body.
func TestCommentPRNeedsAStdinRunner(t *testing.T) {
	_, err := NewWithRunner(noStdinRunner{}).CommentPR("/repos/nat", "7", "a comment")
	if err == nil || !strings.Contains(err.Error(), "cannot carry a comment") {
		t.Errorf("CommentPR() = %v, want it to refuse a runner with no stdin", err)
	}
}

// TestCommentPRFailure hands gh's own words back — "no pull requests found for
// branch X" is the sentence the caller has to show.
func TestCommentPRFailure(t *testing.T) {
	refusal := &ExitError{Code: 1, Stderr: "no pull requests found for branch \"slice/x\"\n"}
	runner := &fakeRunner{err: refusal}
	url, err := NewWithRunner(runner).CommentPR("/repos/nat", "slice/x", "a comment")
	if !errors.Is(err, error(refusal)) {
		t.Errorf("CommentPR() = %v, want gh's own refusal", err)
	}
	if err.Error() != `no pull requests found for branch "slice/x"` {
		t.Errorf("CommentPR() = %q, want gh's first line", err.Error())
	}
	if url != "" {
		t.Errorf("CommentPR() = %q, want nothing read", url)
	}
}

// TestCommentPRWithoutAURL covers a gh that succeeded but printed nothing
// this package recognises as a URL: the comment still went through, so this
// is not an error, only an empty URL to report.
func TestCommentPRWithoutAURL(t *testing.T) {
	runner := &fakeRunner{out: "Commenting on pull request #7\n"}
	url, err := NewWithRunner(runner).CommentPR("/repos/nat", "7", "a comment")
	if err != nil {
		t.Fatalf("CommentPR() = %v, want it to succeed", err)
	}
	if url != "" {
		t.Errorf("CommentPR() = %q, want no URL recognised", url)
	}
}
