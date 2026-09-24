// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// fakeProvider is TMDB behind the provider interface (standards/testing.md, Network): films by
// id, search results by name and year, and IMDb ids. Its stored body is the Metadata as JSON.
type fakeProvider struct {
	films    map[string]Metadata
	searches map[string][]Candidate
	imdb     map[string]string
	seasons  map[string][]Episode // by "series/season"
	fail     error
	calls    []string
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{
		films: map[string]Metadata{}, searches: map[string][]Candidate{}, imdb: map[string]string{},
		seasons: map[string][]Episode{},
	}
}

func (fake *fakeProvider) FetchSeason(_ context.Context, seriesID string, season int) ([]byte, error) {
	key := fmt.Sprintf("%s/%d", seriesID, season)
	fake.calls = append(fake.calls, "season "+key)
	if fake.fail != nil {
		return nil, fake.fail
	}
	episodes, found := fake.seasons[key]
	if !found {
		return nil, ErrNoMatch
	}
	return json.Marshal(episodes) //nolint:wrapcheck,musttag // a test fake stores Episodes as Go names them
}

func (*fakeProvider) ParseSeason(body []byte) ([]Episode, error) {
	var episodes []Episode
	err := json.Unmarshal(body, &episodes) //nolint:musttag // as the fake stored it
	return episodes, err                   //nolint:wrapcheck // a test fake
}

func searchKey(name string, year int) string { return fmt.Sprintf("%s|%d", name, year) }

func (*fakeProvider) Name() string { return "tmdb" }

func (fake *fakeProvider) Search(_ context.Context, query Query) ([]Candidate, error) {
	fake.calls = append(fake.calls, "search "+searchKey(query.Name, query.Year))
	if fake.fail != nil {
		return nil, fake.fail
	}
	return fake.searches[searchKey(query.Name, query.Year)], nil
}

func (fake *fakeProvider) Find(_ context.Context, _, source, id string) (string, error) {
	fake.calls = append(fake.calls, "find "+source+" "+id)
	if found, ok := fake.imdb[id]; ok {
		return found, nil
	}
	return "", ErrNoMatch
}

func (fake *fakeProvider) Fetch(_ context.Context, _, id string) ([]byte, error) {
	fake.calls = append(fake.calls, "fetch "+id)
	film, found := fake.films[id]
	if !found {
		return nil, ErrNoMatch
	}
	return json.Marshal(film) //nolint:wrapcheck,musttag // a test fake stores Metadata as Go names it
}

func (*fakeProvider) Parse(_ string, body []byte) (Metadata, error) {
	var film Metadata
	err := json.Unmarshal(body, &film) //nolint:musttag // as the fake stored it
	return film, err                   //nolint:wrapcheck // a test fake
}

func (fake *fakeProvider) add(film Metadata) Candidate {
	fake.films[film.ID] = film
	return Candidate{ID: film.ID, Name: film.Name, OriginalName: film.OriginalName, Year: film.Year}
}

var (
	leon = Metadata{
		ID: "101", Name: "Variety", OriginalName: "Variety", Year: 1994,
		RuntimeMs: 110 * 60_000, Overview: "A hitman takes in a girl.", IDs: map[string]string{"imdb": "tt0345678"},
	}
	heat = Metadata{ID: "9019", Name: "Metropolis", OriginalName: "Metropolis", Year: 1995, RuntimeMs: 170 * 60_000}
)

// match runs the matcher on the title of the file at rel.
func (lib *world) match(rel string) Outcome {
	lib.t.Helper()
	file, found := lib.files()[rel]
	if !found {
		lib.t.Fatalf("no file %s", rel)
	}
	outcome, err := lib.matcher.Match(lib.t.Context(), file.TitleID)
	if err != nil {
		lib.t.Fatalf("Match(%s): %v", rel, err)
	}
	return outcome
}

