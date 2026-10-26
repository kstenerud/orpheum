// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/providers/tmdb"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// These tests are scenarios from docs/contributors/design/plans/scenarios.md, named by their ids:
// the server's own wiring of the queue, TMDB's provider service and the matcher, against a TMDB
// on loopback that goes down and comes back, on a fake clock.

// identifiedState is a title matched to its provider, as persistent.db's identification_state says it.
const identifiedState = "identified"

// scenarioKey is the TMDB key the scenarios' TMDB accepts.
const scenarioKey = "the-right-key"

// fakeTMDBHost is a TMDB that knows every "Film NNN (2001)" as work 1NNN, and can be down or
// refuse the key. It counts the requests it is sent.
type fakeTMDBHost struct {
	t            *testing.T
	mutex        sync.Mutex
	isDown       bool
	requestCount int
	// answeredPaths are the paths of the requests it answered, in order.
	answeredPaths []string
}

func (host *fakeTMDBHost) serve(writer http.ResponseWriter, request *http.Request) {
	host.mutex.Lock()
	host.requestCount++
	isDown := host.isDown
	host.mutex.Unlock()
	switch {
	case isDown:
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	case request.URL.Query().Get("api_key") != scenarioKey:
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}
	host.mutex.Lock()
	host.answeredPaths = append(host.answeredPaths, request.URL.Path)
	host.mutex.Unlock()
	writer.Header().Set("Content-Type", "application/json")
	work := map[string]any{"release_date": "2001-05-01"}
	if request.URL.Path == "/search/movie" {
		name := request.URL.Query().Get("query")
		work["id"], _ = strconv.Atoi("1" + strings.TrimPrefix(name, "Film ")) //nolint:errcheck // 0 if not
		work["title"], work["original_title"] = name, name
		if err := json.NewEncoder(writer).Encode(map[string]any{"results": []any{work}}); err != nil {
			host.t.Error(err)
		}
		return
	}
	id, _ := strconv.Atoi(strings.TrimPrefix(request.URL.Path, "/movie/")) //nolint:errcheck // 0 if not
	name := fmt.Sprintf("Film %03d", id-1000)
	work["id"], work["title"], work["original_title"] = id, name, name
	if err := json.NewEncoder(writer).Encode(work); err != nil {
		host.t.Error(err)
	}
}

func (host *fakeTMDBHost) set(isDown bool) {
	host.mutex.Lock()
	defer host.mutex.Unlock()
	host.isDown = isDown
}

func (host *fakeTMDBHost) requests() int {
	host.mutex.Lock()
	defer host.mutex.Unlock()
	return host.requestCount
}

// jobsWorld is the server's metadata jobs, wired as serve wires them, over one library of films.
type jobsWorld struct {
	t         *testing.T
	database  *store.Database
	cache     *store.Cache
	config    *settings.Service
	tmdbURL   string
	clock     *clocktest.Fake
	queue     *jobs.Queue
	secrets   *secrets.Store
	hosts     hostProviders
	tmdb      *fakeTMDBHost
	pass      *identify.Pass
	matcher   *identify.Matcher
	refresher *identify.Refresher
	runner    *jobs.Runner
	libraryID int64
	nodeID    int64
	ingester  *library.Ingester
}

func newJobsWorld(t *testing.T, start time.Time) *jobsWorld {
	t.Helper()
	database := storetest.OpenMigrated(t)
	config := settingstest.Seeded(t, database)
	fakeClock := clocktest.NewFake(start)
	key, err := secrets.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	world := &jobsWorld{
		t: t, database: database, clock: fakeClock, config: config,
		secrets: secrets.NewStore(database, key, fakeClock, rand.Reader, config), tmdb: &fakeTMDBHost{t: t},
		ingester: library.NewIngester(database, fakeClock, config, slog.New(slog.DiscardHandler)),
	}
	if setErr := world.secrets.Set(t.Context(), secrets.TMDBAPIKey, secrets.NewValue(scenarioKey), nil); setErr != nil {
		t.Fatal(setErr)
	}
	server := httptest.NewServer(http.HandlerFunc(world.tmdb.serve))
	t.Cleanup(server.Close)
	world.tmdbURL = server.URL
	world.wireQueue(storetest.OpenCache(t))
	world.nodeID, world.libraryID = librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "tmdb", Root: "/media"})
	return world
}

