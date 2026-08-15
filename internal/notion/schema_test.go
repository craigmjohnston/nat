package notion

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSchemaBuilders(t *testing.T) {
	tests := []struct {
		name   string
		schema PropertySchema
		want   string
	}{
		{"title", SchemaTitle(), `{"title":{}}`},
		{"rich text", SchemaRichText(), `{"rich_text":{}}`},
		{"select", SchemaSelect("Queued", "Active"), `{"select":{"options":[{"name":"Queued"},{"name":"Active"}]}}`},
		{"select without options", SchemaSelect(), `{"select":{}}`},
		{"people", SchemaPeople(), `{"people":{}}`},
		{"url", SchemaURL(), `{"url":{}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.schema)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(b) != tt.want {
				t.Errorf("marshalled to %s, want %s", b, tt.want)
			}
		})
	}
}

func TestPropertySchemaOptions(t *testing.T) {
	tests := []struct {
		name   string
		schema PropertySchema
		want   []string
	}{
		{"select", SchemaSelect("Todo", "Done"), []string{"Todo", "Done"}},
		{"select with no options", SchemaSelect(), []string{}},
		{
			"a status column converted in the Notion UI",
			PropertySchema{Type: TypeStatus, Status: &OptionsConfig{Options: selectOptions([]string{"Todo", "In progress"})}},
			[]string{"Todo", "In progress"},
		},
		{"not a fixed-choice property", SchemaTitle(), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.schema.OptionNames()
			if len(got) != len(tt.want) {
				t.Fatalf("OptionNames() = %v, want %v", got, tt.want)
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("option %d = %q, want %q", i, got[i], w)
				}
			}
			if tt.want == nil && got != nil {
				t.Errorf("OptionNames() = %v, want nil for a property with no options", got)
			}
		})
	}
}

// AppendedOptions is what a milestone added to a plan kept on a select is
// written as: every option already there, exactly as it was read, and the new
// ones after them.
func TestPropertySchemaAppendedOptions(t *testing.T) {
	existing := PropertySchema{
		ID:   "mS",
		Name: PropMilestone,
		Type: TypeSelect,
		Select: &OptionsConfig{Options: []SelectOption{
			{ID: "o1", Name: "M1: Client", Color: "blue"},
			{ID: "o2", Name: "M2: Board", Color: "green"},
		}},
	}

	got, ok := existing.AppendedOptions("M3: Agents", "M4: Polish")

	if !ok {
		t.Fatal("AppendedOptions() reported a select it cannot add to")
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `{"select":{"options":[` +
		`{"id":"o1","name":"M1: Client","color":"blue"},` +
		`{"id":"o2","name":"M2: Board","color":"green"},` +
		`{"name":"M3: Agents"},{"name":"M4: Polish"}]}}`
	if string(b) != want {
		t.Errorf("marshalled to %s\nwant %s", b, want)
	}
	if len(existing.Select.Options) != 2 {
		t.Errorf("the property it was called on grew options: %+v", existing.Select.Options)
	}
}

// A column whose options cannot be written is refused rather than replaced by
// one holding only what was appended.
func TestPropertySchemaAppendedOptionsRefusesWhatItCannotWrite(t *testing.T) {
	tests := []struct {
		name   string
		schema PropertySchema
	}{
		{"a status column", PropertySchema{Type: TypeStatus, Status: &OptionsConfig{Options: selectOptions([]string{"Todo"})}}},
		{"a relation", PropertySchema{Type: "relation", Relation: &RelationConfig{DataSourceID: "ds-1"}}},
		{"a property that is not there at all", PropertySchema{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.schema.AppendedOptions("M4: Polish")

			if ok {
				t.Fatalf("AppendedOptions() = %+v, true; want it refused", got)
			}
			if !reflect.DeepEqual(got, PropertySchema{}) {
				t.Errorf("AppendedOptions() = %+v, want the zero schema", got)
			}
		})
	}
}

// An empty select is still a select: a plan whose first milestone is being
// added has no options to keep, and the write is the new one alone.
func TestPropertySchemaAppendedOptionsAddsToAnEmptySelect(t *testing.T) {
	got, ok := SchemaSelect().AppendedOptions("M1: Client")

	if !ok {
		t.Fatal("AppendedOptions() refused an empty select")
	}
	b, _ := json.Marshal(got)
	if want := `{"select":{"options":[{"name":"M1: Client"}]}}`; string(b) != want {
		t.Errorf("marshalled to %s, want %s", b, want)
	}
}
