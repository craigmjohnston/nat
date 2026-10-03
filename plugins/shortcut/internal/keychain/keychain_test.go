package keychain

import (
	"errors"
	"reflect"
	"testing"
)

type fakeRunner struct {
	out   string
	err   error
	calls [][]string
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

// The real runner, run on harmless commands — never on security.
func TestExecRunner(t *testing.T) {
	out, err := ExecRunner{}.Output("echo", "hi")
	if err != nil || string(out) != "hi\n" {
		t.Errorf("Output = %q, %v", out, err)
	}
	if err := (ExecRunner{}).Interactive("true"); err != nil {
		t.Errorf("Interactive = %v", err)
	}
}
