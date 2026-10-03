package cli

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/store"
)

// TestInfoAndSliceShowReadFixingOffTheRecord: an approved slice — in progress,
// its pull request recorded — reads as fixing once a Relaunched follows its
// hand-back, and not once the fix's own hand-back follows that, in both info
// and slice-show; a slice with no pull request never does.
func TestInfoAndSliceShowReadFixingOffTheRecord(t *testing.T) {
	ctx := context.Background()
	env, id, st, sp, _ := scratchWithWork(t)
	env.NewGit = func() GitCLI { return git.NewWithRunner(&fakeGitRunner{base: "origin/main"}) }
	sh, err := st.Shape(ctx, sp)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := st.Plan(ctx, sp)
	if err != nil {
		t.Fatal(err)
	}
	s, err := st.AddSlice(ctx, sp, store.NewSlice{Title: "approved", Milestone: plan.Shape.Milestones[0]})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReopenSlice(ctx, s.ID, sh); err != nil {
		t.Fatal(err)
	}
	handBack := func() {
		t.Helper()
		if _, err := st.CompleteSlice(ctx, s.ID, sh, store.Outcome{Summary: "Done.", Branch: "slice/approved"}); err != nil {
			t.Fatal(err)
		}
	}
	handBack()
	if err := st.RecordPR(ctx, s.ID, "https://github.test/pr/1"); err != nil {
		t.Fatal(err)
	}

	fixing := func() (info, show bool) {
		t.Helper()
		out := env.Out.(interface {
			String() string
			Reset()
		})
		out.Reset()
		if err := Run(ctx, []string{"info", "--json", "--project", id}, env); err != nil {
			t.Fatalf("info: %v", err)
		}
		var doc struct {
			Slices []struct {
				ID     string `json:"id"`
				Fixing bool   `json:"fixing"`
			} `json:"slices"`
		}
		if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
			t.Fatalf("info json: %v", err)
		}
		for _, sj := range doc.Slices {
			if sj.ID == s.ID {
				info = sj.Fixing
			} else if sj.Fixing {
				t.Errorf("slice %s reads as fixing with no pull request", sj.ID)
			}
		}
		out.Reset()
		if err := Run(ctx, []string{"slice-show", s.ID, "--json", "--project", id}, env); err != nil {
			t.Fatalf("slice-show: %v", err)
		}
		var sj struct {
			Fixing bool `json:"fixing"`
		}
		if err := json.Unmarshal([]byte(out.String()), &sj); err != nil {
			t.Fatalf("slice-show json: %v", err)
		}
		return info, sj.Fixing
	}

	if info, show := fixing(); info || show {
		t.Errorf("handed back and approved: fixing = %v/%v, want false", info, show)
	}
	if err := st.RecordRelaunch(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	if info, show := fixing(); !info || !show {
		t.Errorf("after a Relaunched: fixing = %v/%v, want true", info, show)
	}
	handBack()
	if info, show := fixing(); info || show {
		t.Errorf("after the fix's hand-back: fixing = %v/%v, want false", info, show)
	}
}

// failingBodyStore is a plan whose slice bodies cannot be read.
type failingBodyStore struct{ store.Store }

func (failingBodyStore) Body(context.Context, string) (string, error) {
	return "", errBodyUnread
}

var errBodyUnread = errors.New("body unread")

// A task log that cannot be read concludes nothing: the slice reads as not
// fixing, and info goes on.
func TestFixingSlicesPassesOverAnUnreadableBody(t *testing.T) {
	_, _, st, sp, _ := scratchWithWork(t)
	ctx := context.Background()
	plan, err := st.Plan(ctx, sp)
	if err != nil {
		t.Fatal(err)
	}
	approved := plan.Project.Slices[0]
	approved.Status, approved.PRURL = domain.SliceClaimed, "https://github.test/pr/1"
	got := fixingSlices(ctx, failingBodyStore{st}, append(plan.Project.Slices, approved))
	if len(got) != 0 {
		t.Errorf("fixing = %v, want nothing concluded", got)
	}
}
