package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// longTitle is a title of exactly n runes, every one of them multibyte, so a
// count of bytes would refuse even the shortest.
func longTitle(n int) string { return strings.Repeat("é", n) }

// A created slice may not take a title the board already holds, however it is
// cased, nor one another created slice or a retitling edit takes; an edit may
// not rename a slice to a title already held.
func TestPlanApplyRefusesADuplicateTitle(t *testing.T) {
	for _, tc := range []struct{ name, doc, want string }{
		{"a Todo slice on the board", `{"slices": [{"title": "Queued work", "milestone": "M2: Board"}]}`,
			`slice 1 ("Queued work") is already on the board as a Todo slice: edit it to change its brief, or remove it to replace it`},
		{"another case", `{"slices": [{"title": "  queued WORK ", "milestone": "M2: Board"}]}`,
			`slice 1 ("queued WORK") is already on the board as a Todo slice`},
		{"a slice in progress", `{"slices": [{"title": "Work in flight", "milestone": "M2: Board"}]}`,
			`slice 1 ("Work in flight") is already on the board as an In progress slice: give this one a title of its own`},
		{"a Done slice", `{"slices": [{"title": "Notion client", "milestone": "M2: Board"}]}`,
			`slice 1 ("Notion client") is already on the board as a Done slice: give this one a title of its own`},
		{"two created slices", `{"slices": [{"title": "Fresh", "milestone": "M2: Board"}, {"title": "fresh", "milestone": "M2: Board"}]}`,
			`slice 2 ("fresh") is already slice 1 of the plan: give each its own title`},
		{"an edit's new title", `{"edit": [{"slice": "Queued work", "title": "Fresh"}],
			"slices": [{"title": "Fresh", "milestone": "M2: Board"}]}`,
			`slice 1 ("Fresh") is the title edit 1 gives "Queued work": give this one a title of its own`},
		{"an edit to a held title", `{"edit": [{"slice": "Queued work", "title": "Style the board"}]}`,
			`edit 1 renames "Queued work" to "Style the board", which is already on the board as a Todo slice: give this one a title of its own`},
		{"two edits to one title", `{"edit": [{"slice": "Queued work", "title": "Fresh"}, {"slice": "Style the board", "title": "FRESH"}]}`,
			`edit 2 renames "Style the board" to "FRESH", which is the title edit 1 gives "Queued work"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := changesAPI(t, 2)

			_, err := runPlan(t, api, tc.doc)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			assertNothingChanged(t, api)
		})
	}
}

// A document removing a slice may create its replacement under its title, and
// one retitling a slice may create another under the title it gave up.
func TestPlanApplyAllowsATitleThePlanFrees(t *testing.T) {
	for name, doc := range map[string]string{
		"removed": `{"remove": ["Queued work"], "slices": [{"title": "Queued work", "milestone": "M2: Board"}]}`,
		"renamed": `{"edit": [{"slice": "Queued work", "title": "Older work"}],
			"slices": [{"title": "Queued work", "milestone": "M2: Board"}]}`,
		// A rename changing only the case is the slice keeping its own title.
		"recased": `{"edit": [{"slice": "Queued work", "title": "Queued Work"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := runPlan(t, changesAPI(t, 1), doc); err != nil {
				t.Fatalf("plan-apply: %v", err)
			}
		})
	}
}

// Two slices already sharing a title that the plan does not touch are not the
// document's doing, and do not stop it.
func TestPlanApplyLeavesADuplicateAlreadyOnTheBoard(t *testing.T) {
	api := changesAPI(t, 1)
	api.pages["slices-ds"] = append(api.pages["slices-ds"],
		slicePage("3b838308f654816da085f46dd135ade7", "Queued work", notion.SliceTodo, "M1: Client", "", ""))

	if _, err := runPlan(t, api, `{"slices": [{"title": "Fresh", "milestone": "M2: Board"}]}`); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}
}

// An edit's title alone renames the slice and leaves its body alone; with a
// description too, both are written; the output says which.
func TestPlanApplyRetitlesAnEditedSlice(t *testing.T) {
	t.Run("title alone", func(t *testing.T) {
		api := changesAPI(t, 0)
		out, err := runPlan(t, api, `{"edit": [{"slice": "Queued work", "title": " Older work "}]}`, "--json")
		if err != nil {
			t.Fatalf("plan-apply: %v", err)
		}
		if len(api.updates) != 1 || api.updates[0].id != depSpare || titleText(api.updates[0].props) != "Older work" {
			t.Errorf("updates = %+v, want the one rename", api.updates)
		}
		if len(api.appends) != 0 || len(api.deletes) != 0 {
			t.Errorf("appends = %+v, deletes = %v, want the body left alone", api.appends, api.deletes)
		}
		var got planAppliedJSON
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("not JSON: %v\n%s", err, out)
		}
		if len(got.Edited) != 1 || got.Edited[0].Name != "Queued work" || got.Edited[0].Title != "Older work" || got.Edited[0].Brief != "" {
			t.Errorf("edited = %+v, want the rename alone", got.Edited)
		}
	})
	t.Run("both", func(t *testing.T) {
		api := changesAPI(t, 0)
		out, err := runPlan(t, api, `{"edit": [{"slice": "Queued work", "title": "Older work", "description": "Queue it."}]}`)
		if err != nil {
			t.Fatalf("plan-apply: %v", err)
		}
		if len(api.updates) != 1 || titleText(api.updates[0].props) != "Older work" || len(api.appends) != 1 {
			t.Errorf("updates = %+v, appends = %+v, want the rename and the brief", api.updates, api.appends)
		}
		if !strings.Contains(out, `- Queued work — renamed "Older work", brief replaced`) {
			t.Errorf("output =\n%s\nwant the edit reported", out)
		}
	})
	t.Run("title alone, read", func(t *testing.T) {
		out, err := runPlan(t, changesAPI(t, 0), `{"edit": [{"slice": "Queued work", "title": "Older work"}]}`)
		if err != nil {
			t.Fatalf("plan-apply: %v", err)
		}
		if !strings.Contains(out, `- Queued work — renamed "Older work"`+"\n") {
			t.Errorf("output =\n%s\nwant the rename reported", out)
		}
	})
}

