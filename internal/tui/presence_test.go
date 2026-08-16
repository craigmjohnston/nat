package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/agent"
)

// testPulseFrame is the frame a test pulses to when it means to find the star
// by its glyph alone: the pulse begins and ends on a middle dot, which is also
// the board's unknown-status glyph and the separator in its aggregates, so
// looking for frame 0 in a view would find one whether or not a star was drawn.
// The middle of the cycle is the full sparkle, which nothing else draws.
const testPulseFrame = len(pulseFrames) / 2

// liveBoard is a board with an agent on the claimed slice and one working a
// slice of another project entirely, which is the state the star tests draw.
func liveBoard(activity map[string]Presence) *Board {
	b := newTestBoard()
	b.SetLive(map[string]string{
		"s4":            agent.SessionName("s4"),
		"another-slice": "nat-deadbeef",
	})
	b.SetActivity(activity)
	return b
}

// TestBoardStarsALiveAgentThroughThePulse pins each frame of the animation on
// the board: the star swells and settles on the row with an agent on it, and
// no other row grows one.
func TestBoardStarsALiveAgentThroughThePulse(t *testing.T) {
	for frame, glyph := range pulseFrames {
		b := liveBoard(nil)
		b.Pulse(frame)
		view := b.View()
		golden(t, fmt.Sprintf("board-pulse-%d", frame), view)
		if !strings.Contains(stripANSI(view), "Board screen "+glyph) {
			t.Errorf("frame %d does not star the live slice with %q:\n%s", frame, glyph, view)
		}
		if strings.Contains(stripANSI(view), "Info view "+glyph) {
			t.Errorf("frame %d stars a slice with no agent on it:\n%s", frame, view)
		}
	}
}

// TestBoardHoldsAWaitingStarSteady pins the other state: an agent that has
// stopped for input keeps its own star whatever frame the pulse is on, so it
// reads as attention rather than motion.
func TestBoardHoldsAWaitingStarSteady(t *testing.T) {
	golden(t, "board-star-waiting", liveBoard(map[string]Presence{"s4": PresenceWaiting}).View())

	for frame := range pulseFrames {
		b := liveBoard(map[string]Presence{"s4": PresenceWaiting})
		b.Pulse(frame)
		if !strings.Contains(stripANSI(b.View()), "Board screen "+waitingStar) {
			t.Errorf("frame %d moved the waiting star:\n%s", frame, b.View())
		}
	}
}

// TestBoardDrawsNoStarWithoutASession pins the third state: an agent that has
// gone leaves no mark, whatever the last reading of it said.
func TestBoardDrawsNoStarWithoutASession(t *testing.T) {
	b := newTestBoard()
	// A classification with no session behind it any more.
	b.SetActivity(map[string]Presence{"s4": PresenceWaiting})

	// The star is looked for where it would be drawn rather than anywhere in
	// the view: the dot the pulse starts and ends on is also the board's own
	// unknown-status glyph and its aggregate separator, so a bare containment
	// check would find one on a board with no agent at all.
	view := stripANSI(b.View())
	for _, glyph := range append(pulseFrames[:], waitingStar) {
		if strings.Contains(view, "Board screen "+glyph) {
			t.Errorf("a slice with no session is starred with %q:\n%s", glyph, view)
		}
	}
}

// TestStarWrapsWithTheRestOfANarrowRow pins the star's share of the row
// narrowing: it is a chip like any other, so it takes a continuation line
// rather than being dropped, and the row never outgrows the board.
func TestStarWrapsWithTheRestOfANarrowRow(t *testing.T) {
	for _, tc := range []struct {
		name     string
		activity map[string]Presence
		glyph    string
	}{
		{"working", nil, pulseFrames[testPulseFrame]},
		{"waiting", map[string]Presence{"s1": PresenceWaiting}, waitingStar},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for width := 1; width <= 80; width++ {
				b := newLongRowBoard(width)
				b.SetActivity(tc.activity)
				view := b.View()
				for _, line := range strings.Split(view, "\n") {
					if got := lipgloss.Width(line); got > width {
						t.Fatalf("at %d the line %q is %d wide", width, line, got)
					}
				}
				// Below the width the row is cut to, the star goes with
				// everything else; above it, nothing is dropped.
				if width >= 40 && !strings.Contains(stripANSI(view), tc.glyph) {
					t.Errorf("at %d the star is gone:\n%s", width, view)
				}
			}
			b := newLongRowBoard(40)
			b.SetActivity(tc.activity)
			golden(t, "board-star-narrow-"+tc.name, b.View())
		})
	}
}

