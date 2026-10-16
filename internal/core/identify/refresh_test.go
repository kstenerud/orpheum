// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
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
	passes := 0
	lib.pass.AfterPass(func(context.Context, identify.Report) { passes++ })
	lib.refresh(titleID)
	if passes != 0 {
		t.Errorf("the refresh ran %d passes of its library; want it marked as owing one (D303, DL14)", passes)
	}
	if ran, err := lib.pass.IdentifyOwed(t.Context()); err != nil || ran != 1 {
		t.Errorf("the owed passes ran %d, %v; want the film's library's", ran, err)
	}
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
	lib.runOwedPasses()
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
	settingstest.Set(t, lib.settings, settings.TMDBRefreshAfterDays, "0")
	if count, err := refresher.Sweep(t.Context()); err != nil || count != 0 {
		t.Errorf("with the period set to 0: queued %d (%v); want never", count, err)
	}
	if queued, err := refresher.Request(t.Context(), titleID); err != nil || !queued {
		t.Errorf("a person's request: %v, %v; they may refresh whenever they like", queued, err)
	}
}

func TestRefresher_TheRollingRefreshPassesOverAnItemWithAFailureOfItsOwn(t *testing.T) {
	t.Parallel()
	lib, titleID := matchedMetropolis(t)
	queue := &jobstest.Queue{}
	refresher := identify.NewRefresher(lib.database, queue, lib.settings, lib.clock, lib.logger)
	job := jobs.Job{ID: 7, Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: titleID}}
	if err := lib.matcher.RecordFailure(t.Context(), job, "TMDB answered 500 five times"); err != nil {
		t.Fatal(err)
	}
	lib.clock.Advance(365 * 24 * time.Hour)
	if count, err := refresher.Sweep(t.Context()); err != nil || count != 0 {
		t.Errorf("a year on: queued %d (%v); want Metropolis left for a person to try again (D231 section 4)", count, err)
	}
}

// fakeImageRetrier records the items whose images a refresh asked for again.
type fakeImageRetrier struct{ itemIDs []int64 }

func (retrier *fakeImageRetrier) MarkImagesOwedAgain(_ context.Context, itemIDs ...int64) error {
	retrier.itemIDs = append(retrier.itemIDs, itemIDs...)
	return nil
}

