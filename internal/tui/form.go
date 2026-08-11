package tui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/huh/v2"
)

// newForm builds a huh form with the chrome every form on the board shares:
// the styles' theme, and the keymap below. Forms are built here rather than
// straight from huh.NewForm so a binding is decided once for the board instead
// of once per form.
func newForm(theme huh.Theme, groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).WithTheme(theme).WithKeyMap(formKeyMap())
}

// formKeyMap is huh's defaults with shift+enter added to the multiline field's
// newline binding, so a brief typed on the board breaks its lines the same way
// one typed at an agent's pane does. Enter is untouched and still submits;
// huh's own alt+enter and ctrl+j stay for terminals that report no modifier on
// enter at all.
func formKeyMap() *huh.KeyMap {
	k := huh.NewDefaultKeyMap()
	k.Text.NewLine = key.NewBinding(
		key.WithKeys("shift+enter", "alt+enter", "ctrl+j"),
		key.WithHelp("shift+enter", "new line"),
	)
	return k
}