// The cap holds on a created slice's title and an edit's, counted in runes.
func TestPlanApplyCapsTitles(t *testing.T) {
	for _, tc := range []struct {
		name, doc, want string
	}{
		{"a 64-rune created title", fmt.Sprintf(`{"slices": [{"title": %q, "milestone": "M2: Board"}]}`, longTitle(64)), ""},
		{"a 65-rune created title", fmt.Sprintf(`{"slices": [{"title": %q, "milestone": "M2: Board"}]}`, longTitle(65)),
			"slice 1: the title \"" + longTitle(65) + "\" is 65 characters, over the 64"},
		{"a 64-rune edit title", fmt.Sprintf(`{"edit": [{"slice": "Queued work", "title": %q}]}`, longTitle(64)), ""},
		{"a 65-rune edit title", fmt.Sprintf(`{"edit": [{"slice": "Queued work", "title": %q}]}`, longTitle(65)),
			"edit 1 (\"Queued work\"): the title \"" + longTitle(65) + "\" is 65 characters"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := changesAPI(t, 1)
			_, err := runPlan(t, api, tc.doc)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("plan-apply: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			assertNothingChanged(t, api)
		})
	}
}

func (s *changesStub) SetSliceTitle(_ context.Context, id, _ string) error { return s.call("title " + id) }

// A rename that fails stops the run as a failed brief write does.
func TestApplyPlanReportsAFailedRename(t *testing.T) {
	st := &changesStub{failOn: "title edited"}
	doc, targets := stubChanges()
	targets.changes.edits[0].title = "Renamed"

	_, err := applyPlan(context.Background(), st, store.Project{}, store.Shape{}, doc, targets, nil)

	if err == nil || !strings.Contains(err.Error(), `edit "Edited": notion is down`) {
		t.Errorf("err = %v, want the rename's failure", err)
	}
	if len(st.calls) != 1 {
		t.Errorf("calls = %v, want nothing after the failed rename", st.calls)
	}
}

