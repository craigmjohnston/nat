package notion

// EmptyConfig is the configuration of a property type that has none — Notion
// still expects the key to be present as an empty object, e.g.
// `{"Name": {"title": {}}}`.
type EmptyConfig struct{}

// OptionsConfig configures a select property with its choices.
type OptionsConfig struct {
	Options []SelectOption `json:"options,omitempty"`
}

// RelationConfig configures a relation property. Relations point at a data
// source, not a database, from the data-source API version on. Creating one
// also requires saying which kind it is, with the matching sub-object present:
// omitting them fails validation.
type RelationConfig struct {
	DataSourceID string `json:"data_source_id"`
	// Kind is "single_property" or "dual_property".
	Kind           string       `json:"type,omitempty"`
	SingleProperty *EmptyConfig `json:"single_property,omitempty"`
}

// PropertySchema is one property definition in a data source schema. Reads
// populate ID, Name and Type plus the config field matching Type; writes set
// exactly one config field and leave the rest nil, so a value built by one of
// the Schema* constructors serialises to precisely that property definition.
type PropertySchema struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`

	Title    *EmptyConfig    `json:"title,omitempty"`
	RichText *EmptyConfig    `json:"rich_text,omitempty"`
	Number   *EmptyConfig    `json:"number,omitempty"`
	Select   *OptionsConfig  `json:"select,omitempty"`
	Status   *OptionsConfig  `json:"status,omitempty"`
	Relation *RelationConfig `json:"relation,omitempty"`
	People   *EmptyConfig    `json:"people,omitempty"`
	URL      *EmptyConfig    `json:"url,omitempty"`
}

// SchemaTitle builds the title property definition. Every data source has
// exactly one.
func SchemaTitle() PropertySchema {
	return PropertySchema{Title: &EmptyConfig{}}
}

// SchemaRichText builds a rich_text property definition.
func SchemaRichText() PropertySchema {
	return PropertySchema{RichText: &EmptyConfig{}}
}

// SchemaNumber builds a number property definition.
func SchemaNumber() PropertySchema {
	return PropertySchema{Number: &EmptyConfig{}}
}

// SchemaSelect builds a select property definition offering the named options.
// Every fixed-choice column in this app — slice Status, milestone Status — is a
// select; the API cannot create Notion's status type, so the app does not use
// it anywhere.
func SchemaSelect(options ...string) PropertySchema {
	return PropertySchema{Select: &OptionsConfig{Options: selectOptions(options)}}
}

// SchemaRelation builds a single-property relation definition pointing at a
// data source — one-way, so the related data source gains no column of its own.
func SchemaRelation(dataSourceID string) PropertySchema {
	return PropertySchema{Relation: &RelationConfig{
		DataSourceID:   dataSourceID,
		Kind:           "single_property",
		SingleProperty: &EmptyConfig{},
	}}
}

// SchemaPeople builds a people property definition.
func SchemaPeople() PropertySchema {
	return PropertySchema{People: &EmptyConfig{}}
}

// SchemaURL builds a url property definition.
func SchemaURL() PropertySchema {
	return PropertySchema{URL: &EmptyConfig{}}
}

// selectOptions turns option names into the option objects Notion expects.
func selectOptions(names []string) []SelectOption {
	if len(names) == 0 {
		return nil
	}
	opts := make([]SelectOption, len(names))
	for i, n := range names {
		opts[i] = SelectOption{Name: n}
	}
	return opts
}

// OptionNames returns the option names of a fixed-choice property — a select,
// or a status column converted in the Notion UI — and nil for any other
// property type. This app only ever creates selects, but it reads back whatever
// the project has become.
func (s PropertySchema) OptionNames() []string {
	options := s.Select
	if options == nil {
		options = s.Status
	}
	if options == nil {
		return nil
	}
	names := make([]string, len(options.Options))
	for i, o := range options.Options {
		names[i] = o.Name
	}
	return names
}
