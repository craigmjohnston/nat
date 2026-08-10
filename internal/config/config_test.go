package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// handWritten mirrors the bootstrap config that was written by hand before
// this package existed; Load must tolerate it, including unknown fields.
const handWritten = `{
  "project_db_id": "3cbd79ca-8b6c-49a5-8f9e-fe960b825353",
  "project_db_data_source_id": "f7709ced-9231-49fd-b389-314ceb944103",
  "assignee_user_id": "578d8a0a-b778-46fb-8e0b-6dcedb314447",
  "assignee_user_name": "Craig Johnston",
  "active_project_id": "3b738308-f654-811c-948d-e1fb36f71df3",
  "some_future_field": true,
  "projects": {
    "3b738308-f654-811c-948d-e1fb36f71df3": {
      "name": "notion-agent-tracker",
      "slices_ds_id": "91fd1049-a9fd-4275-b9a1-364ceecc03b0",
      "milestones_ds_id": "f026fcfd-43a5-4a4c-8590-6d8600df802f",
      "working_dir": "/Users/craig/Projects/notion-agent-tracker"
    }
  }
}`

func sampleConfig() Config {
	return Config{
		ProjectDBID:           "db-id",
		ProjectDBDataSourceID: "db-ds-id",
		AssigneeUserID:        "user-id",
		AssigneeUserName:      "Craig Johnston",
		ActiveProjectID:       "proj-1",
		Projects: map[string]ProjectConfig{
			"proj-1": {
				Name:           "notion-agent-tracker",
				SlicesDSID:     "slices-ds",
				MilestonesDSID: "milestones-ds",
				WorkingDir:     "/tmp/work",
			},
		},
	}
}

func TestDir(t *testing.T) {
	tests := []struct {
		name    string
		xdg     string
		home    string
		want    string
		wantErr bool
	}{
		{name: "xdg set", xdg: "/xdg", home: "/home/u", want: "/xdg/notion-agent-tracker"},
		{name: "xdg unset falls back to home", xdg: "", home: "/home/u", want: "/home/u/.config/notion-agent-tracker"},
		{name: "xdg and home unset", xdg: "", home: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", tt.xdg)
			t.Setenv("HOME", tt.home)
			got, err := Dir()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Dir() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Dir() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Dir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	if want := "/xdg/notion-agent-tracker/config.json"; got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestPathError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if _, err := Path(); err == nil {
		t.Fatal("Path() succeeded, want error")
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, dir string)
		want      Config
		wantFound bool
		wantErr   string
	}{
		{
			name:  "missing file returns zero config for onboarding",
			setup: func(t *testing.T, dir string) {},
		},
		{
			name: "hand-written bootstrap file",
			setup: func(t *testing.T, dir string) {
				writeConfigFile(t, dir, handWritten)
			},
			want: Config{
				ProjectDBID:           "3cbd79ca-8b6c-49a5-8f9e-fe960b825353",
				ProjectDBDataSourceID: "f7709ced-9231-49fd-b389-314ceb944103",
				AssigneeUserID:        "578d8a0a-b778-46fb-8e0b-6dcedb314447",
				AssigneeUserName:      "Craig Johnston",
				ActiveProjectID:       "3b738308-f654-811c-948d-e1fb36f71df3",
				Projects: map[string]ProjectConfig{
					"3b738308-f654-811c-948d-e1fb36f71df3": {
						Name:           "notion-agent-tracker",
						SlicesDSID:     "91fd1049-a9fd-4275-b9a1-364ceecc03b0",
						MilestonesDSID: "f026fcfd-43a5-4a4c-8590-6d8600df802f",
						WorkingDir:     "/Users/craig/Projects/notion-agent-tracker",
					},
				},
			},
			wantFound: true,
		},
		{
			name: "invalid JSON",
			setup: func(t *testing.T, dir string) {
				writeConfigFile(t, dir, "{not json")
			},
			wantErr: "parse config",
		},
		{
			name: "unreadable file",
			setup: func(t *testing.T, dir string) {
				path := filepath.Join(dir, appDirName, configFileName)
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "read config",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", dir)
			tt.setup(t, dir)
			got, found, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if found != tt.wantFound {
				t.Errorf("Load() found = %v, want %v", found, tt.wantFound)
			}
			assertConfigEqual(t, got, tt.want)
		})
	}
}

func TestLoadPathError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if _, _, err := Load(); err == nil {
		t.Fatal("Load() succeeded, want error")
	}
}

func TestSaveRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	want := sampleConfig()
	if err := Save(want); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("config file mode = %o, want 644", perm)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("config file does not end with a newline")
	}

	got, found, err := Load()
	if err != nil {
		t.Fatalf("Load() after Save() error: %v", err)
	}
	if !found {
		t.Error("Load() after Save() found = false, want true")
	}
	assertConfigEqual(t, got, want)
}

func TestSplitPercent(t *testing.T) {
	tests := []struct {
		name string
		set  int
		want int
	}{
		{"unset", 0, DefaultSplitPercent},
		{"as configured", 50, 50},
		{"at the lower bound", minSplitPercent, minSplitPercent},
		{"at the upper bound", maxSplitPercent, maxSplitPercent},
		// A pane of a few columns, or one that leaves the board a few, is a
		// typo rather than an instruction.
		{"too narrow", minSplitPercent - 1, DefaultSplitPercent},
		{"too wide", maxSplitPercent + 1, DefaultSplitPercent},
		{"negative", -20, DefaultSplitPercent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{AgentSplitPercent: tt.set}
			if got := c.SplitPercent(); got != tt.want {
				t.Errorf("SplitPercent() = %d, want %d", got, tt.want)
			}
		})
	}
}

// The property is hand-written, so Load has to read it — and Save must not put
// a zero into a config that never set one, which would read as a broken split
// rather than an absent one.
func TestSplitPercentRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Save(sampleConfig()); err != nil {
		t.Fatal(err)
	}
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "agent_split_percent") {
		t.Errorf("an unset split was written out:\n%s", data)
	}

	set := sampleConfig()
	set.AgentSplitPercent = 75
	if err := Save(set); err != nil {
		t.Fatal(err)
	}
	got, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.SplitPercent() != 75 {
		t.Errorf("SplitPercent() = %d, want the configured 75", got.SplitPercent())
	}
}

func TestSaveErrors(t *testing.T) {
	t.Run("path error", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
		if err := Save(Config{}); err == nil {
			t.Fatal("Save() succeeded, want error")
		}
	})

	t.Run("mkdir error", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		if err := os.WriteFile(filepath.Join(dir, appDirName), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		err := Save(Config{})
		if err == nil || !strings.Contains(err.Error(), "create config dir") {
			t.Fatalf("Save() error = %v, want containing %q", err, "create config dir")
		}
	})

	t.Run("marshal error", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		orig := marshalIndent
		marshalIndent = func(v any, prefix, indent string) ([]byte, error) {
			return nil, errors.New("boom")
		}
		t.Cleanup(func() { marshalIndent = orig })
		err := Save(Config{})
		if err == nil || !strings.Contains(err.Error(), "encode config") {
			t.Fatalf("Save() error = %v, want containing %q", err, "encode config")
		}
	})

	t.Run("write error", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		appDir := filepath.Join(dir, appDirName)
		if err := os.MkdirAll(appDir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(appDir, 0o755) })
		err := Save(Config{})
		if err == nil || !strings.Contains(err.Error(), "write config") {
			t.Fatalf("Save() error = %v, want containing %q", err, "write config")
		}
	})
}

func writeConfigFile(t *testing.T, xdgDir, content string) {
	t.Helper()
	appDir := filepath.Join(xdgDir, appDirName)
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, configFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertConfigEqual(t *testing.T, got, want Config) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("config mismatch:\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}