func TestRefresh_ARefreshThatLooksAgainAsksForTheItemsImagesAgain(t *testing.T) {
	t.Parallel()
	lib, titleID := matchedMetropolis(t)
	retrier := &fakeImageRetrier{}
	lib.matcher.SetImageRetrier(retrier)
	lib.refresh(titleID)
	if len(retrier.itemIDs) != 0 {
		t.Fatalf("the rolling refresh asked again for the images of %v; want none", retrier.itemIDs)
	}
	job, err := identify.RefreshJob(titleID, "a person asked", time.Time{}, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = lib.matcher.Handle(t.Context(), jobs.Job{ID: 2, Kind: job.Kind, Target: job.Target, ArgsJSON: job.ArgsJSON})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(retrier.itemIDs, []int64{titleID}) {
		t.Errorf("a person's refresh asked again for the images of %v; want Metropolis's (D213 §2)", retrier.itemIDs)
	}
}

func TestRefresher_ARecheckIsARefreshThatAsksForTheImagesAgainDerivedBySweep(t *testing.T) {
	t.Parallel()
	lib, titleID := matchedMetropolis(t)
	queue := &jobstest.Queue{}
	refresher := identify.NewRefresher(lib.database, queue, lib.settings, lib.clock, lib.logger)
	if queued, err := refresher.QueueRechecks(t.Context(), []int64{titleID}); err != nil || queued != 1 {
		t.Fatalf("QueueRechecks = %d, %v; want Metropolis's refresh queued", queued, err)
	}
	if len(*queue) != 1 || (*queue)[0].Kind != jobs.KindFetchMetadata || (*queue)[0].ItemID != titleID ||
		!(*queue)[0].RunnableAt.IsZero() || !(*queue)[0].IsDerived {
		t.Fatalf("queued %+v; want Metropolis's refresh now, derived by the sweep rather than held as a job", *queue)
	}
	var args struct {
		IsRefresh         bool `json:"isRefresh"`
		ShouldRetryImages bool `json:"shouldRetryImages"`
	}
	if err := json.Unmarshal([]byte((*queue)[0].ArgsJSON), &args); err != nil || !args.IsRefresh ||
		!args.ShouldRetryImages {
		t.Errorf("the recheck's args %s (%v); want a refresh that asks for the images again (D213 §1)",
			(*queue)[0].ArgsJSON, err)
	}
}

func TestRefresher_AFirstMatchIsOwedUntilTheItemHasAnAnswerHoweverOftenItsJobIsLost(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(heat)}
	lib.provider.Searches[identifytest.SearchKey("Nothing", 2001)] = []identify.Candidate{}
	lib.scan(videoRecord("Metropolis (1995).mkv"), videoRecord("Nothing (2001).mkv"))
	metropolisID, nothingID := lib.titlesByPath()["Metropolis (1995).mkv"].TitleID, lib.titlesByPath()["Nothing (2001).mkv"].TitleID
	owed := func(when string, want ...int64) {
		t.Helper()
		queue := &jobstest.Queue{}
		refresher := identify.NewRefresher(lib.database, queue, lib.settings, lib.clock, lib.logger)
		if _, err := refresher.QueueFirstMatches(t.Context()); err != nil {
			t.Fatal(err)
		}
		var got []int64
		for _, job := range *queue {
			got = append(got, job.ItemID)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: first matches owed %v, want %v (D231 section 3)", when, got, want)
		}
	}
	// Queued twice: each sweep derives the work again, whatever became of the first job.
	owed("before any match", metropolisID, nothingID)
	owed("with the first jobs lost", metropolisID, nothingID)
	if outcome := lib.matchFileTitle("Metropolis (1995).mkv"); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s", outcome)
	}
	if outcome := lib.matchFileTitle("Nothing (2001).mkv"); outcome != identify.OutcomeNoMatch {
		t.Fatalf("outcome %s", outcome)
	}
	owed("once one is matched and the other answered as not found")
}

// onlyUnidentified is the one unidentified item the chooser lists.
func (lib *testLibrary) onlyUnidentified() identify.ItemSummary {
	lib.t.Helper()
	items, err := lib.chooser.Items(lib.t.Context(), auth.EveryLibrary(), identify.IdentificationUnidentified)
	if err != nil || len(items) != 1 {
		lib.t.Fatalf("unidentified items %+v (%v), want one", items, err)
	}
	return items[0]
}

func TestRecordFailure_AnItemsOwnFailureIsShownPassedOverAndClearedByAPersonsRefresh(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.scan(videoRecord("Metropolis (1995).mkv"))
	metropolisID := lib.titlesByPath()["Metropolis (1995).mkv"].TitleID
	job := jobs.Job{ID: 7, Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: metropolisID}}
	if err := lib.matcher.RecordFailure(t.Context(), job, "TMDB answered 500 five times"); err != nil {
		t.Fatal(err)
	}
	listed := lib.onlyUnidentified()
	if failure := listed.FetchFailure; failure == nil || failure.Failure != "TMDB answered 500 five times" ||
		!failure.FailedAt.Equal(lib.clock.Now()) {
		t.Fatalf("listed %+v; want Metropolis with its failure and when, for its page to show", listed)
	}
	queue := &jobstest.Queue{}
	refresher := identify.NewRefresher(lib.database, queue, lib.settings, lib.clock, lib.logger)
	if queued, sweepErr := refresher.QueueFirstMatches(t.Context()); sweepErr != nil || queued != 0 {
		t.Errorf("the sweep queued %d (%v) for an item with a failure of its own, want none", queued, sweepErr)
	}
	lib.clock.Advance(time.Minute)
	if _, err := refresher.Request(t.Context(), metropolisID); err != nil {
		t.Fatal(err)
	}
	listed = lib.onlyUnidentified()
	if listed.FetchFailure != nil || !listed.RefreshRequestedAt.Equal(lib.clock.Now()) {
		t.Errorf("after try again: %+v; want the failure cleared and the request shown", listed)
	}
	if len(*queue) != 1 || (*queue)[0].ItemID != metropolisID || (*queue)[0].Priority != jobs.PriorityPersonsRequest {
		t.Errorf("try again queued %+v; want Metropolis's fetch at a person's priority", *queue)
	}
}

