package notion

// EmptyConfig is the configuration of a property type that has none — Notion
// still expects the key to be present as an empty object, e.g.
// `{"Name": {"title": {}}}`.
type EmptyConfig struct{}

// OptionsConfig configures a select or status property with its choices.
type OptionsConfig struct {
	Options []SelectOption `json:"options,omitempty"`
}

// RelationConfig configures a relation property. Relations point at a data
// source, not a database, from the data-source API version on.
type RelationConfig struct {
	DataSourceID string `json:"data_source_id"`
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
//
// Bootstrapping uses select, not status: the API cannot create status
// properties, so databases this app creates carry their workflow states as a
// select. Read them back with OptionNames, which spans both shapes.
func SchemaSelect(options ...string) PropertySchema {
	return PropertySchema{Select: &OptionsConfig{Options: selectOptions(options)}}
}

// SchemaStatus builds a status property definition offering the named options,
// for databases that already carry a status property.
func SchemaStatus(options ...string) PropertySchema {
	return PropertySchema{Status: &OptionsConfig{Options: selectOptions(options)}}
}

// SchemaRelation builds a relation property definition pointing at a data
// source.
func SchemaRelation(dataSourceID string) PropertySchema {
	return PropertySchema{Relation: &RelationConfig{DataSourceID: dataSourceID}}
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

// IsChoice reports whether the property offers a fixed set of options, in
// either the select or the status shape. Schema verification treats the two as
// interchangeable: a database this app bootstrapped uses select, one a human
// converted in the Notion UI uses status, and both drive the same workflow.
func (s PropertySchema) IsChoice() bool {
	return s.Select != nil || s.Status != nil
}

// OptionNames returns the option names of a select or status property, and nil
// for any other property type. This is the one accessor for both shapes; the
// value-side equivalent is PropertyValue.SelectName.
func (s PropertySchema) OptionNames() []string {
	cfg := s.Select
	if cfg == nil {
		cfg = s.Status
	}
	if cfg == nil {
		return nil
	}
	names := make([]string, len(cfg.Options))
	for i, o := range cfg.Options {
		names[i] = o.Name
	}
	return names
}
