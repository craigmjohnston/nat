package tui

import (
	"testing"
)

// startSelectiveLoad is now exactly startLoad(false): the edited-since query
// it used to run had no meaning against a plan file. These tests pin that
// fallback rather than the query it replaced.

func TestAppSelectiveLoadIsAFullLoad(t *testing.T) {
	client := newLoadingClient()
	app := NewApp(testConfig(t), client)

	cmd := app.startSelectiveLoad()
	if !app.loading {
		t.Error("startSelectiveLoad should behave exactly like a full load")
	}
	run(cmd)

	if client.fetchedDSs == nil {
		t.Error("the full load reads the schema; the fallback did not")
	}
}

func TestAppSelectiveLoadWithNoProjectOrClientDoesNothing(t *testing.T) {
	app := NewApp(testConfig(t), &fakeNotion{})
	app.cfg.ActiveProjectID = ""
	if cmd := app.startSelectiveLoad(); cmd != nil {
		t.Error("no active project, so nothing to load")
	}

	app = NewApp(testConfig(t), &fakeNotion{})
	app.client = nil
	if cmd := app.startSelectiveLoad(); cmd != nil {
		t.Error("no client, so nothing to load with")
	}
}