// TestStarFramesAreOneCellWide pins what keeps the animation from shifting the
// row under itself: every state of the star takes exactly one column, so the
// chips beside it do not move as it pulses.
func TestStarFramesAreOneCellWide(t *testing.T) {
	for _, glyph := range append(pulseFrames[:], waitingStar) {
		if got := lipgloss.Width(glyph); got != 1 {
			t.Errorf("the star %q is %d cells wide, want 1", glyph, got)
		}
	}
}

// TestBoardCyclesThePulse pins the frame arithmetic: the counter runs on
// without bound and the star cycles under it, and a counter that has gone
// negative still lands on a frame rather than panicking.
func TestBoardCyclesThePulse(t *testing.T) {
	b := liveBoard(nil)
	for _, frame := range []int{0, 1, 2, 3, 4, 5, -1, -4} {
		b.Pulse(frame)
		star, live := b.star("s4", false)
		if !live {
			t.Fatalf("frame %d lost the live agent", frame)
		}
		want := pulseFrames[((frame%len(pulseFrames))+len(pulseFrames))%len(pulseFrames)]
		if got := stripANSI(star); got != want {
			t.Errorf("frame %d = %q, want %q", frame, got, want)
		}
	}
}

// TestBoardStarsASelectedRowToo pins the one case the colours are no help in:
// a selected row is drawn without any chip's styling, so the star has to say
// which state it is in by its glyph alone.
func TestBoardStarsASelectedRowToo(t *testing.T) {
	b := liveBoard(nil)
	working, live := b.star("s4", true)
	if !live || working != pulseFrames[0] {
		t.Errorf("selected working star = %q, want the bare frame %q", working, pulseFrames[0])
	}
	b.SetActivity(map[string]Presence{"s4": PresenceWaiting})
	waiting, live := b.star("s4", true)
	if !live || waiting != waitingStar {
		t.Errorf("selected waiting star = %q, want the bare %q", waiting, waitingStar)
	}
	if working == waiting {
		t.Error("the two states are indistinguishable on a selected row")
	}
}

// TestBoardPulsingReportsWhetherAStarMoves pins what the frame timer is run
// for: any live agent that is not waiting, and nothing else.
func TestBoardPulsingReportsWhetherAStarMoves(t *testing.T) {
	for _, tc := range []struct {
		name     string
		activity map[string]Presence
		want     bool
	}{
		{"unclassified agents are working", nil, true},
		{"a working agent", map[string]Presence{"s4": PresenceWorking}, true},
		{"one waiting, one unclassified", map[string]Presence{"s4": PresenceWaiting}, true},
		{"every agent waiting", map[string]Presence{
			"s4": PresenceWaiting, "another-slice": PresenceWaiting}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := liveBoard(tc.activity).Pulsing(); got != tc.want {
				t.Errorf("Pulsing() = %v, want %v", got, tc.want)
			}
		})
	}

	if newTestBoard().Pulsing() {
		t.Error("a board with no agent on it should want no frame timer")
	}
}