func TestHandle_APersonsRefreshIsAFactWhicheverFetchRunsDoesAndAnswers(t *testing.T) {
	t.Parallel()
	lib, titleID := matchedMetropolis(t)
	retrier := &fakeImageRetrier{}
	lib.matcher.SetImageRetrier(retrier)
	queue := &jobstest.Queue{}
	refresher := identify.NewRefresher(lib.database, queue, lib.settings, lib.clock, lib.logger)
	if _, err := refresher.Request(t.Context(), titleID); err != nil {
		t.Fatal(err)
	}
	// cache.db is lost: the job is gone, and the sweep derives it again from the request.
	*queue = nil
	if queued, err := refresher.QueueRequested(t.Context()); err != nil || queued != 1 || !(*queue)[0].IsDerived {
		t.Fatalf("the sweep queued %d (%v): %+v; want the person's refresh again", queued, err, *queue)
	}
	// Whichever fetch runs, a rolling refresh that asks for no images or the derived one, does
	// what the person asked, and answers it.
	rolling, err := identify.RefreshJob(titleID, "due", time.Time{}, false, -10)
	if err != nil {
		t.Fatal(err)
	}
	err = lib.matcher.Handle(t.Context(), jobs.Job{
		ID: 3, Kind: rolling.Kind, Target: rolling.Target, ArgsJSON: rolling.ArgsJSON,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(retrier.itemIDs, []int64{titleID}) {
		t.Errorf("the fetch asked again for the images of %v; want Metropolis's, as the person asked", retrier.itemIDs)
	}
	*queue = nil
	if queued, err := refresher.QueueRequested(t.Context()); err != nil || queued != 0 {
		t.Errorf("after the fetch the sweep queued %d (%v); want the request answered", queued, err)
	}
}

func TestFetchArgsMerge_ARefreshFoldsIntoARefreshButNeverIntoAMatch(t *testing.T) {
	t.Parallel()
	const (
		match       = "{}"
		refresh     = `{"isRefresh":true}`
		imagesAgain = `{"isRefresh":true,"shouldRetryImages":true}`
	)
	merge := identify.FetchArgsMerge()
	for _, merging := range []struct{ queued, requested, want string }{
		{refresh, imagesAgain, imagesAgain},
		{imagesAgain, refresh, imagesAgain},
		{match, imagesAgain, match},
		{refresh, match, refresh},
	} {
		if got, err := merge.Merge(merging.queued, merging.requested); err != nil || got != merging.want {
			t.Errorf("merging %s into queued %s = %s, %v; want %s", merging.requested, merging.queued, got, err,
				merging.want)
		}
	}
	for _, covering := range []struct {
		running, requested string
		want               bool
	}{
		{refresh, refresh, true},
		{imagesAgain, refresh, true},
		{refresh, imagesAgain, false},
		{match, match, true},
		{match, refresh, false},
		{refresh, match, false},
	} {
		if got, err := merge.Covers(covering.running, covering.requested); err != nil || got != covering.want {
			t.Errorf("does running %s cover %s: %t, %v; want %t", covering.running, covering.requested, got, err,
				covering.want)
		}
	}
}
