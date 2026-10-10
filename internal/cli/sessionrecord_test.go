package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
)

// leaveRecord writes a session record for session, as the mod would have, and
// answers its path, under the state directory the test's config points at.
func leaveRecord(t *testing.T, session string) string {
	t.Helper()
	path, err := agent.SessionRecordPath(session)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"session_id":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// A released slice's next launch is a fresh claim: its record goes.
func TestReleaseSliceForgetsTheSession(t *testing.T) {
	env, _ := releaseEnv(t, releasableAPI())
	path := leaveRecord(t, agent.SessionName(sliceID))
	if err := Run(context.Background(), []string{"release-slice", sliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("release-slice: %v", err)
	}
	if exists(path) {
		t.Error("the released slice's session record is still there")
	}
}

// Killing a slice's agent keeps its record — the relaunch resumes it — where
// killing a planning or ad hoc session, which nothing relaunches, removes it.
func TestAgentKillKeepsOnlyASliceRecord(t *testing.T) {
	for _, tt := range []struct {
		name, tag, session string
		args               []string
		keep               bool
	}{
		{"slice", testSliceID, "nat-slice-kill", []string{"agent-kill", testSliceID, "--project", "project-1"}, true},
		{"planning", agent.PlanTag("project-1"), "nat-plan-kill", []string{"agent-kill", "--workshop", "--project", "project-1"}, false},
		{"ad hoc", agent.SessionTag("project-1", "s1"), "nat-session-kill", []string{"agent-kill", agent.SessionTag("project-1", "s1"), "--project", "project-1"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env, _ := testEnv(testClaimConfig(t), &fakeAPI{})
			path := leaveRecord(t, tt.session)
			runner := &agentTestRunner{liveSessions: map[string]string{tt.tag: tt.session}}
			env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
			if err := Run(context.Background(), tt.args, env); err != nil {
				t.Fatalf("agent-kill: %v", err)
			}
			if exists(path) != tt.keep {
				t.Errorf("record kept = %v, want %v", exists(path), tt.keep)
			}
		})
	}
}