// plan-propose --project reads the board and refuses a duplicate before the
// proposal file is written, so the agent hears of it rather than the user.
func TestPlanProposeWithProjectRefusesADuplicate(t *testing.T) {
	env, _ := proposeProjectEnv(t, testConfig(t), changesAPI(t, 0))
	env.In = strings.NewReader(`{"slices": [{"title": "Queued work", "milestone": "M2: Board"}]}`)

	err := Run(context.Background(), []string{"plan-propose", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), `slice 1 ("Queued work") is already on the board as a Todo slice`) {
		t.Fatalf("err = %v, want the duplicate refused", err)
	}
	assertNothingWritten(t, "project-1")
}

// A new project's workspace has no board, so only the document's own titles
// are checked against each other, and its titles against the cap.
func TestPlanProposeWithWorkspaceChecksOnlyItsOwnTitles(t *testing.T) {
	for _, tc := range []struct{ doc, want string }{
		{`{"milestones": [{"name": "M1"}], "slices": [{"title": "A", "milestone": "M1"}, {"title": "a", "milestone": "M1"}]}`,
			`slice 2 ("a") is already slice 1 of the plan`},
		{fmt.Sprintf(`{"milestones": [{"name": "M1"}], "slices": [{"title": %q, "milestone": "M1"}]}`, longTitle(65)),
			"is 65 characters"},
	} {
		_, _, err := runPropose(t, tc.doc, "--workspace", "ws-1", "--name", "importer")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("err = %v, want %q", err, tc.want)
		}
		assertNothingWritten(t, "ws-1")
	}
	if _, _, err := runPropose(t, validProposalDoc, "--workspace", "ws-1", "--name", "importer"); err != nil {
		t.Errorf("a plan with distinct titles: %v", err)
	}
}

// plan-accept re-validates against the board as it stands, so a slice filed
// by hand under a proposed title since the proposal was written refuses it.
func TestPlanAcceptWithProjectRefusesADuplicateFiledSince(t *testing.T) {
	env, _, _ := acceptEnv(t)
	id := makeLocalProject(t, env)
	proposeToProject(t, env, id, validProposalDoc)
	if err := Run(context.Background(), []string{"milestone-add", "M0", "--project", id}, env); err != nil {
		t.Fatalf("milestone-add: %v", err)
	}
	if err := Run(context.Background(), []string{"slice-add", "Build on it", "--milestone", "M0", "--project", id}, env); err != nil {
		t.Fatalf("slice-add: %v", err)
	}

	err := Run(context.Background(), []string{"plan-accept", "--project", id}, env)

	if err == nil || !strings.Contains(err.Error(), `slice 2 ("Build on it") is already on the board as a Todo slice`) {
		t.Fatalf("err = %v, want the duplicate refused", err)
	}
}

// acceptWithPlanner runs plan-accept --project with a tmux whose planning
// session for the project is live (or not), returning what was sent.
func acceptWithPlanner(t *testing.T, live bool, sendErr string) (*agentTestRunner, string, error) {
	t.Helper()
	env, _, _ := acceptEnv(t)
	id := makeLocalProject(t, env)
	proposeToProject(t, env, id, validProposalDoc)
	runner := &agentTestRunner{liveSessions: map[string]string{}, sendErr: sendErr}
	if live {
		runner.liveSessions[agent.PlanTag(id)] = "nat-plan-x"
	}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	err := Run(context.Background(), []string{"plan-accept", "--project", id}, env)
	return runner, id, err
}

// A live planning session is told its proposal was accepted, with what the
// accept filed by name.
func TestPlanAcceptTellsTheLivePlanningAgent(t *testing.T) {
	runner, id, err := acceptWithPlanner(t, true, "")
	if err != nil {
		t.Fatalf("plan-accept: %v", err)
	}
	want := agent.ProposalAcceptedPrompt(id, []string{`"M1: Groundwork"`},
		[]string{`"Lay the foundation" (M1: Groundwork)`, `"Build on it" (M1: Groundwork)`})
	if len(runner.sends) != 1 || runner.sends[0].session != "nat-plan-x" || runner.sends[0].prompt != want {
		t.Errorf("sends = %+v, want one to nat-plan-x of\n%s", runner.sends, want)
	}
}

