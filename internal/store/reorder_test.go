package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// positionOf reads a slice's position straight off the file, since a reorder's
// whole claim is about the number it leaves there.
func positionOf(t *testing.T, l *Local, id string) float64 {
	t.Helper()
	var p float64
	if err := l.db.QueryRow(`SELECT position FROM slices WHERE id = ?`, id).Scan(&p); err != nil {
		t.Fatalf("read %s's position: %v", id, err)
	}
	return p
}

// order is the IDs of the slices filed under a milestone, as the plan reads them.
func order(t *testing.T, l *Local, milestone string) []string {
	t.Helper()
	all, err := l.slices(context.Background(), l.db)
	if err != nil {
		t.Fatalf("slices: %v", err)
	}
	var ids []string
	for _, s := range all {
		if s.MilestoneID == milestone {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

func TestLocalReorderSliceWithinAMilestone(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	write(t, l, `INSERT INTO slices (id, title, status, milestone, position) VALUES ('extra', 'Extra', 'Todo', 'M2: Reads', 5)`)
	ctx := context.Background()

	// Before the first of a milestone: one step below it, nothing else touched.
	moved, to, err := l.ReorderSlice(ctx, wholeShape, "writes", "reads", true)
	if err != nil {
		t.Fatalf("ReorderSlice: %v", err)
	}
	if moved.ID != "writes" || to.ID != "reads" || moved.MilestoneID != "M2: Reads" {
		t.Errorf("answered %+v beside %+v, want writes beside reads under M2", moved, to)
	}
	if got := positionOf(t, l, "writes"); got != -1 {
		t.Errorf("position = %v, want one step below reads", got)
	}
	if got := positionOf(t, l, "reads"); got != 0 {
		t.Errorf("reads' position = %v, want it untouched", got)
	}
	// A same-milestone reorder leaves nothing for a sync to send.
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want a reorder within a milestone to set no flag")
	}

	// After the last: one step past it.
	if _, _, err := l.ReorderSlice(ctx, wholeShape, "reads", "extra", false); err != nil {
		t.Fatalf("ReorderSlice: %v", err)
	}
	if got := positionOf(t, l, "reads"); got != 6 {
		t.Errorf("position = %v, want one step past extra", got)
	}

	// Between two: the midpoint, sparse, and no other row rewritten.
	if _, _, err := l.ReorderSlice(ctx, wholeShape, "reads", "extra", true); err != nil {
		t.Fatalf("ReorderSlice: %v", err)
	}
	if got := positionOf(t, l, "reads"); got != 2 {
		t.Errorf("position = %v, want the midpoint of writes (-1) and extra (5)", got)
	}
	if _, _, err := l.ReorderSlice(ctx, wholeShape, "extra", "reads", false); err != nil {
		t.Fatalf("ReorderSlice: %v", err)
	}
	if got := positionOf(t, l, "extra"); got != 3 {
		t.Errorf("position = %v, want one step past reads", got)
	}
	// After a slice that has a successor: halfway to it.
	if _, _, err := l.ReorderSlice(ctx, wholeShape, "extra", "writes", false); err != nil {
		t.Fatalf("ReorderSlice: %v", err)
	}
	if got := positionOf(t, l, "extra"); got != 0.5 {
		t.Errorf("position = %v, want the midpoint of writes (-1) and reads (2)", got)
	}
	if got, want := strings.Join(order(t, l, "M2: Reads"), ","), "writes,extra,reads"; got != want {
		t.Errorf("order = %s, want %s", got, want)
	}
}

func TestLocalReorderSliceAcrossMilestonesRefilesAndMarksDirty(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	moved, to, err := l.ReorderSlice(ctx, wholeShape, "stray", "design", false)
	if err != nil {
		t.Fatalf("ReorderSlice: %v", err)
	}
	if moved.MilestoneID != "M1: The format" || to.ID != "design" {
		t.Errorf("answered %+v beside %+v, want stray refiled under M1", moved, to)
	}
	if got := readBack(t, l, "stray"); got.MilestoneID != "M1: The format" {
		t.Errorf("stored milestone = %q, want the refile written", got.MilestoneID)
	}
	if got := positionOf(t, l, "stray"); got != 1 {
		t.Errorf("position = %v, want one step past design", got)
	}
	if dirty, _ := l.Dirty(ctx, "stray"); !dirty {
		t.Error("dirty = false, want the refile marked for the workspace")
	}

	// And out of a milestone, under none.
	moved, _, err = l.ReorderSlice(ctx, wholeShape, "writes", "stray", true)
	if err != nil {
		t.Fatalf("ReorderSlice: %v", err)
	}
	if moved.MilestoneID != "M1: The format" {
		t.Errorf("milestone = %q, want writes refiled beside stray", moved.MilestoneID)
	}
	moved, _, err = l.ReorderSlice(ctx, wholeShape, "writes", "design", true)
	if err != nil || moved.MilestoneID != "M1: The format" {
		t.Errorf("ReorderSlice = %+v, %v", moved, err)
	}
	write(t, l, `UPDATE slices SET milestone = NULL WHERE id = 'design'`)
	moved, _, err = l.ReorderSlice(ctx, wholeShape, "writes", "design", true)
	if err != nil || moved.MilestoneID != "" {
		t.Errorf("ReorderSlice under none = %+v, %v, want the slice under no milestone", moved, err)
	}
}

func TestLocalReorderSliceRefusals(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	if _, _, err := l.ReorderSlice(ctx, wholeShape, "reads", "reads", true); !errors.Is(err, errReorderSelf) {
		t.Errorf("err = %v, want the slice as its own target refused", err)
	}
	if _, _, err := l.ReorderSlice(ctx, wholeShape, "ghost", "reads", true); !errors.Is(err, ErrSliceNotFound) {
		t.Errorf("err = %v, want the unknown slice refused", err)
	}
	if _, _, err := l.ReorderSlice(ctx, wholeShape, "reads", "ghost", true); !errors.Is(err, ErrSliceNotFound) {
		t.Errorf("err = %v, want the unknown target refused", err)
	}
	if got := positionOf(t, l, "reads"); got != 0 {
		t.Errorf("position = %v, want a refusal to leave the plan as it was", got)
	}

	// Two slices tied on position leave no room between them.
	write(t, l, `INSERT INTO slices (id, title, status, milestone, position) VALUES ('tied', 'Tied', 'Todo', 'M2: Reads', 1)`)
	write(t, l, `INSERT INTO slices (id, title, status, milestone, position) VALUES ('mover', 'Mover', 'Todo', 'M1: The format', 9)`)
	if _, _, err := l.ReorderSlice(ctx, wholeShape, "mover", "writes", true); err == nil ||
		!strings.Contains(err.Error(), "tied") {
		t.Errorf("err = %v, want the tie refused", err)
	}
}

// columnsOf is a table's columns, comma-separated, for a view that stands in
// for it with one of them made to fail.
func columnsOf(t *testing.T, l *Local, table string) string {
	t.Helper()
	rows, err := l.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatalf("read %s's columns: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		names = append(names, n)
	}
	return strings.Join(names, ", ")
}

// The write itself refused, with every read leading up to it answering.
func TestLocalReorderSliceNamesItsFileWhenTheWriteIsRefused(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	write(t, l, `CREATE TRIGGER block_reorder BEFORE UPDATE OF position ON slices
		BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
	_, _, err := l.ReorderSlice(context.Background(), wholeShape, "writes", "reads", true)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("err = %v, want the file named", err)
	}
}

func TestLocalReorderSliceNamesItsFileWhenAReadFails(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct{ bad, id, target string }{
		"the target's position":  {"writes", "reads", "writes"},
		"a neighbour's position": {"reads", "stray", "writes"},
		"the dirty flag's table": {"", "stray", "design"},
	} {
		l, path := openPlan(t)
		fillPlan(t, l)
		if tc.bad != "" {
			write(t, l, `ALTER TABLE slices RENAME TO slices_data`)
			cols := columnsOf(t, l, "slices_data")
			shown := strings.ReplaceAll(cols, "position",
				`CASE WHEN id = '`+tc.bad+`' THEN abs(-9223372036854775808) ELSE position END`)
			write(t, l, `CREATE VIEW slices (`+cols+`) AS SELECT `+shown+` FROM slices_data`)
		} else {
			write(t, l, `DROP TABLE sync`)
		}
		_, _, err := l.ReorderSlice(ctx, wholeShape, tc.id, tc.target, true)
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Errorf("%s: err = %v, want the file named", name, err)
		}
	}
}

func TestMirroredReorderWithinAMilestoneSendsNothingAndSetsNoFlag(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()

	if _, _, err := m.ReorderSlice(ctx, wholeShape, "writes", "reads", true); err != nil {
		t.Fatalf("ReorderSlice: %v", err)
	}
	if len(api.calls) != 0 {
		t.Errorf("calls = %v, want no request spent", api.calls)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want no flag for a reorder the workspace cannot hold")
	}
	if got := positionOf(t, l, "writes"); got != -1 {
		t.Errorf("position = %v, want the file to hold the order", got)
	}
}

func pageUnder(milestone string) *notion.Page {
	p := slicePage("x", "Slice", notion.SliceTodo)
	p.Properties[notion.PropMilestone] = notion.PropertyValue{Type: notion.TypeSelect, Select: &notion.SelectOption{Name: milestone}}
	return p
}

func TestMirroredReorderAcrossMilestonesPushesTheRefile(t *testing.T) {
	api := &fakeAPI{page: func(id string) (*notion.Page, error) {
		p := pageUnder("M1: The format")
		p.ID = id
		if id == "stray" {
			p = slicePage(id, "Stray", notion.SliceTodo)
		}
		return p, nil
	}}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh := Shape{Milestones: []domain.Milestone{{ID: "M1: The format", Name: "M1: The format", SelectType: notion.TypeSelect}}}

	if _, _, err := m.ReorderSlice(ctx, sh, "stray", "design", false); err != nil {
		t.Fatalf("ReorderSlice: %v", err)
	}
	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want the refile pushed", api.updates)
	}
	if got := api.updates[0][notion.PropMilestone].Select; got == nil || got.Name != "M1: The format" {
		t.Errorf("milestone written = %+v, want M1", api.updates[0])
	}
	if dirty, _ := l.Dirty(ctx, "stray"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestMirroredReorderPushFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{
		page: func(id string) (*notion.Page, error) {
			if id == "stray" {
				return slicePage(id, "Stray", notion.SliceTodo), nil
			}
			return pageUnder("M1: The format"), nil
		},
		updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) { return nil, errBoom },
	}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	if _, _, err := m.ReorderSlice(ctx, Shape{}, "stray", "design", false); err != nil {
		t.Fatalf("ReorderSlice: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "stray"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestMirroredReorderCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if _, _, err := m.ReorderSlice(context.Background(), Shape{}, "ghost", "other", true); err == nil {
		t.Error("ReorderSlice on a slice not in the plan: want an error")
	}
	fillPlan(t, l)
	stampHydrated(t, l)
	if _, _, err := m.ReorderSlice(context.Background(), Shape{}, "reads", "reads", true); !errors.Is(err, errReorderSelf) {
		t.Errorf("err = %v, want a slice as its own target refused", err)
	}
}

func TestNotionReorderSliceIsTheRefileAlone(t *testing.T) {
	pages := map[string]*notion.Page{"a": pageUnder("M1"), "b": pageUnder("M2")}
	api := &fakeAPI{page: func(id string) (*notion.Page, error) { return pages[id], nil }}
	sh := Shape{Milestones: []domain.Milestone{{ID: "M2", Name: "M2", SelectType: notion.TypeSelect}}}
	ctx := context.Background()

	moved, to, err := Over(api).ReorderSlice(ctx, sh, "a", "b", true)
	if err != nil {
		t.Fatalf("ReorderSlice: %v", err)
	}
	if moved.MilestoneID != "M2" || to.MilestoneID != "M2" {
		t.Errorf("answered %+v beside %+v, want the slice refiled under M2", moved, to)
	}
	want := domain.Milestone{ID: "M2", Name: "M2", SelectType: notion.TypeSelect}.Ref()
	if len(api.updates) != 1 || api.updates[0][notion.PropMilestone].Select.Name != want.Select.Name || len(api.updates[0]) != 1 {
		t.Errorf("updates = %+v, want the Milestone column alone", api.updates)
	}

	// A milestone the shape does not list is written by its own name.
	api.updates = nil
	if _, _, err := Over(api).ReorderSlice(ctx, Shape{}, "b", "a", false); err != nil {
		t.Fatalf("ReorderSlice: %v", err)
	}
	if len(api.updates) != 1 {
		t.Errorf("updates = %+v, want the refile written", api.updates)
	}

	// Same milestone: both pages read, no request written.
	api.updates, api.calls = nil, nil
	pages["a"] = pageUnder("M2")
	if _, _, err := Over(api).ReorderSlice(ctx, sh, "a", "b", true); err != nil {
		t.Fatalf("ReorderSlice: %v", err)
	}
	if len(api.updates) != 0 || len(api.calls) != 2 {
		t.Errorf("updates = %+v, calls = %v, want two reads and nothing written", api.updates, api.calls)
	}
}

func TestNotionReorderSliceRefusals(t *testing.T) {
	ctx := context.Background()
	pages := map[string]*notion.Page{"a": pageUnder("M1"), "none": slicePage("none", "Loose", notion.SliceTodo)}
	fail := map[string]bool{}
	api := &fakeAPI{
		page: func(id string) (*notion.Page, error) {
			if fail[id] {
				return nil, errBoom
			}
			return pages[id], nil
		},
		updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) { return nil, errBoom },
	}
	n := Over(api)
	if _, _, err := n.ReorderSlice(ctx, Shape{}, "a", "a", true); !errors.Is(err, errReorderSelf) {
		t.Errorf("err = %v, want the slice as its own target refused", err)
	}
	if _, _, err := n.ReorderSlice(ctx, Shape{}, "a", "none", true); err == nil {
		t.Error("want a refile to no milestone refused")
	}
	fail["a"] = true
	if _, _, err := n.ReorderSlice(ctx, Shape{}, "a", "none", true); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the slice's read failure", err)
	}
	fail["a"], fail["none"] = false, true
	if _, _, err := n.ReorderSlice(ctx, Shape{}, "a", "none", true); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the target's read failure", err)
	}
	fail["none"] = false
	pages["b"] = pageUnder("M2")
	if _, _, err := n.ReorderSlice(ctx, Shape{}, "a", "b", true); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the refile's failure", err)
	}
}
