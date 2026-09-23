package gh

import (
	"errors"
	"slices"
	"testing"
)

func TestListPRsForHead(t *testing.T) {
	runner := &fakeRunner{out: `[
		{"number":1,"title":"First","url":"https://github.test/craig/nat/pull/1","state":"MERGED","mergedAt":"2026-01-02T03:04:05Z"},
		{"number":2,"title":"Second","url":"https://github.test/craig/nat/pull/2","state":"OPEN"}
	]`}
	prs, err := NewWithRunner(runner).ListPRsForHead("/repo", "session/abcd1234")
	if err != nil {
		t.Fatalf("ListPRsForHead: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("ListPRsForHead = %+v, want two", prs)
	}
	if prs[0].Number != 1 || prs[0].State != "MERGED" {
		t.Errorf("prs[0] = %+v, want #1 merged", prs[0])
	}
	if prs[1].Number != 2 || prs[1].State != "OPEN" {
		t.Errorf("prs[1] = %+v, want #2 open", prs[1])
	}
	if !slices.Contains(runner.args, "--head") || !slices.Contains(runner.args, "session/abcd1234") {
		t.Errorf("args = %v, want --head session/abcd1234", runner.args)
	}
}

func TestListPRsForHeadRefusesAnEmptyBranch(t *testing.T) {
	runner := &fakeRunner{}
	if _, err := NewWithRunner(runner).ListPRsForHead("/repo", ""); err == nil {
		t.Fatal("ListPRsForHead with no branch: want a refusal")
	}
	if runner.runs != 0 {
		t.Errorf("runs = %d, want 0: gh should never have been asked", runner.runs)
	}
}

func TestListPRsForHeadFailure(t *testing.T) {
	runner := &fakeRunner{err: errors.New("boom")}
	if _, err := NewWithRunner(runner).ListPRsForHead("/repo", "b"); err == nil {
		t.Fatal("ListPRsForHead: want the failure surfaced")
	}
}

func TestListPRsForHeadUnreadableJSON(t *testing.T) {
	runner := &fakeRunner{out: "not json"}
	if _, err := NewWithRunner(runner).ListPRsForHead("/repo", "b"); err == nil {
		t.Fatal("ListPRsForHead with unreadable JSON: want an error")
	}
}