// No live planning session is told nothing; a failed send or an unreadable
// tmux leaves the accept succeeded.
func TestPlanAcceptTellsNobodyAndNeverFailsOverTheTelling(t *testing.T) {
	runner, _, err := acceptWithPlanner(t, false, "")
	if err != nil || len(runner.sends) != 0 {
		t.Errorf("no live session: err = %v, sends = %+v, want success and none", err, runner.sends)
	}
	if _, _, err := acceptWithPlanner(t, true, "pane gone"); err != nil {
		t.Errorf("a failed send: err = %v, want the accept to stand", err)
	}

	env, _, _ := acceptEnv(t)
	id := makeLocalProject(t, env)
	proposeToProject(t, env, id, validProposalDoc)
	broken := &agentTestRunner{liveFatalErr: "tmux broke"}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(broken) }
	if err := Run(context.Background(), []string{"plan-accept", "--project", id}, env); err != nil {
		t.Errorf("an unreadable tmux: err = %v, want the accept to stand", err)
	}
}

// A new project's accept has no planning session of a project to tell: it
// reads no tmux at all.
func TestPlanAcceptWithWorkspaceTellsNobody(t *testing.T) {
	env, _, _ := acceptEnv(t)
	env.NewTmux = func() *agent.Tmux { t.Fatal("tmux was read"); return nil }
	propose(t, env, "ws-1", "importer", validProposalDoc)
	if err := Run(context.Background(), []string{"plan-accept", "--workspace", "ws-1", "--name", "importer"}, env); err != nil {
		t.Fatalf("plan-accept: %v", err)
	}
}

func TestSliceEditRetitles(t *testing.T) {
	t.Run("title alone", func(t *testing.T) {
		api := editableAPI()
		env, out := testEnv(testConfig(t), api)
		if err := Run(context.Background(), []string{"slice-edit", testSliceID, "--title", " Draw the board ",
			"--project", "project-1"}, env); err != nil {
			t.Fatalf("slice-edit: %v", err)
		}
		if len(api.updates) != 1 || titleText(api.updates[0].props) != "Draw the board" {
			t.Errorf("updates = %+v, want the rename", api.updates)
		}
		if len(api.appends) != 0 || len(api.deletes) != 0 {
			t.Errorf("appends = %+v, deletes = %v, want the body left alone", api.appends, api.deletes)
		}
		want := "# Draw the board\n\nRenamed from \"Render the board\".\n\n- Notion page: " + testSliceID + "\n- Notion URL: https://notion.so/" + testSliceID + "\n- Working directory: /tmp/nat\n"
		if out.String() != want {
			t.Errorf("output =\n%q\nwant\n%q", out.String(), want)
		}
	})
	t.Run("both, as JSON", func(t *testing.T) {
		api := editableAPI()
		env, out := testEnv(testConfig(t), api)
		if err := Run(context.Background(), []string{"slice-edit", testSliceID, "--title", "Draw the board",
			"--description", "New brief.", "--json", "--project", "project-1"}, env); err != nil {
			t.Fatalf("slice-edit: %v", err)
		}
		if len(api.updates) != 1 || len(api.appends) != 1 {
			t.Errorf("updates = %+v, appends = %+v, want the rename and the brief", api.updates, api.appends)
		}
		var got sliceEditedJSON
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("not JSON: %v", err)
		}
		if got.Name != "Render the board" || got.Title != "Draw the board" || got.Brief != "New brief." {
			t.Errorf("json = %+v", got)
		}
	})
}

