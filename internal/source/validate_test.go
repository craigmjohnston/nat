package source

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidateDescribe(t *testing.T) {
	choice := []Action{{ID: "owner", Label: "Owner", Input: InputChoice}}
	for _, tt := range []struct {
		name string
		d    Describe
		want string
	}{
		{"one letter", Describe{Tag: "S"}, ""},
		{"three with digits", Describe{Tag: "SC9", Menu: []Action{{ID: "a", Input: InputChoice, Options: []string{"x"}}}}, ""},
		{"empty tag", Describe{Tag: ""}, `tag "" is not 1–3`},
		{"lower case", Describe{Tag: "sc"}, `tag "sc"`},
		{"too long", Describe{Tag: "SHRT"}, `tag "SHRT"`},
		{"choice with no options", Describe{Tag: "SC", Menu: choice}, `the source menu: choice action "owner" has no options`},
		{"a secret action", Describe{Tag: "SC", Menu: []Action{{ID: "tok", Input: InputSecret}}}, `the source menu: action "tok" asks for a secret`},
		{"setup fields", Describe{Tag: "SC", Setup: []SetupField{
			{ID: "token", Label: "API token", Input: InputSecret, Hint: "Settings"},
			{ID: "work-space2", Label: "Workspace", Input: InputText, Set: new(bool)},
		}}, ""},
		{"a setup id with upper case", Describe{Tag: "SC", Setup: []SetupField{{ID: "Token", Input: InputSecret}}}, `setup field id "Token" is not lower-case`},
		{"an empty setup id", Describe{Tag: "SC", Setup: []SetupField{{ID: "", Input: InputSecret}}}, `setup field id ""`},
		{"a repeated setup id", Describe{Tag: "SC", Setup: []SetupField{{ID: "t", Input: InputSecret}, {ID: "t", Input: InputText}}}, `setup field id "t" is used more than once`},
		{"a setup choice", Describe{Tag: "SC", Setup: []SetupField{{ID: "t", Input: InputChoice}}}, `setup field "t": input "choice" is not secret or text`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDescribe(tt.d)
			if tt.want == "" {
				if err != nil {
					t.Errorf("ValidateDescribe() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ValidateDescribe() = %v, want it to say %q", err, tt.want)
			}
		})
	}
}

func TestValidateGroups(t *testing.T) {
	ok := []Action{{ID: "refresh", Input: InputNone}}
	bad := []Action{{ID: "owner", Input: InputChoice}}
	for _, tt := range []struct {
		name   string
		groups []Group
		want   string
	}{
		{"a valid tree", []Group{
			{ID: "doing", Menu: ok, Containers: []Container{{ID: "c1", Menu: ok}}},
			{ID: "ready", Children: []Group{{ID: "mine", Containers: []Container{{ID: "c1"}}}}},
		}, ""},
		{"both children and containers", []Group{
			{ID: "g", Children: []Group{{ID: "c"}}, Containers: []Container{{ID: "c1"}}},
		}, `group "g" has both children and containers`},
		{"nested two levels", []Group{
			{ID: "g", Children: []Group{{ID: "c", Children: []Group{{ID: "cc"}}}}},
		}, `group "c" nests children more than one level deep`},
		{"a repeated group id", []Group{{ID: "g"}, {ID: "h", Children: []Group{{ID: "g"}}}}, `group id "g" is used more than once`},
		{"a reserved group id", []Group{{ID: "_unlisted"}}, `group id "_unlisted" starts with _`},
		{"an empty group id", []Group{{ID: ""}}, "a group has no id"},
		{"a reserved container id", []Group{{ID: "g", Containers: []Container{{ID: "_c"}}}}, `container id "_c" starts with _`},
		{"a bad group menu", []Group{{ID: "g", Menu: bad}}, `group "g"'s menu: choice action "owner" has no options`},
		{"a bad child", []Group{{ID: "g", Children: []Group{{ID: "_c"}}}}, `group id "_c"`},
		{"a bad container menu", []Group{{ID: "g", Containers: []Container{{ID: "c1", Menu: bad}}}}, `container "c1"'s menu`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGroups(tt.groups)
			if tt.want == "" {
				if err != nil {
					t.Errorf("ValidateGroups() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ValidateGroups() = %v, want it to say %q", err, tt.want)
			}
		})
	}
}

func TestValidateFilterActions(t *testing.T) {
	opts := []FilterOption{{ID: "a", Label: "A", Color: "#112233"}, {ID: "b", Label: "B"}}
	filter := func(fields ...FilterField) []Group {
		return []Group{{ID: "seg", Menu: []Action{{ID: "filter", Label: "Filter…", Input: InputFilter, Fields: fields}}}}
	}
	for _, tt := range []struct {
		name   string
		groups []Group
		want   string
	}{
		{"a valid filter", filter(
			FilterField{ID: "team", Label: "Team", Options: opts, Value: []string{"a"}},
			FilterField{ID: "labels", Label: "Labels", Multi: true, Options: opts, Value: []string{"a", "b"}},
			FilterField{ID: "epic", Label: "Epic", Options: nil, Value: nil},
		), ""},
		{"no fields", filter(), `group "seg"'s menu: filter action "filter": has no fields`},
		{"a field with no id", filter(FilterField{Options: opts}), "a field has no id"},
		{"a repeated field id", filter(FilterField{ID: "t"}, FilterField{ID: "t"}), `field id "t" is used more than once`},
		{"an option with no id", filter(FilterField{ID: "t", Options: []FilterOption{{Label: "x"}}}), `field "t": an option has no id`},
		{"a repeated option id", filter(FilterField{ID: "t", Options: []FilterOption{{ID: "a"}, {ID: "a"}}}), `option id "a" is used more than once`},
		{"two selected, not multi", filter(FilterField{ID: "t", Options: opts, Value: []string{"a", "b"}}), `field "t" selects 2 options but is not multi`},
		{"a selection not offered", filter(FilterField{ID: "t", Options: opts, Value: []string{"z"}}), `field "t" selects "z", which it does not offer`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGroups(tt.groups)
			if tt.want == "" {
				if err != nil {
					t.Errorf("ValidateGroups() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ValidateGroups() = %v, want it to say %q", err, tt.want)
			}
		})
	}
}

