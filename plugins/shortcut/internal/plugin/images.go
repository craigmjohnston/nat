package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/craigmjohnston/nat/internal/source"
)

// imagePattern finds a Markdown image's link: `![alt](url)`, an optional
// `"title"` after the url.
var imagePattern = regexp.MustCompile(`!\[[^\]]*\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)`)

// imageURLs are the links of every Markdown image in texts, each once, in the
// order they first appear.
func imageURLs(texts ...string) []string {
	var out []string
	for _, t := range texts {
		for _, m := range imagePattern.FindAllStringSubmatch(t, -1) {
			if !slices.Contains(out, m[1]) {
				out = append(out, m[1])
			}
		}
	}
	return out
}

// imageExts names a copy's extension after its media type, for an image whose
// link has none; anything else is saved with none.
var imageExts = map[string]string{
	"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif",
	"image/webp": ".webp", "image/svg+xml": ".svg", "image/heic": ".heic",
}

// imageExt is a copy's extension: its media type's, else its link's where
// that is short enough to be one, else none.
func imageExt(kind, u string) string {
	if ext, ok := imageExts[kind]; ok {
		return ext
	}
	if e := path.Ext(u); len(e) > 1 && len(e) <= 5 {
		return e
	}
	return ""
}

// images saves a copy of every image in texts that only the token can read
// — the workspace's own uploads — under the cache directory's images/, and
// says where each is. A copy already there is not fetched again (an upload
// never changes under its link); one that cannot be fetched is left out, and
// gnat draws its placeholder. Copies are fetched in parallel and never fail
// the read.
func (a *app) images(ctx context.Context, texts ...string) []source.Image {
	dir := filepath.Join(a.dirs.CacheDir(), "images")
	var urls []string
	for _, u := range imageURLs(texts...) {
		if a.sc.Holds(u) {
			urls = append(urls, u)
		}
	}
	found := make([]string, len(urls))
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Go(func() { found[i] = a.image(ctx, dir, u) })
	}
	wg.Wait()
	var out []source.Image
	for i, u := range urls {
		if found[i] != "" {
			out = append(out, source.Image{URL: u, File: found[i]})
		}
	}
	return out
}

// image is the copy of one image under dir, fetched where there is none yet,
// or "" where it can't be had.
func (a *app) image(ctx context.Context, dir, u string) string {
	sum := sha256.Sum256([]byte(u))
	name := hex.EncodeToString(sum[:16])
	matches, _ := filepath.Glob(filepath.Join(dir, name+"*"))
	for _, m := range matches {
		if !strings.HasSuffix(m, ".tmp") {
			return m
		}
	}
	b, kind, err := a.sc.Fetch(ctx, u)
	if err != nil {
		return ""
	}
	if os.MkdirAll(dir, 0o700) != nil {
		return ""
	}
	file := filepath.Join(dir, name+imageExt(kind, u))
	if os.WriteFile(file+".tmp", b, 0o600) != nil || os.Rename(file+".tmp", file) != nil {
		_ = os.Remove(file + ".tmp")
		return ""
	}
	return file
}
