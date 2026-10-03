package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetPutDrop(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	c := Cache{Dir: t.TempDir(), TTL: 30 * time.Second, Now: func() time.Time { return now }}
	if _, _, ok := c.Get("p", "k"); ok {
		t.Fatal("empty cache had an entry")
	}
	c.Put("p", "k", []byte(`{"a":1}`))
	body, fresh, ok := c.Get("p", "k")
	if !ok || !fresh || string(body) != `{"a":1}` {
		t.Fatalf("Get = %s, %v, %v", body, fresh, ok)
	}
	if _, _, ok := c.Get("p", "other"); ok {
		t.Error("another key hit")
	}
	if _, _, ok := c.Get("q", "k"); ok {
		t.Error("another project hit")
	}
	now = now.Add(30 * time.Second)
	if _, fresh, ok := c.Get("p", "k"); !ok || fresh {
		t.Errorf("at the TTL: fresh=%v ok=%v, want stale", fresh, ok)
	}
	c.Put("q", "k", []byte(`1`))
	c.Drop("p")
	if _, _, ok := c.Get("p", "k"); ok {
		t.Error("dropped entry still there")
	}
	if _, _, ok := c.Get("q", "k"); !ok {
		t.Error("Drop took another project's entry")
	}
}

func TestCorruptAndUnwritable(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	c := Cache{Dir: dir, TTL: time.Minute, Now: func() time.Time { return now }}
	_ = os.MkdirAll(c.projectDir("p"), 0o700)
	_ = os.WriteFile(c.path("p", "k"), []byte("not json"), 0o600)
	if _, _, ok := c.Get("p", "k"); ok {
		t.Error("corrupt entry read")
	}
	_ = os.WriteFile(c.path("p", "k"), []byte(`{"at":"2026-01-01T00:00:00Z"}`), 0o600)
	if _, _, ok := c.Get("p", "k"); ok {
		t.Error("bodiless entry read")
	}

	// A file where the cache dir should be: nothing is cached, nothing fails.
	blocked := Cache{Dir: filepath.Join(dir, "file"), TTL: time.Minute, Now: c.Now}
	_ = os.WriteFile(blocked.Dir, nil, 0o600)
	blocked.Put("p", "k", []byte(`1`))
	if _, _, ok := blocked.Get("p", "k"); ok {
		t.Error("cached under a file")
	}
	// A directory where the temp file goes.
	_ = os.MkdirAll(c.path("p", "k2")+".tmp", 0o700)
	c.Put("p", "k2", []byte(`1`))
	if _, _, ok := c.Get("p", "k2"); ok {
		t.Error("cached past an unwritable temp file")
	}
}
