package tui

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
)

// The board's own binding: shift+enter breaks the line in a multiline field,
// alongside the two huh already ships for terminals that report no modifier on
// enter. Enter is not among them — it still submits.
func TestFormKeyMapBreaksTheLineOnShiftEnter(t *testing.T) {
	k := formKeyMap()
	want := []string{"shift+enter", "alt+enter", "ctrl+j"}
	if got := k.Text.NewLine.Keys(); !reflect.DeepEqual(got, want) {
		t.Errorf("newline keys = %q, want %q", got, want)
	}
	for name, binding := range map[string]key.Binding{"submit": k.Text.Submit, "next": k.Text.Next} {
		if key.Matches(keyPress("shift+enter"), binding) {
			t.Errorf("shift+enter should not %s", name)
		}
		if !key.Matches(keyPress("enter"), binding) {
			t.Errorf("enter should still %s", name)
		}
	}
}

// The workshop prompt behind w: shift+enter adds a line and leaves the form
// open, and the enter that follows submits it, so the whole request — line
// break included — rides into the agent's prompt.
func TestAppPlanPromptBreaksTheLineOnShiftEnter(t *testing.T) {
	app, launcher, _ := launchApp(t)

	feed(t, app, press(app, "w"))
	typeText(app, "split the reporting milestone")
	feed(t, app, press(app, "shift+enter"))
	if app.form == nil {
		t.Fatal("shift+enter submitted the form, want a newline")
	}
	typeText(app, "and slim the first slice down")
	feed(t, app, press(app, "enter")) // past the request
	feed(t, app, press(app, "enter")) // past the model
	drive(t, app, press(app, "enter"))

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	prompt, err := os.ReadFile(launcher.launches[0].promptFile)
	if err != nil {
		t.Fatalf("read the prompt file: %v", err)
	}
	want := "split the reporting milestone\nand slim the first slice down"
	if !strings.Contains(string(prompt), want) {
		t.Errorf("the prompt is missing the two lines:\n%s", prompt)
	}
}

// The slice form's brief, the other multiline field a slice is written from:
// the same key breaks its line, and the break survives into the page body.
func TestAppAddSliceBriefBreaksTheLineOnShiftEnter(t *testing.T) {
	client := &fakeNotion{}
	app := newWriteApp(client)
	app.board.cursor = rowActiveMilestone

	feed(t, app, press(app, "a"))
	typeText(app, "New slice")
	feed(t, app, press(app, "enter"))
	typeText(app, "The brief.")
	feed(t, app, press(app, "shift+enter"))
	if app.form == nil {
		t.Fatal("shift+enter submitted the form, want a newline")
	}
	typeText(app, "The rest of it.")
	feed(t, app, press(app, "tab"))
	finishForm(t, app, press(app, "enter"))

	if len(client.created) != 1 {
		t.Fatalf("created %d pages, want the slice written", len(client.created))
	}
	want := []string{"The brief.\nThe rest of it."}
	if got := paragraphTexts(t, client.created[0].children); !reflect.DeepEqual(got, want) {
		t.Errorf("body = %q, want the two lines that were typed", got)
	}
}

// The new-project form's info blurb, the third multiline field on the board.
func TestAppNewProjectInfoBreaksTheLineOnShiftEnter(t *testing.T) {
	capturedConfig(t)
	client := creatingClient()
	app := newProjectApp(client)

	feed(t, app, press(app, "N"))
	typeText(app, "tracker two")
	feed(t, app, press(app, "enter"))
	typeText(app, "The conventions.")
	feed(t, app, press(app, "shift+enter"))
	if app.form == nil {
		t.Fatal("shift+enter submitted the form, want a newline")
	}
	typeText(app, "And the rest.")
	feed(t, app, press(app, "tab"))
	typeText(app, t.TempDir())
	feed(t, app, press(app, "enter"))
	feed(t, app, press(app, "n"))
	finishForm(t, app, press(app, "enter"))

	if len(client.appended) != 1 {
		t.Fatalf("appended %+v, want the blurb written once", client.appended)
	}
	want := []string{"The conventions.\nAnd the rest."}
	if got := paragraphTexts(t, client.appended[0].children); !reflect.DeepEqual(got, want) {
		t.Errorf("body = %q, want the two lines that were typed", got)
	}
}
