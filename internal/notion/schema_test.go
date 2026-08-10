package notion

import (
	"encoding/json"
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
		{"number", SchemaNumber(), `{"number":{}}`},
		{"select", SchemaSelect("Queued", "Active"), `{"select":{"options":[{"name":"Queued"},{"name":"Active"}]}}`},
		{"select without options", SchemaSelect(), `{"select":{}}`},
		{"relation", SchemaRelation("ds-1"), `{"relation":{"data_source_id":"ds-1","type":"single_property","single_property":{}}}`},
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
		{"not a select", SchemaTitle(), nil},
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
				t.Errorf("OptionNames() = %v, want nil for a non-select property", got)
			}
		})
	}
}
