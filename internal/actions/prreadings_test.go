package actions

import (
	"errors"
	"reflect"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// TestPRsWorthAsking asks about every In progress slice with a pull request,
// and a Done one only while its worktree still exists — listing each
// repository once — and never a slice with no pull request, a Todo one, or a
// Done one with no repository to look in.
func TestPRsWorthAsking(t *testing.T) {
	w := &fakeWorktrees{existing: map[string]string{"slice/kept": "/repo-worktrees/kept"}}
	p := config.ProjectConfig{WorkingDir: "/repo"}
	working := domain.Slice{ID: "1", Status: domain.SliceClaimed, PRURL: "https://github.test/o/r/pull/1"}
	kept := domain.Slice{ID: "2", Name: "Kept", Status: domain.SliceDone, PRURL: "https://github.test/o/r/pull/2"}
	gone := domain.Slice{ID: "3", Name: "Gone", Status: domain.SliceDone, PRURL: "https://github.test/o/r/pull/3"}
	noPR := domain.Slice{ID: "4", Status: domain.SliceClaimed}
	todo := domain.Slice{ID: "5", Status: domain.SliceTodo, PRURL: "https://github.test/o/r/pull/5"}

	got := PRsWorthAsking(w, p, []domain.Slice{working, kept, gone, noPR, todo})

	if want := []domain.Slice{working, kept}; !reflect.DeepEqual(got, want) {
		t.Errorf("PRsWorthAsking() = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(w.listed, []string{"/repo"}) {
		t.Errorf("listed %v, want the one repository once", w.listed)
	}

	repoless := PRsWorthAsking(w, config.ProjectConfig{Backend: config.BackendSource}, []domain.Slice{kept})
	if len(repoless) != 0 {
		t.Errorf("PRsWorthAsking() = %+v, want a Done slice with no repository left out", repoless)
	}
}

// TestPRsWorthAskingUnlisted asks about no Done slice of a repository whose
// worktrees could not be listed: no worktree can be said to exist.
func TestPRsWorthAskingUnlisted(t *testing.T) {
	w := &fakeWorktrees{branchesErr: errors.New("not a repository")}
	done := domain.Slice{ID: "2", Name: "Kept", Status: domain.SliceDone, PRURL: "https://github.test/o/r/pull/2"}
	if got := PRsWorthAsking(w, config.ProjectConfig{WorkingDir: "/repo"}, []domain.Slice{done}); len(got) != 0 {
		t.Errorf("PRsWorthAsking() = %+v, want none", got)
	}
}

// TestListedOnce lists each repository once, keeps no failed listing, and
// passes every other call through.
func TestListedOnce(t *testing.T) {
	w := &fakeWorktrees{existing: map[string]string{"b": "/p"}}
	once := ListedOnce(w)
	for range 2 {
		if got, err := once.Branches("/repo"); err != nil || !reflect.DeepEqual(got, []string{"b"}) {
			t.Fatalf("Branches() = %v, %v", got, err)
		}
	}
	if len(w.listed) != 1 {
		t.Errorf("listed %d times, want once", len(w.listed))
	}
	if _, err := once.Path("/repo", "b"); err != nil || len(w.looks) != 1 {
		t.Errorf("Path() = %v after %d looks, want it passed through", err, len(w.looks))
	}

	w.branchesErr = errors.New("git refused")
	for range 2 {
		if _, err := once.Branches("/other"); err == nil {
			t.Fatal("a failed listing came back as read")
		}
	}
	if len(w.listed) != 3 {
		t.Errorf("listed %d times, want a failed listing asked again", len(w.listed))
	}
}
