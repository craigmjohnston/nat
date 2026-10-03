// Package cache keeps the plugin's last responses on disk, per project, so a
// sidebar polled every few seconds costs Shortcut one round of requests per
// TTL, and an API outage still draws the last tree it saw.
//
// It is best-effort throughout: a file that can't be read is no entry, and
// one that can't be written is simply not cached. Nothing here ever fails a
// call.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Cache is a directory of entries, one subdirectory per project.
type Cache struct {
	Dir string
	TTL time.Duration
	Now func() time.Time
}

type entry struct {
	At   time.Time       `json:"at"`
	Body json.RawMessage `json:"body"`
}

// hash names a file or directory after a key without trusting the key's
// characters — a project id or a search key could in principle carry a `/`.
func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:12])
}

func (c Cache) projectDir(project string) string { return filepath.Join(c.Dir, hash(project)) }

func (c Cache) path(project, key string) string {
	return filepath.Join(c.projectDir(project), hash(key)+".json")
}

// Get is the entry under project and key, and whether it is still within the
// TTL. ok is false when there is no readable entry at all.
func (c Cache) Get(project, key string) (body []byte, fresh, ok bool) {
	b, err := os.ReadFile(c.path(project, key))
	if err != nil {
		return nil, false, false
	}
	var e entry
	if json.Unmarshal(b, &e) != nil || len(e.Body) == 0 {
		return nil, false, false
	}
	return e.Body, c.Now().Sub(e.At) < c.TTL, true
}

// Put stores body (one JSON value) under project and key, stamped now.
func (c Cache) Put(project, key string, body []byte) {
	if os.MkdirAll(c.projectDir(project), 0o700) != nil {
		return
	}
	// body is a response the plugin itself just marshalled; the envelope
	// around it cannot fail to.
	b, _ := json.Marshal(entry{At: c.Now(), Body: body})
	path := c.path(project, key)
	if os.WriteFile(path+".tmp", b, 0o600) != nil {
		return
	}
	_ = os.Rename(path+".tmp", path)
}

// Drop forgets everything cached for project.
func (c Cache) Drop(project string) {
	_ = os.RemoveAll(c.projectDir(project))
}
