package notion

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPlainText(t *testing.T) {
	tests := []struct {
		name  string
		spans []RichText
		want  string
	}{
		{"empty", nil, ""},
		{"single span", []RichText{{PlainText: "hello"}}, "hello"},
		{"joined spans", []RichText{{PlainText: "hello "}, {PlainText: "world"}}, "hello world"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PlainText(tt.spans); got != tt.want {
				t.Errorf("PlainText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPropertyValueJSON(t *testing.T) {
	tests := []struct {
		name  string
		value PropertyValue
		want  string
	}{
		{
			"title",
			NewTitle("Slice name"),
			`{"title":[{"type":"text","text":{"content":"Slice name"}}]}`,
		},
		{
			"rich text",
			NewRichText("/repo/path"),
			`{"rich_text":[{"type":"text","text":{"content":"/repo/path"}}]}`,
		},
		{"select", NewSelect("Active"), `{"select":{"name":"Active"}}`},
		{"status", NewStatus("Active"), `{"status":{"name":"Active"}}`},
		{"people", NewPeople("user-1"), `{"people":[{"id":"user-1"}]}`},
		{"url", NewURL("https://example.test/pr/1"), `{"url":"https://example.test/pr/1"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("marshalled to %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNewChoiceMatchesThePropertyType(t *testing.T) {
	tests := []struct {
		propertyType string
		want         PropertyValue
	}{
		{TypeSelect, NewSelect("Done")},
		{TypeStatus, NewStatus("Done")},
		// A property whose type is unknown — or absent, as it is on a value the
		// app built itself — is written the way this app's schemas are made.
		{"", NewSelect("Done")},
		{"rich_text", NewSelect("Done")},
	}
	for _, tt := range tests {
		t.Run(tt.propertyType, func(t *testing.T) {
			if got := NewChoice(tt.propertyType, "Done"); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewChoice(%q, \"Done\") = %+v, want %+v", tt.propertyType, got, tt.want)
			}
		})
	}
}

func TestPropertyValueAccessors(t *testing.T) {
	t.Run("Text reads a title", func(t *testing.T) {
		p := PropertyValue{Title: []RichText{{PlainText: "Slice"}, {PlainText: " name"}}}
		if got := p.Text(); got != "Slice name" {
			t.Errorf("Text() = %q", got)
		}
	})

	t.Run("Text reads rich text", func(t *testing.T) {
		p := PropertyValue{RichText: []RichText{{PlainText: "/repo"}}}
		if got := p.Text(); got != "/repo" {
			t.Errorf("Text() = %q", got)
		}
	})

	t.Run("Text of another property type is empty", func(t *testing.T) {
		if got := NewSelect("Todo").Text(); got != "" {
			t.Errorf("Text() = %q, want empty", got)
		}
	})

	t.Run("SelectName reads a select", func(t *testing.T) {
		if got := (PropertyValue{Select: &SelectOption{Name: "Active"}}).SelectName(); got != "Active" {
			t.Errorf("SelectName() = %q", got)
		}
		if got := (PropertyValue{}).SelectName(); got != "" {
			t.Errorf("SelectName() = %q, want empty", got)
		}
	})

	t.Run("SelectName falls back to a status property", func(t *testing.T) {
		var p PropertyValue
		if err := json.Unmarshal([]byte(`{"type":"status","status":{"name":"Claimed"}}`), &p); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := p.SelectName(); got != "Claimed" {
			t.Errorf("SelectName() = %q, want %q", got, "Claimed")
		}
	})

	t.Run("RelationIDs", func(t *testing.T) {
		got := (PropertyValue{Relation: &[]Relation{{ID: "a"}, {ID: "b"}}}).RelationIDs()
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("RelationIDs() = %v", got)
		}
		if got := (PropertyValue{}).RelationIDs(); len(got) != 0 {
			t.Errorf("RelationIDs() = %v, want empty", got)
		}
	})

	t.Run("PeopleIDs", func(t *testing.T) {
		got := NewPeople("u1", "u2").PeopleIDs()
		if len(got) != 2 || got[0] != "u1" || got[1] != "u2" {
			t.Errorf("PeopleIDs() = %v", got)
		}
		if got := (PropertyValue{}).PeopleIDs(); len(got) != 0 {
			t.Errorf("PeopleIDs() = %v, want empty", got)
		}
	})

}

func TestPropertyValueDecodesNotionShapes(t *testing.T) {
	const payload = `{
		"Name":     {"type":"title","title":[{"type":"text","text":{"content":"Slice"},"plain_text":"Slice"}]},
		"Repo":     {"type":"rich_text","rich_text":[{"type":"text","plain_text":"/repo"}]},
		"Status":   {"type":"select","select":{"id":"opt-1","name":"Todo","color":"gray"}},
		"Milestone":{"type":"relation","relation":[{"id":"m1"}]},
		"Assignee": {"type":"people","people":[{"id":"u1","name":"Craig Johnston"}]},
		"PR":       {"type":"url","url":"https://example.test/pr/1"},
		"Order":    {"type":"number","number":3}
	}`

	var props map[string]PropertyValue
	if err := json.Unmarshal([]byte(payload), &props); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := props["Name"].Text(); got != "Slice" {
		t.Errorf("Name = %q", got)
	}
	if got := props["Repo"].Text(); got != "/repo" {
		t.Errorf("Repo = %q", got)
	}
	if got := props["Status"].SelectName(); got != "Todo" {
		t.Errorf("Status = %q", got)
	}
	if got := props["Status"].Select.Color; got != "gray" {
		t.Errorf("Status colour = %q", got)
	}
	if got := props["Milestone"].RelationIDs(); len(got) != 1 || got[0] != "m1" {
		t.Errorf("Milestone = %v", got)
	}
	if got := props["Assignee"].People; len(got) != 1 || got[0].Name != "Craig Johnston" {
		t.Errorf("Assignee = %v", got)
	}
	if got := props["PR"].URL; got != "https://example.test/pr/1" {
		t.Errorf("PR = %q", got)
	}
	if got := props["Order"].Number; got == nil || *got != 3 {
		t.Errorf("Order = %v", got)
	}
	if got := props["Order"].Type; got != "number" {
		t.Errorf("Type = %q", got)
	}
}

func TestListDecodesEnvelope(t *testing.T) {
	var l List[item]
	if err := json.Unmarshal([]byte(`{"results":[{"id":"a"}],"has_more":true,"next_cursor":"c1"}`), &l); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(l.Results) != 1 || l.Results[0].ID != "a" || !l.HasMore {
		t.Errorf("got %+v", l)
	}
	if l.NextCursor == nil || *l.NextCursor != "c1" {
		t.Errorf("NextCursor = %v", l.NextCursor)
	}
}

// A relation is the one property this app clears, so the empty list has to
// survive being marshalled: an omitted key would leave the write saying nothing
// and the slice waiting on exactly what it was.
func TestNewRelation(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
		want string
	}{
		{"naming pages", []string{"s1", "s2"}, `{"relation":[{"id":"s1"},{"id":"s2"}]}`},
		{"naming none clears the property", nil, `{"relation":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(NewRelation(tt.ids...))
			if err != nil {
				t.Fatalf("marshalling: %v", err)
			}
			if string(b) != tt.want {
				t.Errorf("marshalled to %s, want %s", b, tt.want)
			}
			if got := NewRelation(tt.ids...).RelationIDs(); !reflect.DeepEqual(got, tt.ids) {
				t.Errorf("RelationIDs() = %v, want %v", got, tt.ids)
			}
		})
	}
}