// TestAppRunsOneStarTimer pins the animation's cost: a live read arms the
// timer once however often it lands, each frame arms the next, and the timer
// stops itself as soon as nothing on the board is pulsing.
func TestAppRunsOneStarTimer(t *testing.T) {
	ticks := 0
	restore := pulseTick
	// The stub counts the arming without firing, so the frames this test
	// asks for are the only ones it gets.
	pulseTick = func() tea.Cmd { ticks++; return func() tea.Msg { return nil } }
	t.Cleanup(func() { pulseTick = restore })

	app, launcher, _ := launchApp(t)
	id, session := sliceAt(t, app, rowTodoSlice)
	launcher.live = map[string]string{id: session}

	feed(t, app, app.refreshLive())
	if ticks != 1 {
		t.Fatalf("a live read armed %d timers, want one", ticks)
	}
	// A second read of the same sessions finds the timer already running.
	feed(t, app, app.refreshLive())
	if ticks != 1 {
		t.Errorf("a second live read armed %d timers, want the one still running", ticks)
	}

	// Each frame advances the star and arms the next.
	if cmd := app.pulsed(); cmd == nil {
		t.Error("a frame with an agent still working should arm the next")
	}
	if app.pulse != 1 || app.board.pulse != 1 {
		t.Errorf("pulse = %d/%d, want the board advanced with the app", app.pulse, app.board.pulse)
	}

	// The agent goes, and the frame after it notices lets the timer stop.
	launcher.live = nil
	feed(t, app, app.refreshLive())
	if cmd := app.pulsed(); cmd != nil {
		t.Error("a board with nothing pulsing should let the timer stop")
	}
	if app.pulsing {
		t.Error("the timer should be marked stopped, so the next live read arms it")
	}

	// And a live read after that arms a fresh one.
	launcher.live = map[string]string{id: session}
	feed(t, app, app.refreshLive())
	if ticks != 3 {
		t.Errorf("armed %d timers in all, want the stopped one replaced", ticks)
	}
}

// TestTheRealPulseTimerIsThere pins the animation's own edge, the one the
// suite stands in for everywhere else: a real timer, and the frame it prods
// the app with when it goes off.
func TestTheRealPulseTimerIsThere(t *testing.T) {
	if defaultPulseTick() == nil {
		t.Error("the star should be animated on a timer")
	}
	if _, ok := pulseTicked(time.Time{}).(pulseTickMsg); !ok {
		t.Error("the timer going off should prod the app for a frame")
	}
}

// TestAppAdvancesTheStarOnATick pins the frame reaching the screen: the board's
// rows are drawn into a viewport and cached there, so a frame the root model
// does not sync never gets drawn.
func TestAppAdvancesTheStarOnATick(t *testing.T) {
	restore := pulseTick
	pulseTick = func() tea.Cmd { return func() tea.Msg { return nil } }
	t.Cleanup(func() { pulseTick = restore })

	app, launcher, _ := launchApp(t)
	id, session := sliceAt(t, app, rowTodoSlice)
	launcher.live = map[string]string{id: session}
	feed(t, app, app.refreshLive())

	if view := stripANSI(app.View().Content); !strings.Contains(view, "Info view "+pulseFrames[0]) {
		t.Fatalf("the star did not start at its first frame:\n%s", view)
	}
	if _, cmd := app.Update(pulseTickMsg{}); cmd == nil {
		t.Error("a frame should arm the next")
	}
	if view := stripANSI(app.View().Content); !strings.Contains(view, "Info view "+pulseFrames[1]) {
		t.Errorf("the tick did not reach the drawn board:\n%s", view)
	}
}

// TestAppForgetsTheActivityOfAnAgentThatHasGone pins that a reading cannot
// outlive its agent: the slice may well get another, and what the last one was
// doing says nothing about it.
func TestAppForgetsTheActivityOfAnAgentThatHasGone(t *testing.T) {
	app, launcher, _ := launchApp(t)
	id, session := sliceAt(t, app, rowTodoSlice)
	app.activity = map[string]Presence{id: PresenceWaiting, "gone": PresenceWaiting}
	launcher.live = map[string]string{id: session}

	feed(t, app, app.refreshLive())

	want := map[string]Presence{id: PresenceWaiting}
	if len(app.activity) != len(want) || app.activity[id] != PresenceWaiting {
		t.Errorf("activity = %v, want %v", app.activity, want)
	}
}
