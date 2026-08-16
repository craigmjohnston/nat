package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Presence is how an agent that is running is getting on: whether it is working
// away or has stopped and is waiting to be told something. It says nothing
// about whether the agent is there at all — that is the live map's answer, and
// a slice with no session in it has no star whatever this says.
//
// The zero value is the honest answer for an agent nobody has classified yet:
// it is running, so it is drawn as working. That is what every live agent reads
// as until the activity watcher lands and starts refining it.
type Presence int

const (
	// PresenceUnknown is a live agent whose state has not been read.
	PresenceUnknown Presence = iota
	// PresenceWorking is an agent getting on with the slice.
	PresenceWorking
	// PresenceWaiting is one that has stopped and needs input.
	PresenceWaiting
)

// The star a slice with an agent on it is marked with. Working, it pulses
// through pulseFrames — the same star swelling and settling — and waiting, it
// holds waitingStar steady.
//
// The animation is in the glyph as well as the colour because a selected row is
// drawn without either chip's styling (see [paint]), and a star that pulsed in
// colour alone would go still exactly when the cursor was on it. Every frame is
// one cell wide, so the row's layout does not shift under the animation and the
// star sheds and wraps on a narrow board like any other chip.
var pulseFrames = [...]string{"✦", "✶", "✳", "✶"}

// waitingStar is the steady mark of an agent that has stopped for input. It is
// a star of its own rather than a held pulse frame, so the two states still
// read apart on a selected row, where the colour that separates them is gone.
const waitingStar = "✻"

// pulseInterval is how long each frame of the pulse is on screen. One full
// swell takes len(pulseFrames) of them — slow enough to read as breathing
// rather than flicker, and cheap: one timer for the whole board, however many
// agents are running.
const pulseInterval = 450 * time.Millisecond

// pulseTick is held as a variable so the tests can pin the animation quiet
// rather than wait out real frames.
var pulseTick = defaultPulseTick

// defaultPulseTick schedules the next frame of the pulse.
func defaultPulseTick() tea.Cmd { return tea.Tick(pulseInterval, pulseTicked) }

// pulseTicked turns the timer going off into the prod to advance the pulse.
func pulseTicked(time.Time) tea.Msg { return pulseTickMsg{} }

// pulseTickMsg is that prod: the board's one animation frame.
type pulseTickMsg struct{}

// SetActivity records how the running agents are getting on, keyed by slice ID.
// It refines the live map rather than replacing it: a slice with no session is
// not drawn whatever is in here, and a live one this does not mention is drawn
// as working.
func (b *Board) SetActivity(activity map[string]Presence) { b.activity = activity }

// Pulse advances the star animation to the given frame. The frame counter is
// the root model's — one for the whole board — and the board only draws what it
// is given.
func (b *Board) Pulse(frame int) { b.pulse = frame }

// presence is how the agent on a slice is getting on, and whether there is one
// at all. Liveness comes first: an agent that has gone has no state left worth
// drawing, whatever the last classification of it said.
func (b Board) presence(sliceID string) (Presence, bool) {
	if b.live[sliceID] == "" {
		return PresenceUnknown, false
	}
	return b.activity[sliceID], true
}

// Pulsing reports whether any slice on the board is drawn with a moving star,
// which is what the root model runs the frame timer for. A board with nothing
// working on it costs no timer at all.
func (b Board) Pulsing() bool {
	for id := range b.live {
		if p, live := b.presence(id); live && p != PresenceWaiting {
			return true
		}
	}
	return false
}

// star is the chip a slice with an agent on it leads its chips with, and
// whether it has one at all. It is its own glyph rather than a status: a
// session is running or not, which is a different question from where the slice
// has got to.
func (b Board) star(sliceID string, selected bool) (string, bool) {
	p, live := b.presence(sliceID)
	if !live {
		return "", false
	}
	if p == PresenceWaiting {
		return paint(selected, b.styles.StarWaiting, waitingStar), true
	}
	frame := ((b.pulse % len(pulseFrames)) + len(pulseFrames)) % len(pulseFrames)
	return paint(selected, b.styles.starPulse(frame), pulseFrames[frame]), true
}

// startPulse arms the frame timer, if the board has a star that moves and no
// timer is running already. Exactly one runs at a time, however many agents
// there are and however often the live read lands.
func (a *App) startPulse() tea.Cmd {
	if a.pulsing || !a.board.Pulsing() {
		return nil
	}
	a.pulsing = true
	return pulseTick()
}

// pulsed advances the animation a frame and schedules the next, or lets the
// timer stop where the board has nothing left pulsing — the next live read
// arms it again. The board is re-synced because its rows are drawn into a
// viewport and cached there: a frame that is not synced never reaches the
// screen.
func (a *App) pulsed() tea.Cmd {
	if !a.board.Pulsing() {
		a.pulsing = false
		return nil
	}
	a.pulse++
	a.board.Pulse(a.pulse)
	a.syncBoard()
	return pulseTick()
}

// starPulse is the style of one frame of the pulse, brightening with the star
// as it swells and settling back with it.
func (s Styles) starPulse(frame int) lipgloss.Style {
	switch frame {
	case 2:
		return s.StarPeak
	case 0:
		return s.StarDim
	}
	return s.StarMid
}
