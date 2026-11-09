// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// These tests are scenarios from docs/contributors/design/plans/scenarios.md, named by their ids.

// S29: the partner starts a film whose node is down. The play is refused at once, saying the
// machine is away and since when, and nothing is queued: no one can watch it, so no work for it is
// wanted (D241 §4). Once the node is back, the same play starts.
func TestScenario_S29_APlayWhoseNodeIsAwayIsRefusedAtOnceAndQueuesNothing(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	partner := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.roots = []string{"/media"} // shelveFilms enrolled node 1, "local"
	harness.nodes.isAway.Store(true)
	refused := harness.do(t, http.MethodPost, "/playback", partner, chrome(2))
	var problem struct {
		Error struct {
			Code   api.Code                       `json:"code"`
			Detail api.MediaUnreachableDetailBody `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(refused.body), &problem); err != nil {
		t.Fatal(err)
	}
	want := api.MediaUnreachableDetailBody{
		Reason: "nodeAway", NodeName: "local", UnavailableAt: fakeNodeLastHeard.UnixMilli(),
	}
	if refused.status != http.StatusServiceUnavailable || problem.Error.Code != api.CodeMediaUnreachable ||
		problem.Error.Detail != want {
		t.Errorf("a play whose node is away: %d %s; want media_unreachable, node local away since it was last heard",
			refused.status, refused.body)
	}
	if listed := harness.listedJobs(t, partner); len(listed) != 0 {
		t.Errorf("a refused play queued %+v; want nothing", listed)
	}
	harness.nodes.isAway.Store(false)
	if decided := harness.play(t, partner, chrome(2)); decided.OnLadder == nil {
		t.Errorf("once its node is back the play = %+v, want it on its ladder", decided)
	}
}

// A film shown from its listing whose file is not read yet is refused at once as still being read,
// and its file is asked to be read before what else waits, so asking again soon plays it (D304 §4).
func TestStartPlayback_AFilmStillBeingReadIsRefusedAsSoAndAskedToBeReadFirst(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	partner := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.roots = []string{"/media"}
	storetest.ExecRaw(t, harness.databasePath, "UPDATE file SET payload_hash = NULL WHERE id = 2")
	refused := harness.do(t, http.MethodPost, "/playback", partner, chrome(2))
	var problem struct {
		Error struct {
			Code   api.Code                       `json:"code"`
			Detail api.MediaUnreachableDetailBody `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(refused.body), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Error.Code != api.CodeMediaUnreachable || problem.Error.Detail.Reason != "stillReading" {
		t.Errorf("a play of a film still being read: %d %s; want media_unreachable, still reading", refused.status,
			refused.body)
	}
	if asked := harness.reads.askedFor(); !slices.Equal(asked, []string{"/media/Film (2002).mkv"}) {
		t.Errorf("the play asked to have %v read; want the film's file", asked)
	}
}

// A play-driven read-through whose root goes missing after it was queued ends rather than parking on
// the root: no one can watch its film, so no one waits for it (D241 §4). A background one parks.
func TestReadThrough_APlayDrivenReadWhoseRootGoesMissingEndsRatherThanParks(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	harness.play(t, user, chrome(1)) // queues its read-through, a person waiting
	harness.missingMedia(t)
	reads := harness.queue.NewRunner(jobs.KindReadThrough, harness.readThrough.HandleReadThrough)
	if err := reads.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	read := newestReadThrough(t, harness.listedJobs(t, user))
	if read.State != "failed" || read.WaitingFor != nil ||
		!strings.Contains(read.LastFailure, "rootMissing") || len(harness.nodes.commanded) != 0 {
		t.Errorf("a play-driven read-through under a missing root: %+v; want it ended, saying the root is "+
			"missing, nothing sent", read)
	}
}

// A player whose fetch failed mid-play asks whether its file can be read now, and is told why not,
// or that it can and the failure passed (D241 §7).
func TestPlayback_APlayerAsksWhetherItsFileCanBeReadNow(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	if fine := harness.do(t, http.MethodGet, "/files/2/availability", user, nil); fine.status != http.StatusOK ||
		strings.Contains(fine.body, "unavailable") {
		t.Errorf("a file that can be read = %d %s; want it said so", fine.status, fine.body)
	}
	harness.missingMedia(t)
	missing := harness.do(t, http.MethodGet, "/files/2/availability", user, nil)
	const want = `{"unavailable":{"reason":"rootMissing","nodeName":"local","path":"/media","unavailableAt":5000}}`
	if missing.status != http.StatusOK || !strings.Contains(missing.body, want) {
		t.Errorf("a file under a missing root = %d %s; want %s", missing.status, missing.body, want)
	}
	wantError(t, harness.do(t, http.MethodGet, "/files/99/availability", user, nil), http.StatusNotFound,
		api.CodeNotFound)
}

// A play of a file under a root its node cannot read is refused at once, naming the root and since
// when, and queues nothing (D241 §4).
func TestPlayback_APlayOfAFileUnderAMissingRootIsRefusedAtOnceAndQueuesNothing(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	partner := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.missingMedia(t)
	refused := harness.do(t, http.MethodPost, "/playback", partner, chrome(2))
	const want = `"detail":{"reason":"rootMissing","nodeName":"local","path":"/media","unavailableAt":5000}`
	if refused.status != http.StatusServiceUnavailable || !strings.Contains(refused.body, want) {
		t.Errorf("a play under a missing root: %d %s; want media_unreachable naming /media on local, since 5000",
			refused.status, refused.body)
	}
	if listed := harness.listedJobs(t, partner); len(listed) != 0 {
		t.Errorf("a refused play queued %+v; want nothing", listed)
	}
}

// S76: two nodes, one down. Work for the live node's files is unaffected; work for the down
// node's files parks on it, and a doomed attempt no longer blocks the queue (D231 §6).
func TestScenario_S76_ADownNodeBlocksNoOtherNodesWork(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO node (id, name, enrolled_at) VALUES (2, 'attic', 1)`,
		`INSERT INTO library_root (library_id, node_id, root_path) VALUES (1, 2, '/attic')`)
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 10, LibraryID: 1, NodeID: 2, Path: "/attic/Film (2003).mp4", SizeBytes: 10,
		PayloadHash: []byte{0x1a, 0x1b}, Filing: "listed",
	})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO file_probe (file_id, probe_json, duration_ms)
		 SELECT 10, probe_json, duration_ms FROM file_probe WHERE file_id = 1`)
	librarytest.Stream(t, harness.database,
		librarytest.StreamSpec{BytesFile: 10, StreamIndex: 0, Kind: "audio", Codec: "aac", ChannelCount: 2})
	harness.nodes.awayNodeID = 2
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	for _, fileID := range []int64{10, 1} { // the down node's first, as a doomed attempt would have been
		measurement := jobs.Request{Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: fileID}, Reason: "test"}
		if _, err := harness.queue.Enqueue(t.Context(), measurement); err != nil {
			t.Fatal(err)
		}
	}
	// The live node's measurement is assigned to it, and ends when its answer is taken.
	answered := make(chan struct{})
	go func() {
		defer close(answered)
		command, isSent := harness.commandWithin(t)
		if !isSent {
			return
		}
		harness.answerReadThrough(t, command.JobID,
			nodeproto.ReadThroughResult{Failure: "Invalid data found when processing input"})
	}()
	runner := harness.queue.NewRunner(jobs.KindReadThrough, harness.readThrough.HandleReadThrough)
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	testutil.Receive(t, answered, "the live node's answer")
	for _, job := range harness.listedJobs(t, admin) {
		switch job.FileID {
		case 1:
			if job.State != "failed" || job.AttemptCount != 1 {
				t.Errorf("the live node's measurement = %+v; want it run at once, to FFmpeg's answer", job)
			}
		case 10:
			if job.State != "queued" || job.AttemptCount != 0 || job.Destination != "node:2" || job.WaitingFor == nil {
				t.Errorf("the down node's measurement = %+v; want it waiting for node attic, no attempt spent", job)
			}
		}
	}
}

