// SPDX-License-Identifier: GPL-3.0-or-later

package identify_test

import (
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/jobs/jobstest"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// refresh runs a refresh job for an item, as the queue would.
func (lib *testLibrary) refresh(itemID int64) {
	lib.t.Helper()
	job, err := identify.RefreshJob(itemID, "test", time.Time{}, false, 0)
	if err != nil {
		lib.t.Fatal(err)
	}
	err = lib.matcher.Handle(lib.t.Context(), jobs.Job{
		ID: 1, Kind: job.Kind, Target: jobs.Target{ItemID: itemID},
		ArgsJSON: job.ArgsJSON,
	})
	if err != nil {
		lib.t.Fatalf("refresh %d: %v", itemID, err)
	}
}

// fetchedAt is when an item's own stored response was last fetched.
func (lib *testLibrary) fetchedAt(itemID int64) int64 {
	lib.t.Helper()
	at, err := lib.database.Read().GetProviderResponseFetchedAt(lib.t.Context(),
		sqlcgen.GetProviderResponseFetchedAtParams{ItemID: itemID, Provider: "tmdb", Part: ""})
	if err != nil {
		lib.t.Fatal(err)
	}
	return at
}

// lastAskedAt is when TMDB was last asked for an item's stored parts: the part asked longest ago.
func (lib *testLibrary) lastAskedAt(itemID int64) int64 {
	lib.t.Helper()
	due, err := lib.database.Read().ListResponsesAskedBefore(lib.t.Context(), sqlcgen.ListResponsesAskedBeforeParams{
		Provider: "tmdb", Before: lib.clock.Now().UnixMilli() + 1,
	})
	if err != nil {
		lib.t.Fatal(err)
	}
	for _, item := range due {
		if item.ItemID == itemID {
			return item.OldestAskedAt
		}
	}
	lib.t.Fatalf("item %d has no stored TMDB response", itemID)
	return 0
}

// matchedMetropolis is a TMDB library holding Metropolis, matched.
func matchedMetropolis(t *testing.T) (*testLibrary, int64) {
	t.Helper()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(heat)}
	lib.scan(videoRecord("Metropolis (1995).mkv"))
	if outcome := lib.matchFileTitle("Metropolis (1995).mkv"); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s", outcome)
	}
	return lib, lib.titlesByPath()["Metropolis (1995).mkv"].TitleID
}

func TestRefresh_TakesTheProvidersNewValuesAndKeepsAPersonsOverride(t *testing.T) {
	t.Parallel()
	lib, titleID := matchedMetropolis(t)
	person := storetest.InsertUser(t, lib.database, "alice", "admin")
	err := lib.chooser.SetOverride(t.Context(), titleID, identify.FieldName, "Metropolis (Director's Cut)", person)
	if err != nil {
		t.Fatal(err)
	}
	changed := heat
	changed.Overview, changed.Name = "A thief and a detective.", "Metropolis, renamed at TMDB"
	lib.provider.Films[heat.ID] = changed
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
	if lib.fetchedAt(titleID) != lib.clock.Now().UnixMilli() {
		t.Error("the stored response was not fetched again")
	}
}

func TestRefresh_AnItemGoneFromItsProviderKeepsOurCopy(t *testing.T) {
	t.Parallel()
	lib, titleID := matchedMetropolis(t)
	fetchedAt := lib.fetchedAt(titleID)
	delete(lib.provider.Films, heat.ID)
	lib.clock.Advance(time.Hour)
	lib.refresh(titleID)
	file := lib.titlesByPath()["Metropolis (1995).mkv"]
	if titleName(file) != "Metropolis" || file.FollowedSource == nil || *file.FollowedSource != "tmdb" ||
		file.Filing != identify.FilingListed {
		t.Errorf("after TMDB dropped it: %+v named %q; want it kept as it was (D213 §4)", file, titleName(file))
	}
	if lib.lastAskedAt(titleID) != lib.clock.Now().UnixMilli() {
		t.Error("the attempt was not noted; the next sweep would ask again at once")
	}
	if lib.fetchedAt(titleID) != fetchedAt {
		t.Errorf("the kept copy's fetched time is %d, want %d: no body came, so its age is unchanged",
			lib.fetchedAt(titleID), fetchedAt)
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
	lib.provider.Seasons["615/2"][0].Name = "A Comet of My Own (renamed)"
	calls := len(lib.provider.Calls)
	lib.clock.Advance(time.Hour)
	// An episode's refresh is its series'.
	lib.refresh(lib.titlesByPath()[orbitoramaFiles[0]].TitleID)
	if got, want := lib.provider.Calls[calls:], []string{"fetch 615", "season 615/2"}; !slices.Equal(got, want) {
		t.Errorf("calls %v, want %v", got, want)
	}
	if clone := lib.titlesByPath()[orbitoramaFiles[0]]; titleName(clone) != "A Comet of My Own (renamed)" {
		t.Errorf("S02E10 is %q after the refresh", titleName(clone))
	}
}

func TestRefresher_SweepQueuesWhatIsDueSpreadOverTheLastQuarterOfThePeriod(t *testing.T) {
	t.Parallel()
	lib, titleID := matchedMetropolis(t)
	queue := &jobstest.Queue{}
	refresher := identify.NewRefresher(lib.database, queue, lib.settings, lib.clock, lib.logger)
	period := 180 * 24 * time.Hour
	due := identify.DueAfter(period, titleID)
	if due < period*3/4 || due > period {
		t.Fatalf("due after %v; want within the last quarter of %v", due, period)
	}
	fetched := time.UnixMilli(lib.fetchedAt(titleID))
	lib.clock.Set(fetched.Add(due - time.Minute))
	if count, err := refresher.Sweep(t.Context()); err != nil || count != 0 {
		t.Fatalf("a minute before it is due: queued %d (%v)", count, err)
	}
	lib.clock.Set(fetched.Add(due))
	if count, err := refresher.Sweep(t.Context()); err != nil || count != 1 || (*queue)[0].ItemID != titleID ||
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
