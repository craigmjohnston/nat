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

// Persons returns the real people among users, dropping bots. Onboarding uses
// it to offer a list of assignable users.
func Persons(users []User) []User {
	out := make([]User, 0, len(users))
	for _, u := range users {
		if u.IsPerson() {
			out = append(out, u)
		}
	}
	return out
}

// ListUsers returns every user in the workspace the integration can see,
// following pagination to the end. The list mixes people and bots; filter it
// with Persons.
func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	return paginate[User](ctx, c, http.MethodGet, "/users", nil)
}

// Me returns the bot user the API key authenticates as.
func (c *Client) Me(ctx context.Context) (*User, error) {
	var u User
	if err := c.do(ctx, http.MethodGet, "/users/me", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