// missingMedia records the last scan of node 1's root /media as unable to read it, which makes it
// unavailable since, as the ingest records one.
func (harness testHarness) missingMedia(t *testing.T) {
	t.Helper()
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at)
		 VALUES (1, '/media', 'unreadable', 5000)`,
		`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (1, '/media', '/media', 5000, 'unread')`)
}

func TestSubtitles_AnExtractionWaitsForAMissingRootAndForANodeThatIsAway(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSubtitles(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	harness.play(t, user, chrome(2))
	job := jobs.Job{ID: 93, Kind: jobs.KindExtractSubtitle, Target: jobs.Target{FileID: subtitleFileID}}
	harness.nodes.isAway.Store(true)
	err := harness.subtitles.HandleExtractSubtitle(t.Context(), job)
	if parking, isParked := jobqueue.ParkingOf(err); !isParked || parking.Destination != jobs.NodeDestination(1) {
		t.Errorf("an extraction for a node that is away: %v; want it parked on the node (D231 §11)", err)
	}
	harness.nodes.isAway.Store(false)
	harness.missingMedia(t)
	err = harness.subtitles.HandleExtractSubtitle(t.Context(), job)
	parking, isParked := jobqueue.ParkingOf(err)
	if !isParked || parking.Destination != jobs.RootDestination(1, "/media") {
		t.Errorf("an extraction under a missing root: %v; want it parked on the root (D231 §5)", err)
	}
	if len(harness.nodes.commanded) != 0 {
		t.Error("a command was sent for an extraction that waits")
	}
}

// retire is one of the retire actions, as an admin sends it (D231 §7).
func (harness testHarness) retire(t *testing.T, admin, method, path string, body any) api.RetiredOutput {
	t.Helper()
	answer := harness.do(t, method, path, admin, body)
	var retired api.RetiredOutput
	if err := json.Unmarshal([]byte(answer.body), &retired.Body); err != nil || answer.status != http.StatusOK {
		t.Fatalf("%s %s = %d %s", method, path, answer.status, answer.body)
	}
	return retired
}

// system is the System page, as an admin sees it.
func (harness testHarness) system(t *testing.T, admin string) api.SystemOutput {
	t.Helper()
	answer := harness.do(t, http.MethodGet, "/system", admin, nil)
	var system api.SystemOutput
	if err := json.Unmarshal([]byte(answer.body), &system.Body); err != nil || answer.status != http.StatusOK {
		t.Fatalf("GET /system = %d %s", answer.status, answer.body)
	}
	return system
}

// watchedFilm2 gives film 2 a title (identifyFilm) and a place the admin stopped watching it at.
func (harness testHarness) watchedFilm2(t *testing.T) {
	t.Helper()
	harness.identifyFilm(t)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO play_position (user_id, file_id, position_ms, report_seq, updated_at)
		 SELECT id, 2, 60000, 1, 1 FROM user`)
}

