package notion

import "time"

// Page is a Notion page — a row of a data source, here. Data source queries and
// the page endpoints return the same shape; only the fields this app reads are
// modelled. A missing property reads as the zero PropertyValue, so
// p.Properties["Status"].SelectName() is safe on any page.
type Page struct {
	ID          string                   `json:"id"`
	URL         string                   `json:"url"`
	CreatedTime time.Time                `json:"created_time"`
	Properties  map[string]PropertyValue `json:"properties"`
}

// List is the envelope every Notion list endpoint returns.
type List[T any] struct {
	Results    []T     `json:"results"`
	HasMore    bool    `json:"has_more"`
	NextCursor *string `json:"next_cursor"`
}

// TextContent is the payload of a plain (non-mention, non-equation) rich text
// item.
type TextContent struct {
	Content string `json:"content"`
}

// RichText is one span of Notion rich text. Only the fields this app reads or
// writes are modelled: writes send type+text, reads use plain_text, which
// Notion populates for every span type.
type RichText struct {
	Type      string       `json:"type,omitempty"`
	Text      *TextContent `json:"text,omitempty"`
	PlainText string       `json:"plain_text,omitempty"`
}

// PlainText joins the spans of a rich text value into a single string.
func PlainText(spans []RichText) string {
	switch len(spans) {
	case 0:
		return ""
	case 1:
		return spans[0].PlainText
	}
	out := make([]byte, 0, 32)
	for _, s := range spans {
		out = append(out, s.PlainText...)
	}
	return string(out)
}

// richText builds the single-span rich text array Notion expects on writes.
func richText(s string) []RichText {
	return []RichText{{Type: "text", Text: &TextContent{Content: s}}}
}

// SelectOption is one select choice. Writes set Name only; reads also carry ID
// and Color.
type SelectOption struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Color string `json:"color,omitempty"`
}

// Relation is a reference to another page, as used by relation properties.
type Relation struct {
	ID string `json:"id"`
}

// User is a Notion user (person or bot). People properties are arrays of these;
// writes need only ID.
type User struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// PropertyValue is one property of a page, in the shape Notion uses for both
// reads and writes. Every field is omitted when empty, so a value built by one
// of the New* constructors serialises to exactly the property being written.
// Clearing a property is therefore not expressible here — no flow in this app
// needs it yet.
type PropertyValue struct {
	Type     string        `json:"type,omitempty"`
	Title    []RichText    `json:"title,omitempty"`
	RichText []RichText    `json:"rich_text,omitempty"`
	Select   *SelectOption `json:"select,omitempty"`
	Relation []Relation    `json:"relation,omitempty"`
	People   []User        `json:"people,omitempty"`
	URL      string        `json:"url,omitempty"`
	Number   *float64      `json:"number,omitempty"`
}

// NewTitle builds a title property value.
func NewTitle(s string) PropertyValue {
	return PropertyValue{Title: richText(s)}
}

// NewRichText builds a rich_text property value.
func NewRichText(s string) PropertyValue {
	return PropertyValue{RichText: richText(s)}
}

// NewSelect builds a select property value naming an option.
func NewSelect(name string) PropertyValue {
	return PropertyValue{Select: &SelectOption{Name: name}}
}

// NewRelation builds a relation property value pointing at the given page IDs.
func NewRelation(pageIDs ...string) PropertyValue {
	rels := make([]Relation, len(pageIDs))
	for i, id := range pageIDs {
		rels[i] = Relation{ID: id}
	}
	return PropertyValue{Relation: rels}
}

// NewPeople builds a people property value naming the given user IDs.
func NewPeople(userIDs ...string) PropertyValue {
	people := make([]User, len(userIDs))
	for i, id := range userIDs {
		people[i] = User{ID: id}
	}
	return PropertyValue{People: people}
}

// NewURL builds a url property value.
func NewURL(u string) PropertyValue {
	return PropertyValue{URL: u}
}

// NewNumber builds a number property value.
func NewNumber(n float64) PropertyValue {
	return PropertyValue{Number: &n}
}

// Text returns the plain text of a title or rich_text property, and "" for any
// other property type.
func (p PropertyValue) Text() string {
	if len(p.Title) > 0 {
		return PlainText(p.Title)
	}
	return PlainText(p.RichText)
}

// SelectName returns the option name of a select property, and "" when unset.
func (p PropertyValue) SelectName() string {
	if p.Select == nil {
		return ""
	}
	return p.Select.Name
}

// RelationIDs returns the related page IDs of a relation property.
func (p PropertyValue) RelationIDs() []string {
	ids := make([]string, len(p.Relation))
	for i, r := range p.Relation {
		ids[i] = r.ID
	}
	return ids
}

// PeopleIDs returns the user IDs of a people property.
func (p PropertyValue) PeopleIDs() []string {
	ids := make([]string, len(p.People))
	for i, u := range p.People {
		ids[i] = u.ID
	}
	return ids
}

// NumberValue returns the value of a number property and whether it was set.
func (p PropertyValue) NumberValue() (float64, bool) {
	if p.Number == nil {
		return 0, false
	}
	return *p.Number, true
}