// wireQueue wires the queue, TMDB's provider services and the matcher over cache by serve's own
// newMetadataJobs: again over a new cache.db when a test discards the old one.
func (world *jobsWorld) wireQueue(cache *store.Cache) {
	world.t.Helper()
	logger := slog.New(slog.DiscardHandler)
	world.cache = cache
	httpClient, err := outbound.NewClient(5*time.Second, "test")
	if err != nil {
		world.t.Fatal(err)
	}
	metadata := newMetadataJobs(metadataParts{
		database: world.database, cache: cache, settings: world.config, secrets: world.secrets, clock: world.clock,
		random: jobqueuetest.Unjittered(), httpClient: httpClient, tmdbBaseURL: world.tmdbURL, logger: logger,
	})
	world.queue, world.hosts, world.pass = metadata.queue, metadata.hosts, metadata.pass
	world.matcher, world.refresher = metadata.matcher, metadata.refresher
	world.runner = world.queue.NewRunner(jobs.KindFetchMetadata, world.matcher.Handle)
}

// addFilms lands a copy of count films and has their first matches queued, as a scan's pass does.
func (world *jobsWorld) addFilms(first, count int) {
	world.t.Helper()
	records := make([]nodeproto.ScanRecord, 0, count)
	for number := first; number < first+count; number++ {
		path := fmt.Sprintf("Film %03d (2001).mkv", number)
		sum := sha256.Sum256([]byte(path))
		records = append(records, nodeproto.ScanRecord{Type: nodeproto.RecordObserved, Observed: &nodeproto.Observed{
			Path: nodeproto.Path{Text: path}, SizeBytes: 1, ModifiedAt: 1,
			Identity: nodeproto.Identity{
				Scheme: "payload-v1", Anchor: "whole-file", PayloadHash: hex.EncodeToString(sum[:]),
			},
			Probe: nodeproto.Probe{Format: nodeproto.ProbeFormat{DurationMs: 5_000}},
		}})
	}
	all := librarytest.Scan(nodeproto.ScanBegin{RootPath: "/media"}, records, true)
	if err := world.ingester.IngestScan(world.t.Context(), world.nodeID, "/media", all); err != nil {
		world.t.Fatal(err)
	}
	report, err := world.pass.IdentifyLibrary(world.t.Context(), world.libraryID)
	if err != nil {
		world.t.Fatal(err)
	}
	queueOwedFetches(world.t.Context(), world.queue, report, slog.New(slog.DiscardHandler))
}

func (world *jobsWorld) runDue() {
	world.t.Helper()
	if err := world.runner.RunDue(world.t.Context()); err != nil {
		world.t.Fatal(err)
	}
}

// probeUntil runs each of TMDB's probes as it falls due, until the clock would pass until, and
// says how many ran.
func (world *jobsWorld) probeUntil(until time.Time) int {
	world.t.Helper()
	probes := 0
	for {
		state, isParked := world.queue.DestinationState(jobs.DestinationTMDBAPI)
		if !isParked || state.NextProbeAt.IsZero() || state.NextProbeAt.After(until) {
			return probes
		}
		world.clock.Set(state.NextProbeAt)
		world.runDue()
		probes++
	}
}

// films are each film's title row, by its number.
func (world *jobsWorld) films() map[int]sqlcgen.ListFileTitlesRow {
	world.t.Helper()
	rows, err := world.database.Read().ListFileTitles(world.t.Context(), world.libraryID)
	if err != nil {
		world.t.Fatal(err)
	}
	byNumber := map[int]sqlcgen.ListFileTitlesRow{}
	for _, row := range rows {
		number, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(row.Path, "/media/Film "), " (2001).mkv"))
		if err == nil {
			byNumber[number] = row
		}
	}
	return byNumber
}

// fetchJobs are the fetch jobs in the view, newest first.
func (world *jobsWorld) fetchJobs() []jobs.View {
	world.t.Helper()
	views, err := world.queue.List(world.t.Context(), 1000)
	if err != nil {
		world.t.Fatal(err)
	}
	return views
}

