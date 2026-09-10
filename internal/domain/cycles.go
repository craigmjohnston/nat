package domain

import (
	"sort"
	"strings"
)

// GraphCycles finds the cycles of a directed graph given as the edges out of
// each node, keyed however the caller keys its nodes: a plan's slices are
// keyed by page ID, and a document being validated keys the slices it has yet
// to create by their place in it, since they have no ID until they exist.
//
// Each cycle comes back as the keys around it in the order they point at one
// another, starting at the lowest of them, so the same cycle reads the same way
// whichever node it was found from and two runs over one graph report it
// identically. A node pointing at itself is a cycle of one, which is exactly
// what it is: a slice nothing could ever unblock.
//
// It is a depth-first walk, so what it reports is one cycle per back edge
// rather than every cycle a tangle holds — naming a way round is what tells
// somebody which edge to break, and enumerating every way round a strongly
// connected component is a list nobody reads. Only the nodes that are keys of
// the map are walked from: one that only ever appears as a target has no edges
// out and so can be in no cycle.
func GraphCycles(edges map[string][]string) [][]string {
	const (
		unvisited = iota
		open
		closed
	)
	state := map[string]int{}
	var stack []string
	var out [][]string
	seen := map[string]bool{}

	var visit func(string)
	visit = func(node string) {
		state[node] = open
		stack = append(stack, node)
		for _, next := range edges[node] {
			switch state[next] {
			case unvisited:
				visit(next)
			case open:
				// The walk has come back to a node it is still inside: the way
				// round is the stack from that node to this one.
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i] != next {
						continue
					}
					cycle := lowestFirst(stack[i:])
					if key := strings.Join(cycle, "\x00"); !seen[key] {
						seen[key] = true
						out = append(out, cycle)
					}
					break
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[node] = closed
	}

	nodes := make([]string, 0, len(edges))
	for node := range edges {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		if state[node] == unvisited {
			visit(node)
		}
	}
	return out
}

// lowestFirst turns one way round a cycle into the way round it: the same
// order, rotated to start at the lowest key. A cycle has no first node of its
// own, and two walks that met it from different sides would otherwise report
// the same tangle twice.
func lowestFirst(cycle []string) []string {
	at := 0
	for i, node := range cycle {
		if node < cycle[at] {
			at = i
		}
	}
	return RotateCycle(cycle, cycle[at])
}

// RotateCycle reads a cycle out from one particular node, which is how a
// refusal names it: the edge somebody just asked for is the one to start at,
// so what they did reads as the first step round. A cycle the node is not part
// of is handed back as it stands.
func RotateCycle(cycle []string, start string) []string {
	for i, node := range cycle {
		if node != start {
			continue
		}
		out := make([]string, 0, len(cycle))
		out = append(out, cycle[i:]...)
		return append(out, cycle[:i]...)
	}
	return cycle
}

// CyclePath reads a cycle out as the way round it: every name in order with the
// first said again at the end, so "A → B → A" says plainly that it closes.
func CyclePath(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.Join(append(append(make([]string, 0, len(names)+1), names...), names[0]), " → ")
}

// SliceNames is the names of some slices in the order they were given, which is
// what a cycle is read out as.
func SliceNames(slices []Slice) []string {
	names := make([]string, len(slices))
	for i, s := range slices {
		names[i] = s.Name
	}
	return names
}

// Cycles is the dependency cycles a plan holds: each one the slices around it,
// in the order they wait on one another. Every slice in such a cycle waits,
// through however many others, on itself, so no run of next-slice and no launch
// can ever unblock any of them — which reads as a row that is simply never
// workable unless somebody says it is a cycle.
//
// A dependency on a page the plan does not hold is passed over, exactly as
// [Blockers] passes over it: such a page cannot be read at all, and it has no
// dependencies here to lead anywhere.
func Cycles(slices []Slice) [][]Slice {
	byID := SlicesByID(slices)
	edges := make(map[string][]string, len(slices))
	for _, s := range slices {
		edges[NormaliseID(s.ID)] = nil
	}
	for _, s := range slices {
		key := NormaliseID(s.ID)
		for _, id := range s.DependsOn {
			dep := NormaliseID(id)
			if _, ok := byID[dep]; ok {
				edges[key] = append(edges[key], dep)
			}
		}
	}
	found := GraphCycles(edges)
	out := make([][]Slice, 0, len(found))
	for _, cycle := range found {
		members := make([]Slice, 0, len(cycle))
		for _, key := range cycle {
			members = append(members, byID[key])
		}
		out = append(out, members)
	}
	return out
}

// CycleIndex is those cycles by member: each slice in one, keyed the way
// [SlicesByID] keys them, mapped to the way round the cycle it is in, read out
// from that slice itself — so the slice a caller is looking at is the first
// step of what it is told. A slice in more than one cycle is reported the first
// one found, since what a refusal needs is an edge to break rather than every
// edge there is.
func CycleIndex(slices []Slice) map[string][]Slice {
	index := map[string][]Slice{}
	for _, cycle := range Cycles(slices) {
		for i, s := range cycle {
			key := NormaliseID(s.ID)
			if _, already := index[key]; already {
				continue
			}
			read := make([]Slice, 0, len(cycle))
			read = append(read, cycle[i:]...)
			index[key] = append(read, cycle[:i]...)
		}
	}
	return index
}
