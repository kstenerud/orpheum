// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// refresh runs a refresh job for an item, as the queue would.
func (lib *world) refresh(itemID int64) {
	lib.t.Helper()
	job := RefreshJob(itemID, "test", time.Time{}, false, 0)
	err := lib.matcher.Handle(lib.t.Context(), jobs.Job{ID: 1, Kind: job.Kind, Subject: itemID, Args: job.Args})
	if err != nil {
		lib.t.Fatalf("refresh %d: %v", itemID, err)
	}
}

// fetchedAt is when an item's stored part was last fetched.
func (lib *world) fetchedAt(itemID int64, part string) int64 {
	lib.t.Helper()
	at, err := lib.database.Read().GetProviderResponseFetchedAt(lib.t.Context(),
		sqlcgen.GetProviderResponseFetchedAtParams{ItemID: itemID, Provider: "tmdb", Part: part})
	if err != nil {
		lib.t.Fatal(err)
	}
	return at
}

// matchedMetropolis is a TMDB library holding Metropolis, matched.
func matchedMetropolis(t *testing.T) (*world, int64) {
	t.Helper()
	lib := newWorld(t, "movies", "tmdb")
	lib.provider.searches[searchKey("Metropolis", 1995)] = []Candidate{lib.provider.add(heat)}
	lib.scan(video("Metropolis (1995).mkv"))
	if outcome := lib.match("Metropolis (1995).mkv"); outcome != OutcomeMatched {
		t.Fatalf("outcome %s", outcome)
	}
	return lib, lib.files()["Metropolis (1995).mkv"].TitleID
}

func TestRefresh_TakesTheProvidersNewValuesAndKeepsAPersonsOverride(t *testing.T) {
	t.Parallel()
	lib, titleID := matchedMetropolis(t)
	person := storetest.InsertUser(t, lib.database, "alice", "admin")
	if err := lib.chooser.SetOverride(t.Context(), titleID, fieldName, "Metropolis (Director's Cut)", person); err != nil {
		t.Fatal(err)
	}
	changed := heat
	changed.Overview, changed.Name = "A thief and a detective.", "Metropolis, renamed at TMDB"
	lib.provider.films[heat.ID] = changed
	lib.clock.Advance(time.Hour)
	lib.refresh(titleID)
	detail, err := lib.database.Read().GetTitleDetail(t.Context(), titleID)
	if err != nil {
		t.Fatal(err)
	}
	if ptr.ValueOrZero(detail.Overview) != changed.Overview || ptr.ValueOrZero(detail.Name) != "Metropolis (Director's Cut)" {
		t.Errorf("after the refresh: %q, %q; want TMDB's new overview and the person's name",
			ptr.ValueOrZero(detail.Name), ptr.ValueOrZero(detail.Overview))
	}
	if lib.fetchedAt(titleID, "") != lib.clock.Now().UnixMilli() {
		t.Error("the stored response was not fetched again")
	}
}

func TestRefresh_AnItemGoneFromItsProviderKeepsOurCopy(t *testing.T) {
	t.Parallel()
	lib, titleID := matchedMetropolis(t)
	delete(lib.provider.films, heat.ID)
	lib.clock.Advance(time.Hour)
	lib.refresh(titleID)
	file := lib.files()["Metropolis (1995).mkv"]
	if name(file) != "Metropolis" || file.Source == nil || *file.Source != "tmdb" || file.Filing != filingListed {
		t.Errorf("after TMDB dropped it: %+v named %q; want it kept as it was (D213 §4)", file, name(file))
	}
	if lib.fetchedAt(titleID, "") != lib.clock.Now().UnixMilli() {
		t.Error("the attempt was not noted; the next sweep would ask again at once")
	}
}

func TestRefresh_ASeriesFetchesItsRecordAndEachSeasonItKeeps(t *testing.T) {
	t.Parallel()
	lib := orbitoramaLibrary(t)
	lib.scanAll(orbitoramaFiles...)
	series := lib.onlySeries()
	if _, err := lib.matcher.Match(t.Context(), series.ID); err != nil {
		t.Fatal(err)
	}
	lib.provider.seasons["615/2"][0].Name = "A Comet of My Own (renamed)"
	calls := len(lib.provider.calls)
	lib.clock.Advance(time.Hour)
	// An episode's refresh is its series'.
	lib.refresh(lib.files()[orbitoramaFiles[0]].TitleID)
	if got, want := lib.provider.calls[calls:], []string{"fetch 615", "season 615/2"}; !slices.Equal(got, want) {
		t.Errorf("calls %v, want %v", got, want)
	}
	if clone := lib.files()[orbitoramaFiles[0]]; name(clone) != "A Comet of My Own (renamed)" {
		t.Errorf("S02E10 is %q after the refresh", name(clone))
	}
}

// queued is the job queue as a list.
type queued []jobs.New

func (queue *queued) Enqueue(_ context.Context, job jobs.New) (bool, error) {
	for _, open := range *queue {
		if open.Kind == job.Kind && open.Subject == job.Subject {
			return false, nil
		}
	}
	*queue = append(*queue, job)
	return true, nil
}

func TestRefresher_SweepQueuesWhatIsDueSpreadOverTheLastQuarterOfThePeriod(t *testing.T) {
	t.Parallel()
	lib, titleID := matchedMetropolis(t)
	queue := &queued{}
	refresher := NewRefresher(lib.database, queue, lib.settings, lib.clock, lib.matcher.logger)
	period := 180 * 24 * time.Hour
	due := dueAfter(period, titleID)
	if due < period*3/4 || due > period {
		t.Fatalf("due after %v; want within the last quarter of %v", due, period)
	}
	fetched := time.UnixMilli(lib.fetchedAt(titleID, ""))
	lib.clock.Set(fetched.Add(due - time.Minute))
	if count, err := refresher.Sweep(t.Context()); err != nil || count != 0 {
		t.Fatalf("a minute before it is due: queued %d (%v)", count, err)
	}
	lib.clock.Set(fetched.Add(due))
	if count, err := refresher.Sweep(t.Context()); err != nil || count != 1 || (*queue)[0].Subject != titleID ||
		(*queue)[0].Priority >= 0 {
		t.Fatalf("once due: queued %d %+v (%v); want Metropolis, behind everything else", count, *queue, err)
	}
	*queue = nil
	settingstest.Set(t, lib.settings, settings.TMDBRefreshAfterD, "0")
	if count, err := refresher.Sweep(t.Context()); err != nil || count != 0 {
		t.Errorf("with the period set to 0: queued %d (%v); want never", count, err)
	}
	if queued, err := refresher.Request(t.Context(), titleID); err != nil || !queued {
		t.Errorf("a person's request: %v, %v; they may refresh whenever they like", queued, err)
	}
}
