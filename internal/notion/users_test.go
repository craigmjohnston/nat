package notion

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMe(t *testing.T) {
	t.Run("returns the bot user", func(t *testing.T) {
		var gotMethod, gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			w.Write([]byte(`{
				"id":"bot-1","name":"Notion CLI","type":"bot",
				"bot":{
					"owner":{"type":"user","user":{
						"id":"user-1","name":"Craig Johnston","type":"person",
						"person":{"email":"craig@example.test"}
					}},
					"workspace_name":"Craig's Notion"
				}
			}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		u, err := c.Me(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotMethod != http.MethodGet || gotPath != "/users/me" {
			t.Errorf("got %s %s, want GET /users/me", gotMethod, gotPath)
		}
		if u.ID != "bot-1" || u.Name != "Notion CLI" || u.IsPerson() {
			t.Errorf("got %+v", u)
		}
		if u.Bot == nil || u.Bot.WorkspaceName != "Craig's Notion" {
			t.Fatalf("bot = %+v, want the workspace name decoded", u.Bot)
		}
		owner, ok := u.OwnerPerson()
		if !ok || owner.ID != "user-1" || owner.Name != "Craig Johnston" {
			t.Errorf("OwnerPerson() = %+v, %v, want the person the token acts for", owner, ok)
		}
		if got := owner.Email(); got != "craig@example.test" {
			t.Errorf("owner Email = %q", got)
		}
	})

	t.Run("propagates an API error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		u, err := c.Me(context.Background())
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("got %v, want *APIError", err)
		}
		if u != nil {
			t.Errorf("user = %+v, want nil on error", u)
		}
	})
}

func TestOwnerPerson(t *testing.T) {
	// A personal access token names the person it acts for; everything else has
	// nobody behind it to claim slices as.
	person := &User{ID: "user-1", Name: "Craig Johnston", Type: UserPerson}
	tests := []struct {
		name string
		user User
		want bool
	}{
		{"a personal access token", User{Type: UserBot, Bot: &Bot{
			Owner: &BotOwner{Type: OwnerUser, User: person}}}, true},
		{"an internal integration", User{Type: UserBot, Bot: &Bot{
			Owner: &BotOwner{Type: OwnerWorkspace}}}, false},
		{"an owner that is somehow not a person", User{Type: UserBot, Bot: &Bot{
			Owner: &BotOwner{Type: OwnerUser, User: &User{ID: "bot-2", Type: UserBot}}}}, false},
		{"no owner at all", User{Type: UserBot, Bot: &Bot{}}, false},
		{"not a bot", User{ID: "user-1", Type: UserPerson}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.user.OwnerPerson()
			if ok != tt.want {
				t.Fatalf("OwnerPerson() ok = %v, want %v", ok, tt.want)
			}
			if ok && got.ID != "user-1" {
				t.Errorf("OwnerPerson() = %+v, want the owning person", got)
			}
		})
	}
}

func TestUserEmail(t *testing.T) {
	// A bot, and a user read off a people property, carry no person at all.
	if got := (User{Person: &Person{Email: "craig@example.test"}}).Email(); got != "craig@example.test" {
		t.Errorf("Email = %q, want the address", got)
	}
	if got := (User{ID: "bot-1", Type: UserBot}).Email(); got != "" {
		t.Errorf("Email = %q, want empty for a user with no person", got)
	}
}
