package source

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestDemoPluginOverTheWire runs examples/nat-source-demo — the reference
// plugin — through a real Exec: it describes with no project and no setup
// fields, and refuses setup in its own words. Skipped where there is no
// python3 to run it.
func TestDemoPluginOverTheWire(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3 to run the demo plugin")
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	path, err := filepath.Abs(filepath.Join("..", "..", "examples", "nat-source-demo", "nat-source-demo"))
	if err != nil {
		t.Fatal(err)
	}
	demo := New("demo", path)
	ctx := context.Background()

	d, err := demo.Describe(ctx, Project{})
	if err != nil || d.Name != "demo" || len(d.Setup) != 0 {
		t.Errorf("Describe() = %+v, %v, want the demo with no setup fields", d, err)
	}
	if _, err := demo.Setup(ctx, "token", "anything"); err == nil || err.Error() != "nat-source-demo setup: demo: nothing to set up" {
		t.Errorf("Setup() = %v, want the demo's refusal", err)
	}
}
