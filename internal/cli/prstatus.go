package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
)

// PRReader is what pr-status needs of the GitHub CLI: every pull request a
// repository currently has open. It names exactly the one gh call this
// command makes, the way [internal/tui.PRReader] does for the board's own
// background reading.
type PRReader interface {
	OpenPRs(dir string) (map[string]gh.PRStatus, error)
}

// prStatus prints the board's own PR-readiness reading, headlessly: every
// slice whose pull request is still worth watching, and how close it is to
// landing. It mirrors internal/tui/prstate.go's refreshPRStates — one gh
// listing per repository the plan spans, rather than one view per slice — so
// the reading costs the number of repositories rather than the number of
// pull requests the plan has ever produced.
func prStatus(ctx context.Context, args []string, env Env) error {
	asJSON, projectRef, err := parseJSONFlag("pr-status", args)
	if err != nil {
		return err
	}

	_, _, project, err := env.projectFor(projectRef)
	if err != nil {
		return err
	}
	client := env.NewClient(env.Tokens.Token)

	pages, err := client.QueryDataSource(ctx, project.SlicesDSID, nil, nil)
	if err != nil {
		return fmt.Errorf("load slices: %w", err)
	}
	slices := domain.SlicesFromPages(pages)

	readings := prReadings(env.NewGH(), slices, project)

	if asJSON {
		return writeJSON(env.Out, prStatusJSON(readings))
	}
	_, err = io.WriteString(env.Out, prStatusMarkdown(readings))
	return err
}

// prReading is one slice's pull request as pr-status reports it.
type prReading struct {
	SliceID   string
	SliceName string
	PR        string
	Readiness domain.PRReadiness
}

// worthReadingPR reports whether a slice has a pull request that anything
// might still be waiting on: the same rule internal/tui/prstate.go's
// worthReading applies. A slice with none has nothing to ask about, and one
// neither in progress nor Done has not got as far as producing one.
func worthReadingPR(s domain.Slice) bool {
	if s.PRURL == "" {
		return false
	}
	return s.Status == domain.SliceClaimed || s.Status == domain.SliceDone
}

// readinessOf turns what gh said about an open pull request into the reading
// pr-status reports, the same mapping prstate.go's readinessOf makes:
// approved and mergeable is the review over, and anything else is a review
// still to come.
func readinessOf(status gh.PRStatus) domain.PRReadiness {
	if status.Approved && status.Mergeable {
		return domain.PRReadyToMerge
	}
	return domain.PRAwaitingReview
}

// prReadings reads what GitHub says about the pull request of every slice
// worth asking about, one listing per repository, and reports a reading per
// slice in the plan's own order.
//
// A slice whose pull request is no longer open, or whose repository's
// listing could not be read at all, comes back with the zero
// [domain.PRReadiness] — unread — which is exactly how the board reads either
// case: nothing distinguishes them, because a pull request the reading never
// reached is worth exactly as much attention as one that has already landed.
// A repository whose listing fails is logged and left out, never guessed at.
func prReadings(reader PRReader, slices []domain.Slice, project config.ProjectConfig) []prReading {
	type read struct{ id, url string }
	var dirs []string
	reads := map[string][]read{}
	for _, s := range slices {
		if !worthReadingPR(s) {
			continue
		}
		dir := actions.WorkdirFor(s, project)
		if _, seen := reads[dir]; !seen {
			dirs = append(dirs, dir)
		}
		reads[dir] = append(reads[dir], read{id: s.ID, url: s.PRURL})
	}

	state := map[string]domain.PRReadiness{}
	for _, dir := range dirs {
		open, err := reader.OpenPRs(dir)
		if err != nil {
			logging.Action("left a repository's pull requests unread", "dir", dir, "error", err)
			continue
		}
		for _, r := range reads[dir] {
			if status, still := open[gh.NormaliseURL(r.url)]; still {
				state[r.id] = readinessOf(status)
			}
		}
	}

	var out []prReading
	for _, s := range slices {
		if !worthReadingPR(s) {
			continue
		}
		out = append(out, prReading{SliceID: s.ID, SliceName: s.Name, PR: s.PRURL, Readiness: state[s.ID]})
	}
	return out
}

// prStatusDoc is the structured form of the reading: one entry per slice worth
// watching, in the plan's own order.
type prStatusDoc struct {
	Slices []prStatusSliceJSON `json:"slices"`
}

type prStatusSliceJSON struct {
	SliceID   string `json:"slice_id"`
	Name      string `json:"name"`
	PR        string `json:"pr"`
	Readiness string `json:"readiness"`
}

// prStatusJSON maps the readings onto the structured form, in
// [domain.PRReadiness]'s own words, so a consumer reads the same vocabulary
// the board's own state does.
func prStatusJSON(readings []prReading) prStatusDoc {
	doc := prStatusDoc{Slices: make([]prStatusSliceJSON, 0, len(readings))}
	for _, r := range readings {
		doc.Slices = append(doc.Slices, prStatusSliceJSON{
			SliceID: r.SliceID, Name: r.SliceName, PR: r.PR, Readiness: r.Readiness.String(),
		})
	}
	return doc
}

// prStatusMarkdown renders the readings as a list, one line per slice.
func prStatusMarkdown(readings []prReading) string {
	out := "# Pull requests\n\n"
	if len(readings) == 0 {
		return out + "_none_\n"
	}
	for _, r := range readings {
		out += fmt.Sprintf("- %s — %s — %s\n", r.SliceName, r.Readiness, r.PR)
	}
	return out
}