func TestMatch_ASingleResultMatchingNameAndYearIsAcceptedAndReplayedFromTheStoredResponse(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "tmdb")
	lib.provider.searches[searchKey("Variety", 1994)] = []Candidate{
		lib.provider.add(leon), {ID: "7", Name: "Léon Morin, Priest", Year: 1961},
	}
	const rel = "Variety (1994)/Variety (1994).mkv"
	report := lib.scan(video(rel))
	if len(report.AwaitingMatch) != 1 || lib.files()[rel].Filing != filingUnidentified {
		t.Fatalf("before matching: %+v, file %+v; want it awaiting its match", report, lib.files()[rel])
	}
	if outcome := lib.match(rel); outcome != OutcomeMatched {
		t.Fatalf("outcome %s, want matched (its original name and year are the file's)", outcome)
	}
	checkLeon := func(when string) {
		t.Helper()
		file := lib.files()[rel]
		if name(file) != leon.Name || file.Ident != identIdentified || file.Source == nil || *file.Source != "tmdb" ||
			file.Filing != filingListed || file.Year == nil || *file.Year != 1994 {
			t.Errorf("%s: %+v (%s), want TMDB's film, listed", when, file, name(file))
		}
	}
	checkLeon("after the match")
	calls := len(lib.provider.calls)
	report = lib.scan(video(rel))
	checkLeon("after another pass")
	if len(lib.provider.calls) != calls || len(report.AwaitingMatch) != 0 {
		t.Errorf("another pass called the provider (%v) or queued the film again (%v)", lib.provider.calls[calls:],
			report.AwaitingMatch)
	}
}

func TestMatch_AnAmbiguousSearchIsNeverGuessed(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "tmdb")
	// Two remakes a year apart, neither in the year the file names; and a file with no year.
	lib.provider.searches[searchKey("The General", 2003)] = []Candidate{
		{ID: "1", Name: "The General", Year: 1972}, {ID: "2", Name: "The General", Year: 2002},
	}
	lib.provider.searches[searchKey("Metropolis", 0)] = []Candidate{lib.provider.add(heat)}
	lib.scan(video("The General (2003).mkv"), video("Metropolis.mkv"))
	for _, rel := range []string{"The General (2003).mkv", "Metropolis.mkv"} {
		if outcome := lib.match(rel); outcome != OutcomeAmbiguous {
			t.Errorf("%s: outcome %s, want ambiguous", rel, outcome)
		}
	}
	lib.scan(video("The General (2003).mkv"), video("Metropolis.mkv"))
	solaris := lib.files()["The General (2003).mkv"]
	if solaris.Ident != identAmbiguous || solaris.Filing != filingListed || solaris.Source != nil {
		t.Errorf("after another pass: %+v; want ambiguous, listed on local data", solaris)
	}
	kept, err := lib.matcher.cache.Read().ListMatchCandidates(t.Context(), solaris.TitleID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, candidate := range kept {
		ids = append(ids, candidate.ProviderID)
	}
	if !slices.Equal(ids, []string{"1", "2"}) {
		t.Errorf("kept candidates %v, want the search's, in its order", ids)
	}
	if slices.Contains(lib.provider.calls, "fetch 949") {
		t.Error("a film with no year was fetched: its single result was taken as its match")
	}
}

func TestMatch_AProviderIDInHandSkipsTheSearch(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "tmdb")
	lib.provider.add(heat)
	lib.provider.add(leon)
	lib.provider.imdb["tt0345678"] = leon.ID
	lib.scan(video("Metropolis {tmdb-9019}.mkv"), video("Variety [imdbid-tt0345678].mkv"))
	lib.match("Metropolis {tmdb-9019}.mkv")
	lib.match("Variety [imdbid-tt0345678].mkv")
	want := []string{"fetch 949", "find imdb tt0345678", "fetch 101"}
	if !slices.Equal(lib.provider.calls, want) {
		t.Errorf("calls %v, want %v: no search", lib.provider.calls, want)
	}
	if file := lib.files()["Variety [imdbid-tt0345678].mkv"]; name(file) != leon.Name {
		t.Errorf("the IMDb-tagged file is %q", name(file))
	}
}

func TestMatch_NothingFoundIsRecordedAndNotSearchedAgain(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "nfo_then_tmdb")
	const rel = "home video (2011).mkv"
	lib.scan(video(rel))
	if outcome := lib.match(rel); outcome != OutcomeNoMatch {
		t.Fatalf("outcome %s, want no match", outcome)
	}
	report := lib.scan(video(rel))
	file := lib.files()[rel]
	why := noMatchReason("nfo_then_tmdb")
	if file.Filing != filingUnidentified || file.FilingReason == nil || *file.FilingReason != why {
		t.Errorf("file %+v, want unidentified, saying why", file)
	}
	if len(report.AwaitingMatch) != 0 {
		t.Errorf("the pass queued it again: %v", report.AwaitingMatch)
	}
	if outcome := lib.match(rel); outcome != OutcomeNothingToDo || len(lib.provider.calls) != 1 {
		t.Errorf("matching again: %s after calls %v; want nothing done, one search", outcome, lib.provider.calls)
	}
}

