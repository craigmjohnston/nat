package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func stubKeyring(get func(string, string) (string, error), set func(string, string, string) error, del func(string, string) error) *Keyring {
	return &Keyring{get: get, set: set, del: del}
}

func TestNewKeyringWiresRealFuncs(t *testing.T) {
	k := NewKeyring()
	if k.get == nil || k.set == nil || k.del == nil {
		t.Fatal("NewKeyring left a keyring func nil")
	}
}

func TestKeyringGetAPIKey(t *testing.T) {
	t.Run("success passes service and account", func(t *testing.T) {
		k := stubKeyring(func(service, user string) (string, error) {
			if service != "notion-agent-tracker" || user != "notion-api-key" {
				t.Errorf("got service=%q user=%q", service, user)
			}
			return "secret-key", nil
		}, nil, nil)
		got, err := k.GetAPIKey()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "secret-key" {
			t.Errorf("got %q, want %q", got, "secret-key")
		}
	})

	t.Run("not found maps to ErrAPIKeyNotFound", func(t *testing.T) {
		k := stubKeyring(func(string, string) (string, error) {
			return "", keyring.ErrNotFound
		}, nil, nil)
		_, err := k.GetAPIKey()
		if !errors.Is(err, ErrAPIKeyNotFound) {
			t.Fatalf("got %v, want ErrAPIKeyNotFound", err)
		}
	})

	t.Run("other errors are wrapped", func(t *testing.T) {
		cause := errors.New("keychain locked")
		k := stubKeyring(func(string, string) (string, error) {
			return "", cause
		}, nil, nil)
		_, err := k.GetAPIKey()
		if !errors.Is(err, cause) {
			t.Fatalf("got %v, want wrapped %v", err, cause)
		}
		if errors.Is(err, ErrAPIKeyNotFound) {
			t.Fatal("non-not-found error must not match ErrAPIKeyNotFound")
		}
	})
}

func TestKeyringSetAPIKey(t *testing.T) {
	t.Run("success passes service, account, and key", func(t *testing.T) {
		k := stubKeyring(nil, func(service, user, password string) error {
			if service != "notion-agent-tracker" || user != "notion-api-key" || password != "secret-key" {
				t.Errorf("got service=%q user=%q password=%q", service, user, password)
			}
			return nil
		}, nil)
		if err := k.SetAPIKey("secret-key"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("errors are wrapped without key material", func(t *testing.T) {
		cause := errors.New("write denied")
		k := stubKeyring(nil, func(string, string, string) error { return cause }, nil)
		err := k.SetAPIKey("secret-key")
		if !errors.Is(err, cause) {
			t.Fatalf("got %v, want wrapped %v", err, cause)
		}
		if strings.Contains(err.Error(), "secret-key") {
			t.Fatal("error message leaks key material")
		}
	})
}

func TestKeyringDeleteAPIKey(t *testing.T) {
	t.Run("success passes service and account", func(t *testing.T) {
		k := stubKeyring(nil, nil, func(service, user string) error {
			if service != "notion-agent-tracker" || user != "notion-api-key" {
				t.Errorf("got service=%q user=%q", service, user)
			}
			return nil
		})
		if err := k.DeleteAPIKey(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found maps to ErrAPIKeyNotFound", func(t *testing.T) {
		k := stubKeyring(nil, nil, func(string, string) error { return keyring.ErrNotFound })
		if err := k.DeleteAPIKey(); !errors.Is(err, ErrAPIKeyNotFound) {
			t.Fatalf("got %v, want ErrAPIKeyNotFound", err)
		}
	})

	t.Run("other errors are wrapped", func(t *testing.T) {
		cause := errors.New("keychain locked")
		k := stubKeyring(nil, nil, func(string, string) error { return cause })
		if err := k.DeleteAPIKey(); !errors.Is(err, cause) {
			t.Fatalf("got %v, want wrapped %v", err, cause)
		}
	})
}

func TestMemorySecrets(t *testing.T) {
	t.Run("get before set returns ErrAPIKeyNotFound", func(t *testing.T) {
		m := &MemorySecrets{}
		if _, err := m.GetAPIKey(); !errors.Is(err, ErrAPIKeyNotFound) {
			t.Fatalf("got %v, want ErrAPIKeyNotFound", err)
		}
	})

	t.Run("set then get round-trips", func(t *testing.T) {
		m := &MemorySecrets{}
		if err := m.SetAPIKey("secret-key"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := m.GetAPIKey()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "secret-key" {
			t.Errorf("got %q, want %q", got, "secret-key")
		}
	})

	t.Run("empty string is a stored value, not absence", func(t *testing.T) {
		m := &MemorySecrets{}
		if err := m.SetAPIKey(""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, err := m.GetAPIKey(); err != nil || got != "" {
			t.Fatalf("got %q, %v; want empty string, nil", got, err)
		}
	})

	t.Run("delete removes the key", func(t *testing.T) {
		m := &MemorySecrets{}
		if err := m.SetAPIKey("secret-key"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := m.DeleteAPIKey(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := m.GetAPIKey(); !errors.Is(err, ErrAPIKeyNotFound) {
			t.Fatalf("got %v, want ErrAPIKeyNotFound after delete", err)
		}
	})

	t.Run("delete when empty returns ErrAPIKeyNotFound", func(t *testing.T) {
		m := &MemorySecrets{}
		if err := m.DeleteAPIKey(); !errors.Is(err, ErrAPIKeyNotFound) {
			t.Fatalf("got %v, want ErrAPIKeyNotFound", err)
		}
	})
}