// checkHistoryKept checks that the admin's place in film 2 survived: history is never deleted
// (D173).
func (harness testHarness) checkHistoryKept(t *testing.T) {
	t.Helper()
	position, err := harness.database.Read().GetPlayPosition(t.Context(), sqlcgen.GetPlayPositionParams{
		UserID: 1, FileID: 2,
	})
	if err != nil || position.PositionMs != 60000 {
		t.Errorf("the admin's place in film 2 = %+v, %v; want it kept (D173)", position, err)
	}
}

// queueWork queues the requests and parks the destinations, as the queue finds them down.
func (harness testHarness) queueWork(t *testing.T, requests []jobs.Request, parked ...jobs.Destination) {
	t.Helper()
	for _, request := range requests {
		request.Reason = "test"
		if _, err := harness.queue.Enqueue(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	for _, destination := range parked {
		harness.queue.Park(t.Context(), jobs.Parking{Destination: destination, Reason: "missing", AwaitsSignal: true})
	}
}

// checkAllCancelled checks that every job in the view was cancelled, and that none of the
// destinations is parked any more.
func (harness testHarness) checkAllCancelled(t *testing.T, admin string, forgotten ...jobs.Destination) {
	t.Helper()
	listed := harness.listedJobs(t, admin)
	if len(listed) == 0 {
		t.Fatal("no jobs listed")
	}
	for _, job := range listed {
		if job.State != "cancelled" {
			t.Errorf("job %+v is %s, want cancelled with what was retired", job, job.State)
		}
	}
	for _, destination := range forgotten {
		if _, isParked := harness.queue.DestinationState(destination); isParked {
			t.Errorf("%s is still parked; nothing brings a retired destination back", destination)
		}
	}
}

// S22: the disk under a root dies for good. The System page says since when it is offline; the
// admin retires the root; its files are left to no library, kept to be found again or reaped
// (D363 §10), their jobs, parked on the root, are cancelled, the root is forgotten, the sweeps
// queue nothing for them again, and the history stays on the title (D41, D173, D231 §7).
func TestScenario_S22_ARetiredRootsFilesAreLeftToNoLibraryItsWorkIsCancelledAndItsHistoryStays(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.watchedFilm2(t)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at)
		 VALUES (1, '/media', 'unreadable', 9000)`,
		`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (1, '/media', '/media', 5000, 'unread')`)
	root := jobs.RootDestination(1, "/media")
	harness.queueWork(t, []jobs.Request{
		{Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 1}, Destination: root},
		{Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: 30}},
	}, root)
	if system := harness.system(t, admin); len(system.Body.Offline) != 1 || system.Body.Offline[0].OfflineAt != 5000 {
		t.Fatalf("the System page's offline list = %+v; want /media since 5000 (D41)", system.Body.Offline)
	}
	retired := harness.retire(t, admin, http.MethodPost, "/libraries/1/roots/retire",
		map[string]string{"nodeName": "local", "path": "/media"})
	if retired.Body.TombstonedFileCount != 0 || retired.Body.UncoveredFileCount != 3 ||
		retired.Body.CancelledJobCount != 2 {
		t.Errorf("the retire = %+v; want the root's 3 files left to no library, none removed, and both jobs "+
			"cancelled", retired.Body)
	}
	harness.checkAllCancelled(t, admin, root)
	if queued, err := harness.readThrough.QueueReadThroughs(t.Context()); err != nil || queued != 0 {
		t.Errorf("the loudness sweep after the retire queued %d, %v; want nothing for a retired root's files",
			queued, err)
	}
	if system := harness.system(t, admin); len(system.Body.Offline) != 0 {
		t.Errorf("the offline list after the retire = %+v; want it empty: the admin ended it", system.Body.Offline)
	}
	harness.checkHistoryKept(t)
}

// S23: the node machine is retired for good. The admin retires the node; its roots are retired,
// their files tombstoned, the jobs parked on it cancelled, and it is forgotten; the history
// stays (D231 §7, §11).
func TestScenario_S23_ARetiredNodesRootsAreRetiredAndItsParkedWorkCancelled(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.watchedFilm2(t)
	harness.nodes.isAway.Store(true)
	node := jobs.NodeDestination(1)
	harness.queueWork(t, []jobs.Request{
		{Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 1}, Destination: node},
		{Kind: jobs.KindMakeChunks, Target: jobs.Target{ChunkStoreID: 7}, Destination: node},
	}, node)
	retired := harness.retire(t, admin, http.MethodPost, "/nodes/local/retire", nil)
	if retired.Body.TombstonedFileCount != 3 || retired.Body.CancelledJobCount != 2 {
		t.Errorf("the retire = %+v; want the node's 3 files removed and both its jobs cancelled", retired.Body)
	}
	harness.checkAllCancelled(t, admin, node)
	var libraries api.LibrariesOutput
	answer := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if err := json.Unmarshal([]byte(answer.body), &libraries.Body); err != nil || len(libraries.Body.Libraries) != 1 ||
		len(libraries.Body.Libraries[0].Roots) != 0 {
		t.Errorf("the libraries after the node was retired = %s; want Films with no roots", answer.body)
	}
	again := harness.do(t, http.MethodPost, "/nodes/local/retire", admin, nil)
	if again.status != http.StatusNotFound {
		t.Errorf("retiring it again = %d %s, want 404: it is gone", again.status, again.body)
	}
	harness.checkHistoryKept(t)
}

// S24: the admin deletes a library. Its jobs are cancelled, and nothing is fetched for it again;
// its titles are listed by no library, and so reached by no person who is not an admin, but no
// item is deleted, and each person's watch history stays (D208 §2, D231 §7, D363 §10). A film of
// it that a library covers again is its title again:
// TestPass_ADeletedLibrarysFilmThatReappearsElsewhereBringsItsTitleBack,
// TestMatch_ADeletedLibrarysFilmMatchedAgainElsewhereBringsItsTitleBack (internal/core/identify).
func TestScenario_S24_ADeletedLibrarysWorkIsCancelledAndItsTitlesListedByNoLibraryWithTheirHistory(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.watchedFilm2(t)
	if title := harness.do(t, http.MethodGet, "/items/30", admin, nil); title.status != http.StatusOK {
		t.Fatalf("the film before the delete = %d %s", title.status, title.body)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	if title := harness.do(t, http.MethodGet, "/items/30", partner, nil); title.status != http.StatusOK {
		t.Fatalf("the film before the delete, to a person = %d %s", title.status, title.body)
	}
	harness.queueWork(t, []jobs.Request{
		{Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: 30}},
		{Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 1}},
	})
	retired := harness.retire(t, admin, http.MethodDelete, "/libraries/1", nil)
	if retired.Body.TombstonedFileCount != 0 || retired.Body.UncoveredFileCount != 3 ||
		retired.Body.CancelledJobCount != 2 {
		t.Errorf("the delete = %+v; want its 3 files left to no library, none removed, and both its jobs "+
			"cancelled", retired.Body)
	}
	harness.checkAllCancelled(t, admin, jobs.RootDestination(1, "/media"))
	if title := harness.do(t, http.MethodGet, "/items/30", partner, nil); title.status != http.StatusNotFound {
		t.Errorf("the deleted library's film, to a person = %d %s; want it reached by no one but an admin",
			title.status, title.body)
	}
	if title := harness.do(t, http.MethodGet, "/items/30", admin, nil); title.status != http.StatusOK {
		t.Errorf("the deleted library's film, to the admin = %d %s; want it kept: no item is deleted",
			title.status, title.body)
	}
	if again := harness.do(t, http.MethodDelete, "/libraries/1", admin, nil); again.status != http.StatusNotFound {
		t.Errorf("deleting it again = %d %s, want 404", again.status, again.body)
	}
	harness.checkHistoryKept(t)
}

// One file that cannot be read now is gone on the admin's word: a tombstone, its history kept; a
// file that can be read is refused, being there (D241 §5).
func TestRetireFile_AFileThatCannotBeReadIsGoneOnTheAdminsWord(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.watchedFilm2(t)
	wantError(t, harness.do(t, http.MethodPost, "/files/2/retire", admin, nil), http.StatusConflict, api.CodeConflict)
	harness.missingMedia(t)
	retired := harness.retire(t, admin, http.MethodPost, "/files/2/retire", nil)
	if retired.Body.TombstonedFileCount != 1 {
		t.Errorf("the retire = %+v; want the one file removed", retired.Body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/files/2/availability", admin, nil), http.StatusNotFound,
		api.CodeNotFound)
	if still := harness.do(t, http.MethodGet, "/files/1/availability", admin, nil); still.status != http.StatusOK {
		t.Errorf("the other file under the root = %d %s; want it kept", still.status, still.body)
	}
	harness.checkHistoryKept(t)
}

// A folder that moved is repointed: a dry run says what would move and changes nothing, then it and
// its files take the new path, the same files with their history, and a scan of the new place is asked
// for, which confirms each (D429 §2; repoint-library.md, "The spec").
func TestRepoint_AFolderThatMovedTakesItsFilesAndHistoryToTheNewPath(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.watchedFilm2(t)
	harness.nodes.roots = []string{"/media", "/mnt/media"}
	move := map[string]string{"nodeName": "local", "path": "/media", "newNodeName": "local", "newPath": "/elsewhere"}
	wantError(t, harness.do(t, http.MethodPost, "/repoints", admin, move),
		http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	move["newPath"] = "/mnt/media"
	preview := harness.do(t, http.MethodPost, "/repoints/preview", admin, move)
	if preview.status != http.StatusOK || !strings.Contains(preview.body, `"livePlaceCount":3`) ||
		!strings.Contains(preview.body, `"refusals":[]`) {
		t.Fatalf("the dry run = %d %s; want the root's three files, nothing refused", preview.status, preview.body)
	}
	if unmoved, err := harness.database.Read().GetPlayableFile(t.Context(), 2); err != nil ||
		string(unmoved.ReportedPathRaw) != "/media/Film (2002).mkv" {
		t.Errorf("file 2 after the dry run = %q, %v; want it where it was", unmoved.ReportedPathRaw, err)
	}
	moved := harness.do(t, http.MethodPost, "/repoints", admin, move)
	if moved.status != http.StatusOK || !strings.Contains(moved.body, `"livePlaceCount":3`) {
		t.Fatalf("the repoint = %d %s; want the root's three files moved", moved.status, moved.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/repoints", admin, move), http.StatusConflict, api.CodeConflict)
	file, err := harness.database.Read().GetPlayableFile(t.Context(), 2)
	if err != nil || string(file.ReportedPathRaw) != "/mnt/media/Film (2002).mkv" {
		t.Errorf("file 2 after the repoint = %q, %v; want it at /mnt/media/Film (2002).mkv", file.ReportedPathRaw, err)
	}
	if !slices.Contains(harness.nodes.scanned, "/mnt/media") {
		t.Errorf("scans asked for of %v; want the new place's", harness.nodes.scanned)
	}
	harness.checkHistoryKept(t)
}
