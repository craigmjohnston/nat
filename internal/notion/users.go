package notion

import (
	"context"
	"net/http"
)

// User object types, as reported by the users endpoints.
const (
	UserPerson = "person"
	UserBot    = "bot"
)

// IsPerson reports whether the user is a real person rather than an
// integration bot. Users read from a people property carry no type, so this
// only answers usefully for users returned by the users endpoints.
func (u User) IsPerson() bool { return u.Type == UserPerson }

// OwnerPerson returns the real person the token acts for, as reported by Me:
// the owner of a personal access token. It reports false for a token owned by
// the workspace rather than a person — an internal integration — where there is
// nobody to claim slices as.
func (u User) OwnerPerson() (User, bool) {
	if u.Bot == nil || u.Bot.Owner == nil || u.Bot.Owner.User == nil {
		return User{}, false
	}
	owner := *u.Bot.Owner.User
	if !owner.IsPerson() {
		return User{}, false
	}
	return owner, true
}

// Me returns the bot user the API key authenticates as.
func (c *Client) Me(ctx context.Context) (*User, error) {
	var u User
	if err := c.do(ctx, http.MethodGet, "/users/me", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