// S25: TMDB is unreachable from 22:00 to 07:00, and 200 films land at 23:00. They appear at once
// on local data; by morning they are matched, and TMDB was asked a handful of times, not a
// thousand (D231 §5, §6).
func TestScenario_S25_ATMDBOutageOvernightParksAndMatchesAllByMorning(t *testing.T) {
	t.Parallel()
	world := newJobsWorld(t, time.Date(2026, 9, 30, 22, 0, 0, 0, time.UTC))
	world.tmdb.set(true)
	world.clock.Set(time.Date(2026, 9, 30, 23, 0, 0, 0, time.UTC))
	world.addFilms(0, 200)
	if listed := len(world.films()); listed != 200 {
		t.Fatalf("%d films listed, want all 200 at once on local data (D170 §2)", listed)
	}
	world.runDue()
	morning := time.Date(2026, 10, 1, 7, 0, 0, 0, time.UTC)
	probes := world.probeUntil(morning)
	askedOvernight := world.tmdb.requests()
	if askedOvernight > 20 || askedOvernight != probes+1 {
		t.Errorf("TMDB was asked %d times overnight (%d probes); want a handful: one failure, then one probe per "+
			"step of a back-off to an hour", askedOvernight, probes)
	}
	world.tmdb.set(false)
	world.clock.Set(morning)
	world.probeUntil(morning.Add(2 * time.Hour))
	for number, film := range world.films() {
		if film.IdentificationState != identifiedState {
			t.Errorf("film %d is %s by morning, want identified", number, film.IdentificationState)
		}
	}
	for _, job := range world.fetchJobs() {
		if job.State != "done" || job.AttemptCount != 1 {
			t.Errorf("fetch job %d: %s after %d attempts; want done at its first attempt: an outage costs none",
				job.ID, job.State, job.AttemptCount)
		}
	}
}

// S70: during S25's outage the owner presses Refresh on a film. TMDB is looked at immediately; while it
// is still down the request waits, shown as waiting for TMDB, and runs first when TMDB returns. A
// person's choice in the chooser, made while they wait, always tries TMDB and counts as its probe
// (D231 §6).
func TestScenario_S70_APersonsRefreshWhileTMDBIsParkedLooksAtOnceAndRunsFirstWhenItReturns(t *testing.T) {
	t.Parallel()
	world := newJobsWorld(t, time.Date(2026, 9, 30, 21, 0, 0, 0, time.UTC))
	world.addFilms(0, 1)
	world.runDue()
	refreshed := world.films()[0].TitleID
	world.tmdb.set(true)
	world.addFilms(1, 3)
	world.runDue()
	if _, isParked := world.queue.DestinationState(jobs.DestinationTMDBAPI); !isParked {
		t.Fatal("TMDB is not parked after a fetch found it down")
	}
	askedBefore := world.tmdb.requests()
	if _, err := world.refresher.Request(t.Context(), refreshed); err != nil {
		t.Fatal(err)
	}
	world.runDue()
	if world.tmdb.requests() != askedBefore+1 {
		t.Errorf("TMDB was asked %d times after a person's refresh; want once, at once, without waiting for its "+
			"probe", world.tmdb.requests()-askedBefore)
	}
	var refresh jobs.View
	for _, job := range world.fetchJobs() {
		if job.ItemID == refreshed && job.State == "queued" {
			refresh = job
		}
	}
	if refresh.WaitingFor == nil || refresh.WaitingFor.Reason != "unreachable" || refresh.AttemptCount != 0 {
		t.Errorf("the person's refresh while TMDB is still down: %+v; want it waiting for TMDB, no attempt spent",
			refresh)
	}
	world.checkAPersonsChoiceIsTMDBsProbe(world.films()[1].TitleID)
	world.tmdb.set(false)
	world.tmdb.mutex.Lock()
	answeredBefore := len(world.tmdb.answeredPaths)
	world.tmdb.mutex.Unlock()
	world.probeUntil(world.clock.Now().Add(2 * time.Hour))
	world.tmdb.mutex.Lock()
	answered := world.tmdb.answeredPaths[answeredBefore:]
	world.tmdb.mutex.Unlock()
	if len(answered) == 0 || answered[0] != "/movie/1000" {
		t.Errorf("once TMDB was back it answered %q; want the person's refresh of film 0 first", answered)
	}
	world.checkEveryFilmIdentified("once TMDB was back")
}

