package domain

import (
	"testing"
	"time"
)

func TestSessionEnded(t *testing.T) {
	s := Session{}
	if s.Ended() {
		t.Error("a zero-value session reads as ended")
	}
	s.EndedAt = time.Now()
	if !s.Ended() {
		t.Error("a session with an end time reads as not ended")
	}
}
