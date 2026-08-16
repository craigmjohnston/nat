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
	// Parent is where the page lives, which the breadcrumb walk climbs.
	Parent Parent `json:"parent"`
}

// TitleText returns the page's name — the text of whichever property holds its
// title, whatever the schema calls it — and "" for an untitled page. A page has
// at most one title property, so which one is found is never in question.
func (p Page) TitleText() string {
	for _, v := range p.Properties {
		if len(v.Title) > 0 {
			return PlainText(v.Title)
		}
	}
	return ""
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

// Annotations are the inline styles Notion attaches to a rich text span. Only
// the ones with a markdown equivalent are modelled — underline and colour have
// none, so they are dropped when rendering.
type Annotations struct {
	Bold          bool `json:"bold,omitempty"`
	Italic        bool `json:"italic,omitempty"`
	Strikethrough bool `json:"strikethrough,omitempty"`
	Code          bool `json:"code,omitempty"`
}

// RichText is one span of Notion rich text. Only the fields this app reads or
// writes are modelled: writes send type+text, reads use plain_text, which
// Notion populates for every span type, plus the annotations and link target
// the markdown renderer needs.
type RichText struct {
	Type        string       `json:"type,omitempty"`
	Text        *TextContent `json:"text,omitempty"`
	PlainText   string       `json:"plain_text,omitempty"`
	Annotations *Annotations `json:"annotations,omitempty"`
	Href        string       `json:"href,omitempty"`
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
// writes need only ID, so every other field is omitted when empty. Type, Person
// and Bot are populated by the users endpoints, not by page properties.
type User struct {
	ID     string  `json:"id"`
	Name   string  `json:"name,omitempty"`
	Type   string  `json:"type,omitempty"`
	Person *Person `json:"person,omitempty"`
	Bot    *Bot    `json:"bot,omitempty"`
}

// Person carries the details Notion exposes for a human user. The email is
// only present when the integration has user-with-email capabilities.
type Person struct {
	Email string `json:"email,omitempty"`
}

// Bot carries the details Notion exposes for the user a token authenticates as.
// Owner is the whole reason it is modelled: it is the only way to learn who a
// personal access token acts for, since such a token cannot list users.
type Bot struct {
	Owner         *BotOwner `json:"owner,omitempty"`
	WorkspaceName string    `json:"workspace_name,omitempty"`
}

// BotOwner says on whose behalf a token acts: a real person, for a personal
// access token, or the workspace itself for an internal integration — in which
// case User is absent, because there is no one person behind it.
type BotOwner struct {
	Type string `json:"type"`
	User *User  `json:"user,omitempty"`
}

// The owner types Notion reports for a bot user.
const (
	OwnerUser      = "user"
	OwnerWorkspace = "workspace"
)

// Email returns the user's email address, and "" when it is a bot or the
// integration cannot read emails.
func (u User) Email() string {
	if u.Person == nil {
		return ""
	}
	return u.Person.Email
}

// PropertyValue is one property of a page, in the shape Notion uses for both
// reads and writes. Every field is omitted when empty, so a value built by one
// of the New* constructors serialises to exactly the property being written.
//
// Relation is the one exception, and a pointer for exactly that reason: a
// relation is the one property this app clears, and an empty list is how Notion
// is told to — which `omitempty` on a plain slice would drop, leaving the write
// saying nothing at all. A nil pointer is the property being left out; a
// pointer to an empty list is the property being emptied.
type PropertyValue struct {
	Type     string        `json:"type,omitempty"`
	Title    []RichText    `json:"title,omitempty"`
	RichText []RichText    `json:"rich_text,omitempty"`
	Select   *SelectOption `json:"select,omitempty"`
	Status   *SelectOption `json:"status,omitempty"`
	Relation *[]Relation   `json:"relation,omitempty"`
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

// The property types of a fixed-choice column. The API cannot create Notion's
// own status type, so this app only ever makes selects — but a column converted
// in the Notion UI is a status, and must still be readable and writable.
const (
	TypeSelect = "select"
	TypeStatus = "status"
)

// TypePeople is the property type of a people column, named because a project
// may or may not have one and the difference is read from the schema.
const TypePeople = "people"

// TypeRichText is the property type of a text column, named for the same reason
// as TypePeople: a project may or may not carry the optional ones, and what is
// there is read from the schema rather than assumed.
const TypeRichText = "rich_text"

// NewSelect builds a select property value naming an option.
func NewSelect(name string) PropertyValue {
	return PropertyValue{Select: &SelectOption{Name: name}}
}

// NewStatus builds a status property value naming an option.
func NewStatus(name string) PropertyValue {
	return PropertyValue{Status: &SelectOption{Name: name}}
}

// NewChoice builds a fixed-choice property value in the shape propertyType
// names — the type Notion reported for the property being written, as carried
// by the page it was read from. Anything but a status is written as a select,
// which is what this app's own schemas use.
func NewChoice(propertyType, name string) PropertyValue {
	if propertyType == TypeStatus {
		return NewStatus(name)
	}
	return NewSelect(name)
}

// NewPeople builds a people property value naming the given user IDs.
func NewPeople(userIDs ...string) PropertyValue {
	people := make([]User, len(userIDs))
	for i, id := range userIDs {
		people[i] = User{ID: id}
	}
	return PropertyValue{People: people}
}

// NewRelation builds a relation property value naming the given pages, and —
// given none — the value that empties the relation, which is how a slice is
// told it waits on nothing.
func NewRelation(pageIDs ...string) PropertyValue {
	rels := make([]Relation, len(pageIDs))
	for i, id := range pageIDs {
		rels[i] = Relation{ID: id}
	}
	return PropertyValue{Relation: &rels}
}

// NewURL builds a url property value.
func NewURL(u string) PropertyValue {
	return PropertyValue{URL: u}
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
// A status property carries the same option shape under a different key — this
// app only ever creates selects, but a project customised in Notion may have
// been converted — so that is read as a fallback.
func (p PropertyValue) SelectName() string {
	if p.Select != nil {
		return p.Select.Name
	}
	if p.Status != nil {
		return p.Status.Name
	}
	return ""
}

// RelationIDs returns the related page IDs of a relation property, and none for
// a property that is not one or holds nothing.
func (p PropertyValue) RelationIDs() []string {
	if p.Relation == nil || len(*p.Relation) == 0 {
		return nil
	}
	ids := make([]string, len(*p.Relation))
	for i, r := range *p.Relation {
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
