package logging

import (
	"errors"
	"strings"
	"testing"
)

func TestRedactTakesOutCredentials(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "authorization header",
			in:   `msg="request failed" header="Authorization: Bearer ntn_o_abc123"`,
			want: `msg="request failed" header="Authorization: Bearer [redacted]"`,
		},
		{
			name: "lower-case bearer",
			in:   "bearer secret_xyz",
			want: "Bearer [redacted]",
		},
		{
			name: "bare workspace token",
			in:   "token ntn_o_A1b2-c3_d4 was rejected",
			want: "token [redacted] was rejected",
		},
		{
			name: "legacy integration token",
			in:   "secret_ABCdef123 rejected",
			want: "[redacted] rejected",
		},
		{
			name: "two tokens in one line",
			in:   "ntn_one and ntn_two",
			want: "[redacted] and [redacted]",
		},
		{
			name: "nothing credential-shaped",
			in:   `msg="slice claimed" slice=3b838308`,
			want: `msg="slice claimed" slice=3b838308`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Redact(tt.in); got != tt.want {
				t.Errorf("Redact(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRedactWriterRedactsWhatItPassesOn(t *testing.T) {
	var sink strings.Builder
	w := redactWriter{w: &sink}
	line := "Bearer ntn_o_secret\n"

	n, err := w.Write([]byte(line))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The count is what the caller handed over, not what came out the other
	// side: a shortened line read as a short write would look like a failure.
	if n != len(line) {
		t.Errorf("n = %d, want %d", n, len(line))
	}
	if got := sink.String(); got != "Bearer [redacted]\n" {
		t.Errorf("wrote %q, want the credential taken out", got)
	}
}

func TestRedactWriterReportsAFailedWrite(t *testing.T) {
	sentinel := errors.New("disk full")

	n, err := redactWriter{w: failingWriter{err: sentinel}}.Write([]byte("anything"))
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want %v", err, sentinel)
	}
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
}

// failingWriter is an io.Writer that never manages to write anything.
type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

// The whole point of the writer is that a credential logged by accident does
// not reach the file, so this checks it through the package's own front door.
func TestOpenRedactsCredentialsOnTheirWayToTheFile(t *testing.T) {
	tempHome(t)
	closeAfter(t)

	path, err := Open()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	Error("notion request failed", "err", errors.New("401 with Bearer ntn_o_leaked"))
	if err := Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readFile(t, path)
	if strings.Contains(got, "ntn_o_leaked") {
		t.Errorf("log = %q, want the credential taken out", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Errorf("log = %q, want it to say something was taken out", got)
	}
}
