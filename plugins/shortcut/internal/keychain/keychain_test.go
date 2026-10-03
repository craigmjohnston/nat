package keychain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	out   string
	err   error
	calls [][]string
	stdin []string
}

func (f *fakeRunner) Feed(stdin string, name string, args ...string) error {
	f.calls = append(f.calls, append([]string{name}, args...))
	f.stdin = append(f.stdin, stdin)
	return f.err
}

func (f *fakeRunner) Output(name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return []byte(f.out), f.err
}

func (f *fakeRunner) Interactive(name string, args ...string) error {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.err
}

func TestToken(t *testing.T) {
	r := &fakeRunner{out: "tok-123\n"}
	got, err := Keychain{Run: r}.Token()
	if err != nil || got != "tok-123" {
		t.Fatalf("Token = %q, %v", got, err)
	}
	want := [][]string{{"security", "find-generic-password", "-s", "nat-source-shortcut", "-w"}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %v", r.calls)
	}
	for _, r := range []*fakeRunner{{err: errors.New("exit status 44")}, {out: "  \n"}} {
		if _, err := (Keychain{Run: r}).Token(); !errors.Is(err, ErrNoToken) {
			t.Errorf("Token err = %v, want ErrNoToken", err)
		}
	}
}

// TestHas: a presence check, never a read — no -w in argv — and a failure
// is simply no token.
func TestHas(t *testing.T) {
	r := &fakeRunner{out: "keychain: \"login.keychain-db\"\n"}
	if !(Keychain{Run: r}).Has() {
		t.Error("Has = false for a stored item")
	}
	want := [][]string{{"security", "find-generic-password", "-s", "nat-source-shortcut"}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
	if (Keychain{Run: &fakeRunner{err: errors.New("exit status 44")}}).Has() {
		t.Error("Has = true for a failed lookup")
	}
}

func TestStore(t *testing.T) {
	r := &fakeRunner{}
	if err := (Keychain{Run: r}).Store("craig"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"security", "add-generic-password", "-U", "-s", "nat-source-shortcut", "-a", "craig", "-w"}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %v", r.calls)
	}
	r.err = errors.New("boom")
	if err := (Keychain{Run: r}).Store("craig"); err == nil {
		t.Error("Store swallowed the error")
	}
}

// TestSave: the token reaches security on stdin, as one quoted
// add-generic-password line for `security -i`, and never in its argv.
func TestSave(t *testing.T) {
	r := &fakeRunner{}
	const tok = `t0k "with" \ odd bits`
	if err := (Keychain{Run: r}).Save("craig j", tok); err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"security", "-i"}}; !reflect.DeepEqual(r.calls, want) {
		t.Errorf("argv = %v, want %v", r.calls, want)
	}
	for _, arg := range r.calls[0] {
		if strings.Contains(arg, "t0k") {
			t.Errorf("the token is in argv: %v", r.calls[0])
		}
	}
	want := `add-generic-password -U -s "nat-source-shortcut" -a "craig j" -w "t0k \"with\" \\ odd bits"` + "\n"
	if len(r.stdin) != 1 || r.stdin[0] != want {
		t.Errorf("stdin = %q, want %q", r.stdin, want)
	}

	r.err = errors.New("exit status 50")
	if err := (Keychain{Run: r}).Save("craig", "tok"); err == nil {
		t.Error("Save swallowed security's failure")
	}
	// A line break would end security's command and start another, so
	// nothing with a control character in it is run at all.
	r = &fakeRunner{}
	for _, c := range [][2]string{{"craig", "tok\nadd-internet-password"}, {"cr\raig", "tok"}, {"craig", "tok\x00"}} {
		if err := (Keychain{Run: r}).Save(c[0], c[1]); !errors.Is(err, ErrUnstorable) {
			t.Errorf("Save(%q, %q) = %v, want ErrUnstorable", c[0], c[1], err)
		}
	}
	if len(r.calls) != 0 {
		t.Errorf("security ran for an unstorable token: %v", r.calls)
	}
}

// The real runner, run on harmless commands — never on security.
func TestExecRunner(t *testing.T) {
	out, err := ExecRunner{}.Output("echo", "hi")
	if err != nil || string(out) != "hi\n" {
		t.Errorf("Output = %q, %v", out, err)
	}
	if err := (ExecRunner{}).Interactive("true"); err != nil {
		t.Errorf("Interactive = %v", err)
	}
	// Feed hands the command its stdin: grep exits 0 only if it read the line.
	if err := (ExecRunner{}).Feed("fed line\n", "grep", "-q", "^fed line$"); err != nil {
		t.Errorf("Feed = %v", err)
	}
	if err := (ExecRunner{}).Feed("other\n", "grep", "-q", "^fed line$"); err == nil {
		t.Error("Feed swallowed a non-zero exit")
	}
}