// A rename that fails writes no brief; a brief that fails after a rename says
// so and still nudges, since the rename stands.
func TestSliceEditRetitleFailures(t *testing.T) {
	run := func(t *testing.T, breakPlan string) (int, error) {
		t.Helper()
		cfg := testConfig(t)
		seedHydratedSlice(t, "project-1", testSliceID, "Render the board", "Todo", func(db *sql.DB) {
			if _, err := db.Exec(breakPlan); err != nil {
				t.Fatalf("break the plan: %v", err)
			}
		})
		env, _ := testEnv(cfg, &fakeAPI{})
		var nudges int
		env.Nudge = func() { nudges++ }
		err := Run(context.Background(), []string{"slice-edit", testSliceID, "--title", "T", "--description", "B",
			"--project", "project-1"}, env)
		return nudges, err
	}
	t.Run("the rename", func(t *testing.T) {
		nudges, err := run(t, `CREATE TRIGGER no_rename BEFORE UPDATE OF title ON slices BEGIN SELECT RAISE(FAIL, 'no'); END`)
		if err == nil || nudges != 0 {
			t.Errorf("err = %v, nudges = %d, want the failure with nothing written", err, nudges)
		}
	})
	t.Run("the brief after it", func(t *testing.T) {
		nudges, err := run(t, `ALTER TABLE slices DROP COLUMN body`)
		if err == nil || nudges != 1 {
			t.Errorf("err = %v, nudges = %d, want the failure, nudged for the rename", err, nudges)
		}
	})
}

// slice-edit, slice-add and slice-followups each hold a title to the cap,
// counted in runes, before anything is written.
func TestDirectWritesCapTitles(t *testing.T) {
	t.Run("slice-edit", func(t *testing.T) {
		for n, ok := range map[int]bool{64: true, 65: false} {
			api := editableAPI()
			env, _ := testEnv(testConfig(t), api)
			err := Run(context.Background(), []string{"slice-edit", testSliceID, "--title", longTitle(n), "--project", "project-1"}, env)
			if ok != (err == nil) {
				t.Errorf("%d runes: err = %v", n, err)
			}
			if !ok && (len(api.updates) != 0 || !strings.Contains(err.Error(), "is 65 characters")) {
				t.Errorf("%d runes: err = %v, updates = %+v, want refused before a write", n, err, api.updates)
			}
		}
	})
	t.Run("slice-add", func(t *testing.T) {
		for n, ok := range map[int]bool{64: true, 65: false} {
			api := plannedAPI(addedSliceID)
			env, _ := testEnv(testConfig(t), api)
			err := Run(context.Background(), []string{"slice-add", longTitle(n), "--milestone", "M2: Board", "--project", "project-1"}, env)
			if ok != (err == nil) {
				t.Errorf("%d runes: err = %v", n, err)
			}
			if !ok && (len(api.creates) != 0 || !strings.Contains(err.Error(), "put the detail in the brief")) {
				t.Errorf("%d runes: err = %v, creates = %+v, want refused before a write", n, err, api.creates)
			}
		}
	})
	t.Run("slice-followups", func(t *testing.T) {
		if _, err := followUpsOf([]string{longTitle(64) + "\n\nDo it.\nDone when: done."}); err != nil {
			t.Errorf("64 runes: %v", err)
		}
		_, err := followUpsOf([]string{longTitle(65) + "\n\nDo it.\nDone when: done."})
		if err == nil || !strings.Contains(err.Error(), "slice-followups: the title") || !strings.Contains(err.Error(), "is 65 characters") {
			t.Errorf("65 runes: err = %v, want the cap", err)
		}
	})
}

// The help text states the cap, with the number domain holds.
func TestUsageStatesTheTitleCap(t *testing.T) {
	if want := fmt.Sprintf("at most %d characters", domain.MaxSliceTitleLen); strings.Count(Usage, want) != 2 {
		t.Errorf("Usage says %q %d times, want twice (slice-add and plan-apply)", want, strings.Count(Usage, want))
	}
	if !strings.Contains(Usage, "nat slice-edit <slice> [--title TITLE] [--description TEXT|-]") {
		t.Error("Usage does not give slice-edit's --title")
	}
}

// withArticle reads the status as the project spells it, and copes with none.
func TestWithArticle(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status domain.SliceStatus
		want   string
	}{
		{"Todo", domain.SliceTodo, "a Todo"},
		{"In progress", domain.SliceClaimed, "an In progress"},
		{"", domain.SliceDone, "a Done"},
		{"", "", "a "},
	} {
		if got := withArticle(tc.name, tc.status); got != tc.want {
			t.Errorf("withArticle(%q, %q) = %q, want %q", tc.name, tc.status, got, tc.want)
		}
	}
}