// checkAPersonsChoiceIsTMDBsProbe has a person choose a match for itemID while TMDB is parked, and
// checks that the choice tried TMDB once, as its probe: still parked, its next probe backed off.
func (world *jobsWorld) checkAPersonsChoiceIsTMDBsProbe(itemID int64) {
	t := world.t
	t.Helper()
	nextProbeBefore := world.hosts.tmdbAPI.State(t.Context()).Parked.NextProbeAt
	chooser := identify.NewChooser(identify.NewMatcher(world.database, world.cache, world.pass, world.clock,
		slog.New(slog.DiscardHandler)), world.pass)
	askedBefore := world.tmdb.requests()
	if err := chooser.Choose(t.Context(), itemID, "tmdb", "1001"); err == nil {
		t.Error("a person's choice of a match was fetched while TMDB is down")
	}
	state := world.hosts.tmdbAPI.State(t.Context())
	if world.tmdb.requests() != askedBefore+1 || state.Parked == nil ||
		!state.Parked.NextProbeAt.After(nextProbeBefore) {
		t.Errorf("a person's choice while TMDB is parked asked it %d times, and left it %+v; want it tried once, "+
			"as TMDB's probe, still parked and its next probe backed off", world.tmdb.requests()-askedBefore,
			state.Parked)
	}
}

// checkEveryFilmIdentified checks that every film is matched, when said.
func (world *jobsWorld) checkEveryFilmIdentified(when string) {
	world.t.Helper()
	for number, film := range world.films() {
		if film.IdentificationState != identifiedState {
			world.t.Errorf("film %d is %s %s, want identified", number, film.IdentificationState, when)
		}
	}
}

