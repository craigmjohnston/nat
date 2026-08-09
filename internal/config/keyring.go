// Package config handles local configuration: the XDG config file and the
// Notion API key stored in the OS keychain.
package config

import (
	"errors"
	"fmt"
	"sync"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "notion-agent-tracker"
	keyringAccount = "notion-api-key"
)

// ErrAPIKeyNotFound is returned when no API key is stored.
var ErrAPIKeyNotFound = errors.New("notion API key not found in keyring")

// Secrets stores and retrieves the Notion API key.
type Secrets interface {
	GetAPIKey() (string, error)
	SetAPIKey(string) error
	DeleteAPIKey() error
}

// Keyring is a Secrets backed by the OS keychain via zalando/go-keyring.
// The keyring calls are held as function fields so tests can stub them.
type Keyring struct {
	get func(service, user string) (string, error)
	set func(service, user, password string) error
	del func(service, user string) error
}

var _ Secrets = (*Keyring)(nil)

// NewKeyring returns a Keyring wired to the real OS keychain.
func NewKeyring() *Keyring {
	return &Keyring{get: keyring.Get, set: keyring.Set, del: keyring.Delete}
}

func (k *Keyring) GetAPIKey() (string, error) {
	v, err := k.get(keyringService, keyringAccount)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrAPIKeyNotFound
	}
	if err != nil {
		return "", fmt.Errorf("keyring get: %w", err)
	}
	return v, nil
}

func (k *Keyring) SetAPIKey(key string) error {
	if err := k.set(keyringService, keyringAccount, key); err != nil {
		return fmt.Errorf("keyring set: %w", err)
	}
	return nil
}

func (k *Keyring) DeleteAPIKey() error {
	err := k.del(keyringService, keyringAccount)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrAPIKeyNotFound
	}
	if err != nil {
		return fmt.Errorf("keyring delete: %w", err)
	}
	return nil
}

// MemorySecrets is an in-memory Secrets for tests (go-keyring has no macOS
// mock mode). Safe for concurrent use.
type MemorySecrets struct {
	mu     sync.Mutex
	key    string
	stored bool
}

var _ Secrets = (*MemorySecrets)(nil)

func (m *MemorySecrets) GetAPIKey() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.stored {
		return "", ErrAPIKeyNotFound
	}
	return m.key, nil
}

func (m *MemorySecrets) SetAPIKey(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.key, m.stored = key, true
	return nil
}

func (m *MemorySecrets) DeleteAPIKey() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.stored {
		return ErrAPIKeyNotFound
	}
	m.key, m.stored = "", false
	return nil
}