func TestMatch_ASecondTitleForTheSameWorkJoinsTheFirst(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "tmdb")
	lib.provider.searches[searchKey("Metropolis", 1995)] = []Candidate{lib.provider.add(heat)}
	lib.scan(video("Metropolis {tmdb-9019}.mkv"), video("other/Metropolis (1995).mkv"))
	lib.match("Metropolis {tmdb-9019}.mkv")
	lib.match("other/Metropolis (1995).mkv")
	files := lib.files()
	first, second := files["Metropolis {tmdb-9019}.mkv"], files["other/Metropolis (1995).mkv"]
	if first.TitleID != second.TitleID {
		t.Errorf("titles %d and %d; one work found twice should be one title with two versions",
			first.TitleID, second.TitleID)
	}
	lib.scan(video("Metropolis {tmdb-9019}.mkv"), video("other/Metropolis (1995).mkv"))
	if files := lib.files(); files["other/Metropolis (1995).mkv"].TitleID != first.TitleID {
		t.Error("another pass split the joined title")
	}
}

func TestMatch_AnOverrideSurvivesTheMatchAndReDerivation(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "tmdb")
	lib.provider.searches[searchKey("Metropolis", 1995)] = []Candidate{lib.provider.add(heat)}
	const rel = "Metropolis (1995).mkv"
	lib.scan(video(rel))
	titleID := lib.files()[rel].TitleID
	err := lib.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.InsertOverrideIfAbsent(t.Context(), sqlcgen.InsertOverrideIfAbsentParams{
			ItemID: titleID, Field: fieldName, Value: "Metropolis (director's copy)", Origin: "user", SetAt: 1,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	lib.match(rel)
	lib.scan(video(rel))
	file := lib.files()[rel]
	if name(file) != "Metropolis (director's copy)" || file.Source == nil || *file.Source != "tmdb" {
		t.Errorf("file %+v named %q; want TMDB's film under the person's name", file, name(file))
	}
}

func TestMatch_AProvidersRefusalReturnsForTheQueueToActOn(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "tmdb")
	lib.scan(video("Metropolis (1995).mkv"))
	lib.provider.fail = &RateLimitedError{After: 3}
	_, err := lib.matcher.Match(t.Context(), lib.files()["Metropolis (1995).mkv"].TitleID)
	if limited, isLimited := errors.AsType[*RateLimitedError](err); !isLimited || limited.After != 3 {
		t.Errorf("Match = %v, want the provider's RateLimitedError", err)
	}
	if file := lib.files()["Metropolis (1995).mkv"]; file.Ident != identUnidentified || file.Source != nil {
		t.Errorf("a refused call changed the title: %+v", file)
	}
}

func TestHandle_TellsTheQueueHowToWait(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "tmdb")
	lib.scan(video("Metropolis (1995).mkv"))
	job := jobs.Job{Kind: jobs.KindFetch, Subject: lib.files()["Metropolis (1995).mkv"].TitleID}
	lib.provider.fail = &RateLimitedError{After: time.Minute}
	err := lib.matcher.Handle(t.Context(), job)
	retry, isRetry := errors.AsType[*jobs.RetryAtError](err)
	if !isRetry || !retry.At.Equal(lib.clock.Now().Add(time.Minute)) {
		t.Errorf("rate limited: %v, want RetryAtError a minute from now", err)
	}
	lib.provider.fail = fmt.Errorf("tmdb: %w", ErrNoKey)
	if err := lib.matcher.Handle(t.Context(), job); err == nil || !errors.Is(err, ErrNoKey) {
		t.Errorf("no key: %v, want it blocked", err)
	}
	lib.provider.fail = nil
	if err := lib.matcher.Handle(t.Context(), job); err != nil {
		t.Errorf("a search with no result: %v, want the job done", err)
	}
}
