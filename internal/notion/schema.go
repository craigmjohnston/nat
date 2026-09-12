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
// source, not a database, from the data-source API version on. A project's
// milestones are the options of a select rather than a relation, but the slices
// point at each other through one — a slice's dependencies — and a project made
// before milestones were options has a relation to read, which migrating it
// starts from the data source it names.
type RelationConfig struct {
	DataSourceID string `json:"data_source_id"`
	// Kind is "single_property" or "dual_property".
	Kind           string              `json:"type,omitempty"`
	SingleProperty *EmptyConfig        `json:"single_property,omitempty"`
	DualProperty   *DualPropertyConfig `json:"dual_property,omitempty"`
}

// DualPropertyConfig configures the reciprocal half of a dual-property
// relation: the column Notion puts on the far side and keeps in step with this
// one. Writes name it and nothing else; reads get the ID Notion gave it back
// alongside.
type DualPropertyConfig struct {
	SyncedPropertyName string `json:"synced_property_name,omitempty"`
	SyncedPropertyID   string `json:"synced_property_id,omitempty"`
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

// SchemaSelect builds a select property definition offering the named options,
// and — given none — one offering nothing yet, which is what a new project's
// Milestone column is until it has a plan. Every fixed-choice column in this
// app is a select; the API cannot create Notion's status type, so the app does
// not use it anywhere.
func SchemaSelect(options ...string) PropertySchema {
	return PropertySchema{Select: &OptionsConfig{Options: selectOptions(options)}}
}

// SchemaRelation builds the dependency relation pointing at the given data
// source, with [PropBlocks] as its reciprocal half.
//
// It is dual-property on purpose, and the reciprocal column is the whole point
// of it. The Slices data source's dependency column points at itself, and a
// self-relation Notion keeps on one side only has nowhere to put the far end of
// a link: what a single-property write can do is land back in the very column
// it was written to, so recording that A waits on B reads afterwards as the two
// waiting on each other — a mutual block neither next-slice nor the launch key
// will step past. Given a side of its own, Notion's far end lands in Blocks and
// Depends on stays directional, which is the one thing this app reads.
//
// Nothing reads Blocks. It exists so that Notion has somewhere to write that is
// not Depends on.
func SchemaRelation(dataSourceID string) PropertySchema {
	return PropertySchema{Relation: &RelationConfig{
		DataSourceID: dataSourceID,
		Kind:         RelationDual,
		DualProperty: &DualPropertyConfig{SyncedPropertyName: PropBlocks},
	}}
}

// The two relation kinds: RelationSingle puts a column on one side only, which
// is what every dependency column written before [SchemaRelation] asked for a
// reciprocal is, and RelationDual puts one on each. Only the first is read for
// — a migration converts it — and only the second is ever written.
const (
	RelationSingle = "single_property"
	RelationDual   = "dual_property"
)

// SingleSelfRelation reports a property that is this app's dependency column in
// the shape it had before it had a reciprocal: a relation from the Slices data
// source to itself, kept by Notion on one side alone. It is what the migration
// converts, and it is deliberately narrow — a relation pointing anywhere else
// is somebody's own column that happens to share a name, and re-targeting it at
// the slices would throw away what it holds.
func SingleSelfRelation(p PropertySchema, dataSourceID string) bool {
	return p.Relation != nil && p.Relation.Kind == RelationSingle &&
		normalisedID(p.Relation.DataSourceID) == normalisedID(dataSourceID)
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

// AppendedOptions builds the property definition that leaves a select's options
// as they are and adds the named ones after them. Notion replaces an option
// list wholesale rather than merging into it, so the options already there are
// sent back exactly as they were read — ID, name and colour — and only the tail
// is new.
//
// It reports false for anything but a select. The API cannot write the options
// of Notion's own status type, and a column this build cannot read options off
// is not one to guess at: either way there is nothing to append to here, and
// the caller says so rather than writing a schema that drops what it could not
// read.
func (s PropertySchema) AppendedOptions(names ...string) (PropertySchema, bool) {
	if s.Select == nil {
		return PropertySchema{}, false
	}
	options := make([]SelectOption, 0, len(s.Select.Options)+len(names))
	options = append(options, s.Select.Options...)
	options = append(options, selectOptions(names)...)
	return PropertySchema{Select: &OptionsConfig{Options: options}}, true
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

// OptionInsertedAfter builds the property definition that keeps a select's
// options as they are and puts one new option directly after the named one.
// Notion replaces an option list wholesale, so every option already there is
// sent back exactly as it was read — ID, name and colour — and only the new one
// is new: an option with no ID is one Notion creates.
//
// It is how a milestone is renamed without moving in the plan. Renaming an
// option in place is quietly ignored by the API, so the new name has to arrive
// as an option of its own; arriving beside the old one rather than at the end
// is what leaves the plan in the order it was in once the old one is dropped.
//
// It reports false for anything but a select, for the reason
// [PropertySchema.AppendedOptions] does, and for an option list that does not
// hold the named option — there is nowhere in particular to put the new one.
func (s PropertySchema) OptionInsertedAfter(existing, name string) (PropertySchema, bool) {
	if s.Select == nil {
		return PropertySchema{}, false
	}
	options := make([]SelectOption, 0, len(s.Select.Options)+1)
	found := false
	for _, o := range s.Select.Options {
		options = append(options, o)
		if o.Name == existing {
			options = append(options, SelectOption{Name: name})
			found = true
		}
	}
	if !found {
		return PropertySchema{}, false
	}
	return PropertySchema{Select: &OptionsConfig{Options: options}}, true
}

// WithoutOption builds the property definition that is this select minus the
// named option, and false for anything but a select. Every option kept is sent
// back exactly as it was read, because Notion replaces an option list wholesale:
// what the list omits is removed.
func (s PropertySchema) WithoutOption(name string) (PropertySchema, bool) {
	if s.Select == nil {
		return PropertySchema{}, false
	}
	options := make([]SelectOption, 0, len(s.Select.Options))
	for _, o := range s.Select.Options {
		if o.Name != name {
			options = append(options, o)
		}
	}
	return PropertySchema{Select: &OptionsConfig{Options: options}}, true
}

// OptionMoved builds the property definition that is this select with one option
// lifted out of the list and put back directly before or after another. Every
// option is exactly the one it was — ID, name and colour — and the order of the
// list is the only thing that differs, which matters because Notion replaces an
// option list wholesale rather than merging into it: an option sent back changed
// would be changed, and one left out would be gone.
//
// It is how a milestone is moved in the plan. A milestone's order is its place
// among these options, so reordering them is the whole of the move: no option
// is created or retired, and so no slice has to be refiled — unlike a rename,
// which the API will not do in place at all.
//
// It reports false for anything but a select, for the reason
// [PropertySchema.AppendedOptions] does, and where the list does not hold both
// named options — there is either nothing to move or nowhere to put it, and
// naming one option twice is both at once.
func (s PropertySchema) OptionMoved(name, target string, before bool) (PropertySchema, bool) {
	if s.Select == nil {
		return PropertySchema{}, false
	}
	var moved SelectOption
	rest := make([]SelectOption, 0, len(s.Select.Options))
	found, marked := false, false
	for _, o := range s.Select.Options {
		if o.Name == name {
			moved, found = o, true
			continue
		}
		rest = append(rest, o)
		if o.Name == target {
			marked = true
		}
	}
	if !found || !marked {
		return PropertySchema{}, false
	}
	options := make([]SelectOption, 0, len(s.Select.Options))
	for _, o := range rest {
		if o.Name == target && before {
			options = append(options, moved)
		}
		options = append(options, o)
		if o.Name == target && !before {
			options = append(options, moved)
		}
	}
	return PropertySchema{Select: &OptionsConfig{Options: options}}, true
}
