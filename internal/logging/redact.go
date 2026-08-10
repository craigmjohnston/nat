package logging

import (
	"io"
	"regexp"
)

// placeholder stands in for whatever was taken out, so a redacted line still
// reads as a line something was said on.
const placeholder = "[redacted]"

// redactions are the shapes a credential takes anywhere near this app: the
// Authorization header the Notion client sets, and the workspace tokens the
// Notion CLI hands out — `ntn_…` today, `secret_…` for integrations made before
// the rename.
//
// Nothing here is supposed to log a token in the first place; this is the
// second line, for the day something is logged whole by someone who did not
// know what was in it.
var redactions = []struct {
	pattern *regexp.Regexp
	with    string
}{
	// The quotes are excluded from the token so that a header quoted inside a
	// log record keeps the quote that closes it, rather than having it swallowed
	// along with the credential.
	{regexp.MustCompile(`(?i)bearer\s+[^\s"']+`), "Bearer " + placeholder},
	{regexp.MustCompile(`(?:ntn|secret)_[A-Za-z0-9_-]+`), placeholder},
}

// Redact replaces anything credential-shaped in s with [placeholder].
func Redact(s string) string {
	for _, r := range redactions {
		s = r.pattern.ReplaceAllString(s, r.with)
	}
	return s
}

// redactWriter passes each write through [Redact] on its way to w. slog hands a
// handler's output over one whole record per write, so a credential cannot be
// split across two calls and slip through between them.
type redactWriter struct{ w io.Writer }

// Write redacts p and writes what is left.
//
// It reports having written len(p) whatever the redaction did to the length: a
// caller comparing the count against what it handed over would read a shortened
// line as a short write, which is a failure, and this is not one.
func (r redactWriter) Write(p []byte) (int, error) {
	if _, err := r.w.Write([]byte(Redact(string(p)))); err != nil {
		return 0, err
	}
	return len(p), nil
}
