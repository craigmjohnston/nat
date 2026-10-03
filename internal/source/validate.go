package source

import (
	"fmt"
	"regexp"
	"strings"
)

// tagPattern is what a describe's tag must be: one to three upper-case letters
// or digits, short enough to sit on an Active row beside a task's title.
var tagPattern = regexp.MustCompile(`^[A-Z0-9]{1,3}$`)

// ValidateDescribe refuses a describe nat could not draw: a tag outside the
// protocol's shape, or a menu action it could not offer. The protocol version
// is not checked here — [Exec.Describe] refuses that first, in its own words.
// The error names the rule broken and the field, never the response.
func ValidateDescribe(d Describe) error {
	if !tagPattern.MatchString(d.Tag) {
		return fmt.Errorf("tag %q is not 1–3 upper-case letters or digits", d.Tag)
	}
	return validateActions("the source menu", d.Menu)
}

// ValidateGroups refuses a sidebar tree that breaks the protocol's shape: a
// group with both children and containers, children nested more than one
// level, a group id used twice, an empty id or one starting with `_` (nat's
// own, for the groups it adds), or a menu action nat could not offer. A
// container may sit in more than one group, so only group ids must be unique.
func ValidateGroups(groups []Group) error {
	seen := map[string]bool{}
	for _, g := range groups {
		if err := validateGroup(g, seen); err != nil {
			return err
		}
		for _, c := range g.Children {
			if len(c.Children) > 0 {
				return fmt.Errorf("group %q nests children more than one level deep", c.ID)
			}
			if err := validateGroup(c, seen); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateGroup checks one group — its id, its shape, its menu and its
// containers — recording its id in seen.
func validateGroup(g Group, seen map[string]bool) error {
	if err := validateID("group", g.ID); err != nil {
		return err
	}
	if seen[g.ID] {
		return fmt.Errorf("group id %q is used more than once", g.ID)
	}
	seen[g.ID] = true
	if len(g.Children) > 0 && len(g.Containers) > 0 {
		return fmt.Errorf("group %q has both children and containers", g.ID)
	}
	if err := validateActions(fmt.Sprintf("group %q's menu", g.ID), g.Menu); err != nil {
		return err
	}
	for _, c := range g.Containers {
		if err := validateID("container", c.ID); err != nil {
			return err
		}
		if err := validateActions(fmt.Sprintf("container %q's menu", c.ID), c.Menu); err != nil {
			return err
		}
	}
	return nil
}

// ValidateContainer refuses a container's detail carrying an action nat could
// not offer, on its menu or as a section's composer.
func ValidateContainer(d ContainerDetail) error {
	if err := validateActions("the container menu", d.Menu); err != nil {
		return err
	}
	for _, s := range d.Sections {
		if s.Composer == nil {
			continue
		}
		if err := validateActions(fmt.Sprintf("section %q's composer", s.ID), []Action{*s.Composer}); err != nil {
			return err
		}
	}
	return nil
}

// validateID refuses an id the protocol does not allow: empty, or one of the
// `_`-prefixed ids nat keeps for itself.
func validateID(kind, id string) error {
	if id == "" {
		return fmt.Errorf("a %s has no id", kind)
	}
	if strings.HasPrefix(id, "_") {
		return fmt.Errorf("%s id %q starts with _, which nat reserves", kind, id)
	}
	return nil
}

// validateActions refuses a choice action with no options to choose from.
func validateActions(where string, actions []Action) error {
	for _, a := range actions {
		if a.Input == InputChoice && len(a.Options) == 0 {
			return fmt.Errorf("%s: choice action %q has no options", where, a.ID)
		}
	}
	return nil
}
