package domain

import (
	"sort"
	"strings"
	"testing"
)

// cyclic builds a plan from an adjacency list, so a test says what waits on
// what and nothing else.
func cyclic(edges map[string][]string) []Slice {
	names := make([]string, 0, len(edges))
	for name := range edges {
		names = append(names, name)
	}
	// A map has no order, and the plan's is what the walk starts from — sorting
	// is what makes the fixture the same on every run.
	sort.Strings(names)
	slices := make([]Slice, 0, len(names))
	for _, name := range names {
		slices = append(slices, Slice{ID: "id-" + name, Name: name, DependsOn: prefixed(edges[name])})
	}
	return slices
}

func prefixed(names []string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = "id-" + name
	}
	return out
}

// paths reads every cycle out, so a test asserts on what a user would be told.
func paths(cycles [][]Slice) []string {
	out := make([]string, len(cycles))
	for i, c := range cycles {
		out[i] = CyclePath(SliceNames(c))
	}
	return out
}

// A plan whose dependencies all go one way holds no cycle, however deep it is.
func TestCyclesFindsNoneInADAG(t *testing.T) {
	plan := cyclic(map[string][]string{
		"A": {"B", "C"},
		"B": {"C"},
		"C": nil,
		"D": {"A"},
	})
	if got := Cycles(plan); len(got) != 0 {
		t.Errorf("Cycles = %v, want none", paths(got))
	}
	if got := CycleIndex(plan); len(got) != 0 {
		t.Errorf("CycleIndex = %v, want empty", got)
	}
}

// The plainest cycle there is: two slices each waiting on the other.
func TestCyclesFindsADirectCycle(t *testing.T) {
	plan := cyclic(map[string][]string{"A": {"B"}, "B": {"A"}})

	got := paths(Cycles(plan))
	if len(got) != 1 || got[0] != "A → B → A" {
		t.Fatalf("Cycles = %v, want one A → B → A", got)
	}
}

// A cycle closed through however many others is found the same way, and read out
// from whichever slice asked — which is what tells that slice which dependency
// is its own.
func TestCyclesFindsATransitiveCycle(t *testing.T) {
	plan := cyclic(map[string][]string{"A": {"B"}, "B": {"C"}, "C": {"A"}})

	got := paths(Cycles(plan))
	if len(got) != 1 || got[0] != "A → B → C → A" {
		t.Fatalf("Cycles = %v, want one A → B → C → A", got)
	}
	index := CycleIndex(plan)
	for _, want := range []struct{ id, path string }{
		{"id-A", "A → B → C → A"},
		{"id-B", "B → C → A → B"},
		{"id-C", "C → A → B → C"},
	} {
		if got := CyclePath(SliceNames(index[NormaliseID(want.id)])); got != want.path {
			t.Errorf("CycleIndex[%s] = %q, want %q", want.id, got, want.path)
		}
	}
}

// A slice waiting on itself is a cycle of one, and reads as one.
func TestCyclesFindsASelfCycle(t *testing.T) {
	plan := cyclic(map[string][]string{"A": {"A"}, "B": {"A"}})

	got := paths(Cycles(plan))
	if len(got) != 1 || got[0] != "A → A" {
		t.Fatalf("Cycles = %v, want one A → A", got)
	}
	if _, in := CycleIndex(plan)[NormaliseID("id-B")]; in {
		t.Error("CycleIndex holds B, which merely waits on a slice in a cycle")
	}
}

// Two cycles are two answers, each read from its lowest member so the pair reads
// the same on every run.
func TestCyclesFindsEveryCycle(t *testing.T) {
	plan := cyclic(map[string][]string{
		"A": {"B"}, "B": {"A"},
		"C": {"D"}, "D": {"C"},
	})

	got := paths(Cycles(plan))
	if len(got) != 2 || got[0] != "A → B → A" || got[1] != "C → D → C" {
		t.Fatalf("Cycles = %v, want A → B → A and C → D → C", got)
	}
}

// A slice in more than one cycle is reported the first found: what a refusal
// needs is a dependency to drop, not every way round the tangle.
func TestCycleIndexReportsOneCyclePerSlice(t *testing.T) {
	plan := cyclic(map[string][]string{"A": {"B", "C"}, "B": {"A"}, "C": {"A"}})

	index := CycleIndex(plan)
	got := CyclePath(SliceNames(index[NormaliseID("id-A")]))
	if got != "A → B → A" && got != "A → C → A" {
		t.Errorf("CycleIndex[A] = %q, want one way round", got)
	}
	if len(index) != 3 {
		t.Errorf("CycleIndex covers %d slices, want all three", len(index))
	}
}

// A cycle the walk reaches partway round is still read out from its lowest
// member, so two plans that differ only in which slice was written first report
// the same tangle in the same words.
func TestCyclesReadsFromTheLowestMember(t *testing.T) {
	plan := cyclic(map[string][]string{
		"A": {"C"},
		"B": {"C"},
		"C": {"B"},
	})

	got := paths(Cycles(plan))
	if len(got) != 1 || got[0] != "B → C → B" {
		t.Fatalf("Cycles = %v, want one B → C → B", got)
	}
}

// A dependency on a page the plan does not hold leads nowhere: it has no
// dependencies here, exactly as Blockers passes over it.
func TestCyclesPassesOverAnUnreadableDependency(t *testing.T) {
	plan := []Slice{{ID: "id-A", Name: "A", DependsOn: []string{"id-gone"}}}
	if got := Cycles(plan); len(got) != 0 {
		t.Errorf("Cycles = %v, want none", paths(got))
	}
}

// The graph is keyed however the caller keys it, which is what lets a document
// being validated mix slices it has yet to create with slices already filed.
func TestGraphCyclesReadsAnyKeys(t *testing.T) {
	got := GraphCycles(map[string][]string{
		"plan:0": {"abc"},
		"abc":    {"plan:0"},
		"loose":  {"nowhere"},
	})
	if len(got) != 1 {
		t.Fatalf("GraphCycles = %v, want one cycle", got)
	}
	if strings.Join(got[0], ",") != "abc,plan:0" {
		t.Errorf("cycle = %v, want it to start at the lowest key", got[0])
	}
}

// A cycle read out from a node it does not hold is handed back as it stands:
// there is nowhere else to start.
func TestRotateCycleLeavesAStrangerAlone(t *testing.T) {
	cycle := []string{"a", "b"}
	if got := RotateCycle(cycle, "z"); strings.Join(got, ",") != "a,b" {
		t.Errorf("RotateCycle = %v, want it unchanged", got)
	}
}

// Nothing to read out is nothing at all, rather than a stray arrow.
func TestCyclePathOfNothing(t *testing.T) {
	if got := CyclePath(nil); got != "" {
		t.Errorf("CyclePath(nil) = %q, want empty", got)
	}
}