// S73: the TMDB key is refused while fetches are queued. They wait at no cost, TMDB is not asked
// again with a key it refused, the admin is told, and they resume the moment the key is changed
// (D231 §5).
func TestScenario_S73_ARefusedKeyParksUntilTheKeyChanges(t *testing.T) {
	t.Parallel()
	world := newJobsWorld(t, time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	if err := world.secrets.Set(t.Context(), secrets.TMDBAPIKey, secrets.NewValue("a-wrong-key"), nil); err != nil {
		t.Fatal(err)
	}
	world.addFilms(0, 5)
	world.runDue()
	world.clock.Advance(24 * time.Hour)
	world.runDue()
	if asked := world.tmdb.requests(); asked != 1 {
		t.Errorf("TMDB was asked %d times in a day with a key it refused; want once", asked)
	}
	state := world.hosts.tmdbAPI.State(t.Context())
	if state.Credentials != providers.CredentialsRefused || state.Parked == nil || !state.Parked.AwaitsSignal {
		t.Errorf("the admin sees TMDB as %+v; want its key refused, parked until it changes", state)
	}
	if err := world.secrets.Set(t.Context(), secrets.TMDBAPIKey, secrets.NewValue(scenarioKey), nil); err != nil {
		t.Fatal(err)
	}
	world.runDue()
	for number, film := range world.films() {
		if film.IdentificationState != identifiedState {
			t.Errorf("film %d is %s once the key changed, want identified", number, film.IdentificationState)
		}
	}
	for _, job := range world.fetchJobs() {
		if job.AttemptCount != 1 {
			t.Errorf("fetch job %d spent %d attempts; the refused key costs none", job.ID, job.AttemptCount)
		}
	}
}

// missingPosterFetcher is TMDB's image server, as far as a recheck needs it: its CDN keeps a 404
// for twelve hours (tmdb.ImageFetcher's).
type missingPosterFetcher struct{}

func (missingPosterFetcher) Fetch(context.Context, string) (artwork.Unsanitised, error) {
	return artwork.Unsanitised{}, errors.New("the scenario fetches no image")
}

func (missingPosterFetcher) RecheckAfter() time.Duration {
	return (&tmdb.ImageFetcher{}).RecheckAfter()
}

func (missingPosterFetcher) Destination() jobs.Destination { return jobs.DestinationTMDBImages }

// openWork is each open job's item and arguments, the work the queue holds.
func (world *jobsWorld) openWork() map[int64]string {
	world.t.Helper()
	rows, err := world.cache.Read().ListJobs(world.t.Context(), 1000)
	if err != nil {
		world.t.Fatal(err)
	}
	argsByItem := map[int64]string{}
	for _, row := range rows {
		if (row.State == "queued" || row.State == "running") && row.ItemID != nil {
			argsByItem[*row.ItemID] = row.ArgsJSON
		}
	}
	return argsByItem
}

// S78: cache.db is discarded under a running library, as a build with a new cache schema does at
// start. The library has matches parked on TMDB, one failed for good, a person's refresh waiting
// and a missing poster's recheck due tomorrow. Nothing is lost but timing: every piece of work
// the library implies is derived again by the sweeps, and every request a person made is still
// honoured (D231 §3, §4; G10, G14, G15).
func TestScenario_S78_DiscardingCacheDBLosesOnlyTiming(t *testing.T) {
	t.Parallel()
	world := newJobsWorld(t, time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	world.addFilms(0, 3)
	world.runDue()
	films := world.films()
	refreshed, rechecked := films[0].TitleID, films[1].TitleID
	world.tmdb.set(true)
	world.addFilms(3, 3)
	world.runDue() // films 3 to 5 park on TMDB
	films = world.films()
	failed := films[5].TitleID
	if _, err := world.refresher.Request(t.Context(), refreshed); err != nil {
		t.Fatal(err)
	}
	storetest.ExecRaw(t, world.database.Path(),
		fmt.Sprintf(`INSERT INTO fetch_failure (item_id, failure, failed_at) VALUES (%d, 'a 500', 1)`, failed),
		fmt.Sprintf(`INSERT INTO artwork (item_id, kind, source, source_ref, refused_at, refusal, is_missing_at_source)
		 VALUES (%d, 'poster', 'tmdb', '/poster.jpg', %d, 'a 404', 1)`, rechecked, world.clock.Now().UnixMilli()))

	world.wireQueue(storetest.OpenCache(t)) // cache.db discarded: a new, empty one
	world.tmdb.set(false)
	images := artwork.NewService(artwork.Parts{
		Database: world.database, Queue: world.queue, Cache: world.cache, Settings: world.config,
		Fetchers: map[string]artwork.Fetcher{"tmdb": missingPosterFetcher{}}, Clock: world.clock,
		Logger: slog.New(slog.DiscardHandler),
	})
	services := jobServices{
		queue: world.queue, refresher: world.refresher, artwork: images, pass: world.pass, matcher: world.matcher,
		invariants: world.database, removedFiles: library.NewFilePurger(world.database), settings: world.config,
	}
	for _, sweep := range services.sweeps(slog.New(slog.DiscardHandler)) {
		// No files to measure, nothing due a rolling refresh, and backups, scans and credentials are
		// not this scenario's work.
		if !slices.Contains([]string{
			sweepReadThroughs, sweepRollingRefresh, sweepScheduledBackup, sweepScheduledScans, sweepEndedCredentials,
			sweepUnavailableRoots,
		}, sweep.Name) {
			world.queue.AddSweep(sweep)
		}
	}
	world.queue.RunSweeps(t.Context())
	want := map[int64]string{
		films[3].TitleID: "{}", films[4].TitleID: "{}", // owed their first match, however it was lost
		refreshed: `{"isRefresh":true,"shouldRetryImages":true}`, // the person's refresh, images and all
	}
	if got := world.openWork(); !maps.Equal(got, want) {
		t.Errorf("the work derived again from persistent.db: %v, want %v: the matches owed and the person's refresh, "+
			"not the film that failed for good, nor a recheck before its time", got, want)
	}
	world.runDue()
	world.clock.Advance(13 * time.Hour) // past the CDN's 404
	world.queue.RunSweeps(t.Context())
	wantNextDay := map[int64]string{rechecked: `{"isRefresh":true,"shouldRetryImages":true}`}
	if got := world.openWork(); !maps.Equal(got, wantNextDay) {
		t.Errorf("the next day's work: %v; want the missing poster's one recheck, derived from its refusal", got)
	}
	world.runDue()
	for number, film := range world.films() {
		if isOwed := number != 5; isOwed != (film.IdentificationState == identifiedState) {
			t.Errorf("film %d is %s; want every film matched but the one that failed for good", number,
				film.IdentificationState)
		}
	}
}
