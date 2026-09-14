package agent

import "testing"

func TestParseFrontend(t *testing.T) {
	for _, want := range []Frontend{"", FrontendTUI, FrontendGnat} {
		got, err := ParseFrontend(string(want))
		if err != nil {
			t.Errorf("ParseFrontend(%q): %v", want, err)
		}
		if got != want {
			t.Errorf("ParseFrontend(%q) = %q, want %q", want, got, want)
		}
	}
}

func TestParseFrontendRefusesAnythingElse(t *testing.T) {
	_, err := ParseFrontend("web")
	if err == nil {
		t.Fatal("ParseFrontend(\"web\"): expected an error")
	}
	if want := `--frontend must be "tui" or "gnat", given "web"`; err.Error() != want {
		t.Errorf("ParseFrontend(\"web\") error = %q, want %q", err.Error(), want)
	}
}