func TestValidateSidebar(t *testing.T) {
	if err := ValidateSidebar(Sidebar{Groups: []Group{{ID: "g"}}, Menu: []Action{{ID: "refresh", Input: InputNone}}}); err != nil {
		t.Errorf("ValidateSidebar() = %v, want nil", err)
	}
	if err := ValidateSidebar(Sidebar{Groups: []Group{{ID: ""}}}); err == nil || !strings.Contains(err.Error(), "a group has no id") {
		t.Errorf("ValidateSidebar() = %v, want the tree refused", err)
	}
	if err := ValidateSidebar(Sidebar{Menu: []Action{{ID: "filter", Input: InputFilter}}}); err == nil ||
		!strings.Contains(err.Error(), `the sidebar menu: filter action "filter": has no fields`) {
		t.Errorf("ValidateSidebar() = %v, want the menu refused", err)
	}
}

func TestValidateContainer(t *testing.T) {
	bad := Action{ID: "pick", Input: InputChoice}
	good := Action{ID: "comment", Input: InputText}
	if err := ValidateContainer(ContainerDetail{Sections: []Section{{ID: "s"}, {ID: "c", Composer: &good}}}); err != nil {
		t.Errorf("ValidateContainer() = %v, want nil", err)
	}
	if err := ValidateContainer(ContainerDetail{Menu: []Action{bad}}); err == nil || !strings.Contains(err.Error(), "the container menu") {
		t.Errorf("ValidateContainer() = %v, want the menu refused", err)
	}
	if err := ValidateContainer(ContainerDetail{Sections: []Section{{ID: "c", Composer: &bad}}}); err == nil || !strings.Contains(err.Error(), `section "c"'s composer`) {
		t.Errorf("ValidateContainer() = %v, want the composer refused", err)
	}
}

// TestExecRefusesInvalidResponses: each decoding method runs its validation,
// and the refusal names the plugin, the method and the rule.
func TestExecRefusesInvalidResponses(t *testing.T) {
	ctx := context.Background()
	for method, tt := range map[string]struct {
		out  string
		call func(*Exec) error
		want string
	}{
		"describe": {`{"protocol":1,"tag":"toolong"}`,
			func(e *Exec) error { _, err := e.Describe(ctx, testProject); return err },
			`nat-source-sc describe: invalid response: tag "toolong"`},
		"sidebar": {`{"groups":[{"id":"_x"}]}`,
			func(e *Exec) error { _, err := e.Sidebar(ctx, testProject, nil); return err },
			`nat-source-sc sidebar: invalid response: group id "_x"`},
		"container": {`{"id":"c1","menu":[{"id":"m","input":"choice"}]}`,
			func(e *Exec) error { _, err := e.Container(ctx, testProject, "c1"); return err },
			`nat-source-sc container: invalid response: the container menu`},
	} {
		err := tt.call(NewWithRunner("sc", "/bin/nat-source-sc", &fakeRunner{out: tt.out}))
		if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want %q", method, err, tt.want)
		}
	}
}

func TestExecRunnerCapsStdout(t *testing.T) {
	old := maxStdout
	maxStdout = 10
	t.Cleanup(func() { maxStdout = old })

	out, err := ExecRunner{}.Run(t.TempDir(), script(t, `printf 0123456789`))
	if err != nil || out != "0123456789" {
		t.Errorf("Run() at the cap = %q, %v, want it all", out, err)
	}
	_, err = ExecRunner{}.Run(t.TempDir(), script(t, `printf 0123456789A`))
	if err == nil || !strings.Contains(err.Error(), "wrote more than 10 bytes to stdout") {
		t.Errorf("Run() past the cap = %v, want it refused", err)
	}
}

func TestUnavailableAnswersItsErrorEverywhere(t *testing.T) {
	want := errors.New("no plugin")
	u := Unavailable{Err: want}
	ctx := context.Background()
	if _, err := u.Describe(ctx, Project{}); !errors.Is(err, want) {
		t.Errorf("Describe() = %v", err)
	}
	if _, err := u.Sidebar(ctx, Project{}, nil); !errors.Is(err, want) {
		t.Errorf("Sidebar() = %v", err)
	}
	if _, err := u.Container(ctx, Project{}, "c"); !errors.Is(err, want) {
		t.Errorf("Container() = %v", err)
	}
	if _, err := u.Action(ctx, Project{}, "a", Target{}, ""); !errors.Is(err, want) {
		t.Errorf("Action() = %v", err)
	}
	if err := u.Event(ctx, Project{}, "c", Task{}, EventCreated); !errors.Is(err, want) {
		t.Errorf("Event() = %v", err)
	}
	if _, err := u.Setup(ctx, "token", "x"); !errors.Is(err, want) {
		t.Errorf("Setup() = %v", err)
	}
}
