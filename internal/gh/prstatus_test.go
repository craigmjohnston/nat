package gh

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestOpenPRsRunsGh pins the invocation: gh, in the slice's repository, asked
// for the open pull requests alone and for the three fields alone, with a limit
// past gh's own default.
func TestOpenPRsRunsGh(t *testing.T) {
	runner := &fakeRunner{out: `[{"url":"https://github.test/craig/nat/pull/7",` +
		`"reviewDecision":"APPROVED","mergeable":"MERGEABLE"}]`}
	open, err := NewWithRunner(runner).OpenPRs("/repos/nat")
	if err != nil {
		t.Fatalf("OpenPRs() = %v, want a listing", err)
	}
	status, listed := open["https://github.test/craig/nat/pull/7"]
	if !listed {
		t.Fatalf("OpenPRs() = %+v, want the pull request keyed by its URL", open)
	}
	if !status.Approved || !status.Mergeable {
		t.Errorf("OpenPRs() = %+v, want it approved and mergeable", status)
	}
	if runner.dir != "/repos/nat" {
		t.Errorf("ran in %q, want the slice's repository", runner.dir)
	}
	if runner.name != Binary {
		t.Errorf("ran %q, want %q", runner.name, Binary)
	}
	want := []string{"pr", "list", "--state", "open", "--json", "url,reviewDecision,mergeable",
		"--limit", "100"}
	if !reflect.DeepEqual(runner.args, want) {
		t.Errorf("args = %v, want %v", runner.args, want)
	}
}

// TestOpenPRsReadings walks what GitHub answers with: only the two affirmative
// words count, and every other value it uses — an unreviewed pull request,
// changes asked for, a conflicting merge, a mergeability GitHub is still
// working out — is read as the fact not being true.
func TestOpenPRsReadings(t *testing.T) {
	const url = "https://github.test/pr/7"
	tests := []struct {
		name          string
		fields        string
		wantApproved  bool
		wantMergeable bool
	}{
		{name: "approved and mergeable", fields: `"reviewDecision":"APPROVED","mergeable":"MERGEABLE"`,
			wantApproved: true, wantMergeable: true},
		{name: "review required", fields: `"reviewDecision":"REVIEW_REQUIRED","mergeable":"MERGEABLE"`,
			wantMergeable: true},
		{name: "changes requested", fields: `"reviewDecision":"CHANGES_REQUESTED","mergeable":"MERGEABLE"`,
			wantMergeable: true},
		// A repository that requires no review and has had none says nothing at
		// all, which is not an approval.
		{name: "no decision", fields: `"reviewDecision":"","mergeable":"MERGEABLE"`, wantMergeable: true},
		{name: "conflicting", fields: `"reviewDecision":"APPROVED","mergeable":"CONFLICTING"`,
			wantApproved: true},
		{name: "mergeability unknown", fields: `"reviewDecision":"APPROVED","mergeable":"UNKNOWN"`,
			wantApproved: true},
		{name: "nothing said", fields: ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `"url":"` + url + `"`
			if tt.fields != "" {
				body += "," + tt.fields
			}
			runner := &fakeRunner{out: "[{" + body + "}]"}
			open, err := NewWithRunner(runner).OpenPRs("/repos/nat")
			if err != nil {
				t.Fatalf("OpenPRs() = %v, want a listing", err)
			}
			status := open[url]
			if status.Approved != tt.wantApproved || status.Mergeable != tt.wantMergeable {
				t.Errorf("OpenPRs() = %+v, want approved=%v mergeable=%v",
					status, tt.wantApproved, tt.wantMergeable)
			}
		})
	}
}

// A pull request that is not open is simply not in the listing, which is the
// whole fact the board reads off it: an empty listing names nothing, and no
// pull request is taken for open on the strength of nothing.
func TestOpenPRsListsOnlyWhatIsOpen(t *testing.T) {
	runner := &fakeRunner{out: "[]\n"}
	open, err := NewWithRunner(runner).OpenPRs("/repos/nat")
	if err != nil {
		t.Fatalf("OpenPRs() = %v, want a listing", err)
	}
	if len(open) != 0 {
		t.Errorf("OpenPRs() = %+v, want nothing named", open)
	}
}

// The listing is keyed the way a URL off a Notion page is looked up, so a link
// copied from a review page or typed with a trailing slash still finds it.
func TestOpenPRsKeysNormalisedURLs(t *testing.T) {
	runner := &fakeRunner{out: `[{"url":"https://github.test/Craig/Nat/pull/7/","mergeable":"MERGEABLE"}]`}
	open, err := NewWithRunner(runner).OpenPRs("/repos/nat")
	if err != nil {
		t.Fatalf("OpenPRs() = %v, want a listing", err)
	}
	if _, listed := open[NormaliseURL("https://github.test/craig/nat/pull/7?w=1")]; !listed {
		t.Errorf("OpenPRs() = %+v, want the URL keyed as it is looked up", open)
	}
}

// TestNormaliseURL walks what the same pull request can be written as.
func TestNormaliseURL(t *testing.T) {
	const want = "https://github.test/craig/nat/pull/7"
	tests := []string{
		"https://github.test/craig/nat/pull/7",
		"https://github.test/craig/nat/pull/7/",
		"  https://github.test/craig/nat/pull/7  ",
		"https://github.test/craig/nat/pull/7?w=1",
		"https://github.test/craig/nat/pull/7#issuecomment-1",
		"https://github.test/Craig/Nat/pull/7",
	}
	for _, url := range tests {
		if got := NormaliseURL(url); got != want {
			t.Errorf("NormaliseURL(%q) = %q, want %q", url, got, want)
		}
	}
}

// TestOpenPRsFailure passes gh's own refusal straight back — an unauthenticated
// gh, or a directory that is no repository — since the caller's answer to it is
// to conclude nothing at all.
func TestOpenPRsFailure(t *testing.T) {
	refusal := &ExitError{Code: 1, Stderr: "gh: Not Found (HTTP 404)\n"}
	runner := &fakeRunner{err: refusal}
	open, err := NewWithRunner(runner).OpenPRs("/repos/nat")
	if !errors.Is(err, error(refusal)) {
		t.Errorf("OpenPRs() = %v, want gh's own refusal", err)
	}
	if open != nil {
		t.Errorf("OpenPRs() = %+v, want nothing read", open)
	}
}

// TestOpenPRsUnreadableJSON covers a gh that exited zero and printed something
// that is not the JSON it was asked for: there is no listing in it, so it is a
// failure here rather than a repository read as having nothing open.
func TestOpenPRsUnreadableJSON(t *testing.T) {
	runner := &fakeRunner{out: "not JSON at all\n"}
	_, err := NewWithRunner(runner).OpenPRs("/repos/nat")
	if err == nil || !strings.Contains(err.Error(), "no readable JSON") {
		t.Errorf("OpenPRs() = %v, want it to report the unreadable output", err)
	}
}
