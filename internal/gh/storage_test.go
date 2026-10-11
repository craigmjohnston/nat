package gh

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// pathRunner answers each gh call by its last argument — the REST path —
// recording every call in order.
type pathRunner struct {
	answers map[string]pathAnswer
	calls   [][]string
	dirs    []string
}

type pathAnswer struct {
	out string
	err error
}

func (s *pathRunner) Run(dir, _ string, args ...string) (string, error) {
	s.calls = append(s.calls, args)
	s.dirs = append(s.dirs, dir)
	a, ok := s.answers[args[len(args)-1]]
	if !ok {
		return "", errors.New("unexpected call " + strings.Join(args, " "))
	}
	return a.out, a.err
}

const storageUserPath = "user"
const storageUsagePath = "users/octo/settings/billing/usage?year=2026&month=10"

// October has 744 hours: 744 GB-hours is one GB-month.
const storageReport = `{"usageItems":[
 {"product":"actions","sku":"Actions storage","quantity":372,"unitType":"GigabyteHours","repositoryName":"Octo/App"},
 {"product":"actions","sku":"Actions storage","quantity":372,"unitType":"GigabyteHours","repositoryName":"octo/app"},
 {"product":"actions","sku":"Actions storage","quantity":74.4,"unitType":"GigabyteHours","repositoryName":"site"},
 {"product":"actions","sku":"Actions storage","quantity":0,"unitType":"GigabyteHours","repositoryName":"octo/empty"},
 {"product":"actions","sku":"Actions cache storage","quantity":9999,"unitType":"GigabyteHours","repositoryName":"octo/app"},
 {"product":"actions","sku":"Actions Linux","quantity":120,"unitType":"Minutes","repositoryName":"octo/app"}
]}`

func storageNowFixture() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) }

func TestArtifactStorageSumsGBHoursIntoGBMonthsPerRepository(t *testing.T) {
	r := &pathRunner{answers: map[string]pathAnswer{
		storageUserPath:  {out: `{"login":"octo","plan":{"name":"Pro"}}`},
		storageUsagePath: {out: storageReport},
	}}

	got, err := NewWithRunner(r).ArtifactStorage(storageNowFixture())
	if err != nil {
		t.Fatalf("ArtifactStorage: %v", err)
	}
	if got.Login != "octo" || got.Plan != "pro" || got.AllowanceGB != 1 {
		t.Errorf("account = %q %q %v, want octo pro 1", got.Login, got.Plan, got.AllowanceGB)
	}
	if len(got.Repos) != 2 {
		t.Fatalf("repos = %v, want octo/app and octo/site alone", got.Repos)
	}
	if math.Abs(got.Repos["octo/app"]-1) > 1e-9 {
		t.Errorf("octo/app = %v GB-months, want 1 (cache and minutes left out)", got.Repos["octo/app"])
	}
	if math.Abs(got.Repos["octo/site"]-0.1) > 1e-9 {
		t.Errorf("octo/site = %v GB-months, want 0.1", got.Repos["octo/site"])
	}
	for i, d := range r.dirs {
		if d != "" {
			t.Errorf("call %d ran in %q, want no directory", i, d)
		}
	}
	if want := []string{"api", storageUsagePath}; strings.Join(r.calls[1], " ") != strings.Join(want, " ") {
		t.Errorf("second call = %v, want %v", r.calls[1], want)
	}
}

func TestArtifactStorageAllowanceByPlan(t *testing.T) {
	for _, tc := range []struct {
		user string
		plan string
		gb   float64
	}{
		{`{"login":"octo","plan":{"name":"free"}}`, "free", 0.5},
		{`{"login":"octo","plan":{"name":"team"}}`, "team", 2},
		{`{"login":"octo","plan":{"name":"enterprise"}}`, "enterprise", 0},
		{`{"login":"octo"}`, "", 0},
	} {
		r := &pathRunner{answers: map[string]pathAnswer{
			storageUserPath:  {out: tc.user},
			storageUsagePath: {out: `{"usageItems":[]}`},
		}}
		got, err := NewWithRunner(r).ArtifactStorage(storageNowFixture())
		if err != nil {
			t.Fatalf("%s: %v", tc.user, err)
		}
		if got.Plan != tc.plan || got.AllowanceGB != tc.gb {
			t.Errorf("%s: plan %q allowance %v, want %q %v", tc.user, got.Plan, got.AllowanceGB, tc.plan, tc.gb)
		}
	}
}

func TestArtifactStorageNamesTheMissingScope(t *testing.T) {
	scope := &ExitError{Code: 1, Stderr: "gh: Not Found (HTTP 404)\ngh: This API operation needs the \"user\" scope. To request it, run:  gh auth refresh -h github.com -s user\n"}
	for _, path := range []string{storageUserPath, storageUsagePath} {
		answers := map[string]pathAnswer{
			storageUserPath:  {out: `{"login":"octo"}`},
			storageUsagePath: {out: `{"usageItems":[]}`},
		}
		answers[path] = pathAnswer{err: scope}
		_, err := NewWithRunner(&pathRunner{answers: answers}).ArtifactStorage(storageNowFixture())
		if !errors.Is(err, ErrBillingScope) {
			t.Errorf("refused at %s: err = %v, want ErrBillingScope", path, err)
		}
	}
}

func TestArtifactStorageRefusals(t *testing.T) {
	other := &ExitError{Code: 1, Stderr: "gh: Bad credentials (HTTP 401)\n"}
	for name, tc := range map[string]struct {
		user, usage pathAnswer
		want        string
	}{
		"user refused":   {user: pathAnswer{err: other}, want: "Bad credentials"},
		"no account":     {user: pathAnswer{out: "{}"}, want: "no account"},
		"report refused": {user: pathAnswer{out: `{"login":"octo"}`}, usage: pathAnswer{err: other}, want: "Bad credentials"},
		"report garbled": {user: pathAnswer{out: `{"login":"octo"}`}, usage: pathAnswer{out: "<html>"}, want: "billing report"},
		"another unit": {
			user:  pathAnswer{out: `{"login":"octo"}`},
			usage: pathAnswer{out: `{"usageItems":[{"product":"actions","sku":"Actions storage","quantity":3,"unitType":"GigabyteMonths","repositoryName":"octo/app"}]}`},
			want:  "GigabyteMonths",
		},
	} {
		r := &pathRunner{answers: map[string]pathAnswer{storageUserPath: tc.user, storageUsagePath: tc.usage}}
		_, err := NewWithRunner(r).ArtifactStorage(storageNowFixture())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want one naming %q", name, err, tc.want)
		}
	}
}

func TestHoursInMonth(t *testing.T) {
	for _, tc := range []struct {
		t    time.Time
		want int
	}{
		{time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC), 744},
		{time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), 672},
		{time.Date(2028, 2, 15, 0, 0, 0, 0, time.UTC), 696},
		{time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), 720},
	} {
		if got := HoursInMonth(tc.t); got != tc.want {
			t.Errorf("HoursInMonth(%v) = %d, want %d", tc.t, got, tc.want)
		}
	}
}

func TestFormatGB(t *testing.T) {
	if got := FormatGB(0.125); got != "0.12" && got != "0.13" {
		t.Errorf("FormatGB(0.125) = %q", got)
	}
	if got := FormatGB(1); got != "1.00" {
		t.Errorf("FormatGB(1) = %q, want 1.00", got)
	}
}
