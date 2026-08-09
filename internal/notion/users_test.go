package notion

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListUsers(t *testing.T) {
	t.Run("returns every user across pages", func(t *testing.T) {
		var paths []string
		var calls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.RequestURI())
			if r.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", r.Method)
			}
			calls++
			if calls == 1 {
				json.NewEncoder(w).Encode(map[string]any{
					"results": []map[string]any{{
						"id": "user-1", "name": "Craig Johnston", "type": "person",
						"person": map[string]any{"email": "craig@example.test"},
					}},
					"has_more":    true,
					"next_cursor": "cursor-1",
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"results":  []map[string]any{{"id": "bot-1", "name": "tracker", "type": "bot"}},
				"has_more": false,
			})
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		users, err := c.ListUsers(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(paths) != 2 || paths[0] != "/users" || paths[1] != "/users?start_cursor=cursor-1" {
			t.Errorf("requests = %v", paths)
		}
		if len(users) != 2 {
			t.Fatalf("users = %+v", users)
		}
		if users[0].ID != "user-1" || users[0].Name != "Craig Johnston" {
			t.Errorf("got %+v", users[0])
		}
		if got := users[0].Email(); got != "craig@example.test" {
			t.Errorf("Email = %q", got)
		}
		if got := users[1].Email(); got != "" {
			t.Errorf("bot Email = %q, want empty", got)
		}
	})

	t.Run("propagates an API error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"code":"unauthorized","message":"bad token"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		users, err := c.ListUsers(context.Background())
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.Unauthorized() {
			t.Fatalf("got %v, want an unauthorized *APIError", err)
		}
		if users != nil {
			t.Errorf("users = %+v, want nil on error", users)
		}
	})
}

func TestMe(t *testing.T) {
	t.Run("returns the bot user", func(t *testing.T) {
		var gotMethod, gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			w.Write([]byte(`{"id":"bot-1","name":"tracker","type":"bot"}`))
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
		if u.ID != "bot-1" || u.Name != "tracker" || u.IsPerson() {
			t.Errorf("got %+v", u)
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

func TestPersons(t *testing.T) {
	users := []User{
		{ID: "user-1", Type: UserPerson},
		{ID: "bot-1", Type: UserBot},
		{ID: "user-2", Type: UserPerson},
		{ID: "prop-user"}, // as read from a people property: no type
	}
	got := Persons(users)
	if len(got) != 2 || got[0].ID != "user-1" || got[1].ID != "user-2" {
		t.Errorf("Persons = %+v, want only the two people", got)
	}
	if len(Persons(nil)) != 0 {
		t.Error("Persons(nil) should be empty")
	}
}
