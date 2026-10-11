package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// StorageReading is one month of the signed-in user's GitHub artifact
// storage, as GitHub's own billing usage report puts it: what each
// repository holds, in GB-months, and what the plan includes.
type StorageReading struct {
	// Login is the account the report is for.
	Login string
	// Plan is GitHub's name for the account's plan, lower-cased ("free",
	// "pro"); empty where GitHub named none.
	Plan string
	// AllowanceGB is the storage the plan includes, in GB; zero where the
	// plan is not one [planStorageGB] knows.
	AllowanceGB float64
	// Repos is each repository's storage this month, in GB-months, by its
	// "owner/name" lower-cased as [ParseRemote] names one. A repository
	// GitHub listed with no storage is absent.
	Repos map[string]float64
}

// planStorageGB is the artifact storage each plan includes, in GB, from
// GitHub's "About billing for GitHub Actions" (October 2026). Actions caches
// are counted apart and are not in it.
var planStorageGB = map[string]float64{
	"free": 0.5,
	"pro":  1,
	"team": 2,
}

// ErrBillingScope is the refusal of a gh signed in without the "user" scope,
// which GitHub requires to read a user's billing report or plan. gh says so
// under a bare "Not Found (HTTP 404)", which tells the user nothing.
var ErrBillingScope = errors.New(`GitHub's billing report needs gh's "user" scope: run ` + ScopeCommand)

// ScopeCommand is what gives gh the "user" scope: the user runs it, signing
// in through GitHub's device flow; nat never does.
const ScopeCommand = "gh auth refresh -h github.com -s user"

// ArtifactStorage reads the signed-in user's artifact storage for the month
// of now, in GitHub's own terms: the billing usage report
// (users/{login}/settings/billing/usage) for that month, each artifact
// storage item's GB-hours summed per repository and divided by the hours in
// the month — how GitHub turns GB-hours into the GB-months its billing page
// shows — and the plan, off `gh api user`. Two REST calls, run in no
// directory: the report names its repositories itself.
func (c CLI) ArtifactStorage(now time.Time) (StorageReading, error) {
	out, err := c.runner.Run("", Binary, "api", "user")
	if err != nil {
		return StorageReading{}, billingError(err)
	}
	var user struct {
		Login string `json:"login"`
		Plan  *struct {
			Name string `json:"name"`
		} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(out), &user); err != nil || user.Login == "" {
		return StorageReading{}, fmt.Errorf("gh api user gave no account: %q", firstLine(out))
	}
	r := StorageReading{Login: user.Login, Repos: map[string]float64{}}
	if user.Plan != nil {
		r.Plan = strings.ToLower(strings.TrimSpace(user.Plan.Name))
		r.AllowanceGB = planStorageGB[r.Plan]
	}

	now = now.UTC()
	path := fmt.Sprintf("users/%s/settings/billing/usage?year=%d&month=%d",
		url.PathEscape(user.Login), now.Year(), int(now.Month()))
	out, err = c.runner.Run("", Binary, "api", path)
	if err != nil {
		return StorageReading{}, billingError(err)
	}
	var report struct {
		UsageItems []struct {
			Product        string  `json:"product"`
			SKU            string  `json:"sku"`
			Quantity       float64 `json:"quantity"`
			UnitType       string  `json:"unitType"`
			RepositoryName string  `json:"repositoryName"`
		} `json:"usageItems"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		return StorageReading{}, fmt.Errorf("read GitHub's billing report: %w", err)
	}
	hours := float64(HoursInMonth(now))
	for _, item := range report.UsageItems {
		if !isArtifactStorage(item.Product, item.SKU) || item.Quantity == 0 {
			continue
		}
		if !strings.EqualFold(item.UnitType, "GigabyteHours") {
			return StorageReading{}, fmt.Errorf("GitHub reported artifact storage in %q, not GB-hours", item.UnitType)
		}
		r.Repos[repoKey(user.Login, item.RepositoryName)] += item.Quantity / hours
	}
	return r, nil
}

// isArtifactStorage is whether a billing report item is Actions artifact
// storage — never an Actions cache, which GitHub bills apart. GitHub's report
// names it product "actions", SKU "Actions storage" (read October 2026; the
// docs name neither).
func isArtifactStorage(product, sku string) bool {
	return strings.EqualFold(product, "actions") && strings.EqualFold(sku, "Actions storage")
}

// repoKey is a report item's repository as [ParseRemote] names one: the
// report gives a user's own repositories by bare name ("nat", read October
// 2026), so the login is put before one with no owner.
func repoKey(login, name string) string {
	if !strings.Contains(name, "/") {
		name = login + "/" + name
	}
	return strings.ToLower(name)
}

// HoursInMonth is the number of hours in the (UTC) month of t — what GitHub
// divides a month's GB-hours by to give GB-months.
func HoursInMonth(t time.Time) int {
	t = t.UTC()
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return int(first.AddDate(0, 1, 0).Sub(first) / time.Hour)
}

// billingError is a refused billing read in words the user can act on: the
// missing scope by name, else gh's own first line.
func billingError(err error) error {
	var exit *ExitError
	if errors.As(err, &exit) && strings.Contains(exit.Stderr, `"user" scope`) {
		return ErrBillingScope
	}
	return err
}

// FormatGB is a storage figure as gnat and the plain output print it: two
// decimals, which is what GitHub's billing page shows.
func FormatGB(gb float64) string { return strconv.FormatFloat(gb, 'f', 2, 64) }
