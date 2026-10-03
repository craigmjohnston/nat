package plugins

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

// maxManifest caps how much of a manifest is read: a source with a hundred
// plugins is still a few tens of kilobytes, and anything past this is not a
// manifest at all.
const maxManifest = 1 << 20

// Manifest is a source's nat-plugins.json: the release it describes and the
// plugins that release carries. Unknown fields are ignored, so a later format
// can add to it.
type Manifest struct {
	Version string  `json:"version"`
	Plugins []Entry `json:"plugins"`
}

// Entry is one plugin a release carries: who it is, and the asset that is its
// binary with the digest that binary must have.
type Entry struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Asset       string `json:"asset"`
	SHA256      string `json:"sha256"`
}

// assetShape is a release asset's name: one path segment, no leading dot.
var assetShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// manifestURL is where a source's manifest is: its latest release's, or one
// version's where a version is asked for.
func (m *Manager) manifestURL(repo, version string) string {
	if version == "" {
		return fmt.Sprintf("%s/%s/releases/latest/download/%s", m.BaseURL, repo, manifestAsset)
	}
	return fmt.Sprintf("%s/%s/releases/download/v%s/%s", m.BaseURL, repo, version, manifestAsset)
}

// assetURL is where one of a release's binaries is.
func (m *Manager) assetURL(repo, version, asset string) string {
	return fmt.Sprintf("%s/%s/releases/download/v%s/%s", m.BaseURL, repo, version, asset)
}

// client is m.HTTP refusing any redirect off https. GitHub answers a release
// download with a redirect to its storage host, so redirects are followed —
// but a binary nat is about to run is never fetched in the clear, whatever a
// server says.
func (m *Manager) client() *http.Client {
	c := *m.HTTP
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("refused a redirect to %s://%s: plugins are fetched over https only", req.URL.Scheme, req.URL.Host)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &c
}

// get fetches url, answering its body for a 200 and an error naming the
// status for anything else. The body is never read into an error: a page
// served in place of a manifest says nothing nat should repeat.
func (m *Manager) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}

// ReadSource reads a source's manifest: its latest release's, or one
// version's. A read that fails — no release, no manifest, a manifest that will
// not parse or breaks a rule — is an error, never an empty manifest: a source
// that could not be read has said nothing about what it offers.
func (m *Manager) ReadSource(ctx context.Context, repo, version string) (Manifest, error) {
	ctx, cancel := context.WithTimeout(ctx, manifestTimeout)
	defer cancel()
	resp, err := m.get(ctx, m.manifestURL(repo, version))
	if err != nil {
		return Manifest{}, fmt.Errorf("read plugin source %s: %w", repo, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var man Manifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxManifest)).Decode(&man); err != nil {
		return Manifest{}, fmt.Errorf("read plugin source %s: malformed manifest", repo)
	}
	if err := validManifest(man); err != nil {
		return Manifest{}, fmt.Errorf("read plugin source %s: invalid manifest: %w", repo, err)
	}
	if version != "" && man.Version != version {
		return Manifest{}, fmt.Errorf("read plugin source %s: the v%s release's manifest says it is %s", repo, version, man.Version)
	}
	return man, nil
}

// validManifest holds a manifest to the rules every path and check built
// from it relies on.
func validManifest(man Manifest) error {
	if _, ok := dotted(man.Version); !ok {
		return fmt.Errorf("version %q is not a dotted version", man.Version)
	}
	seen := map[string]bool{}
	for _, e := range man.Plugins {
		if err := ValidName(e.Name); err != nil {
			return err
		}
		if seen[e.Name] {
			return fmt.Errorf("plugin %s is listed twice", e.Name)
		}
		seen[e.Name] = true
		if !assetShape.MatchString(e.Asset) {
			return fmt.Errorf("plugin %s: asset %q is not a file name", e.Name, e.Asset)
		}
		if b, err := hex.DecodeString(e.SHA256); err != nil || len(b) != 32 {
			return fmt.Errorf("plugin %s: sha256 %q is not a SHA-256 digest", e.Name, e.SHA256)
		}
	}
	return nil
}
