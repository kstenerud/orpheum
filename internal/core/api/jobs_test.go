// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// listedJobs is the job view as an admin's GET /jobs answers it.
func (harness testHarness) listedJobs(t *testing.T, token string) []api.JobBody {
	t.Helper()
	answer := harness.do(t, http.MethodGet, "/jobs", token, nil)
	var listed api.JobsOutput
	if err := json.Unmarshal([]byte(answer.body), &listed.Body); err != nil || answer.status != http.StatusOK {
		t.Fatalf("GET /jobs = %d %s", answer.status, answer.body)
	}
	return listed.Body.Jobs
}

// newestReadThrough is the newest read-through in the view.
func newestReadThrough(t *testing.T, listed []api.JobBody) api.JobBody {
	t.Helper()
	return newestOf(t, listed, jobs.KindReadThrough)
}

// newestOf is the newest job of a kind in the view.
func newestOf(t *testing.T, listed []api.JobBody, kind jobs.Kind) api.JobBody {
	t.Helper()
	for _, job := range listed {
		if string(job.Kind) == string(kind) {
			return job
		}
	}
	t.Fatalf("no %s job in %+v", kind, listed)
	return api.JobBody{}
}

// The view lists a play's read-through of its film, and a cancel stops it on its node, so the next play
// asks for it again rather than waiting on it.
func TestJobs_TheViewListsAReadThroughAndACancelStopsItOnTheNodeSoTheNextPlayAsksAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	harness.play(t, admin, chrome(1))
	queued := newestReadThrough(t, harness.listedJobs(t, admin))
	if queued.State != "queued" || queued.Reason == "" {
		t.Errorf("the read-through in the view = %+v, want queued with its reason", queued)
	}
	runner := harness.queue.NewRunner(jobs.KindReadThrough, harness.readThrough.HandleReadThrough)
	ran := make(chan error, 1)
	go func() { ran <- runner.RunDue(t.Context()) }()
	if command := harness.nextCommand(t); command.Type != nodeproto.CommandReadThrough {
		t.Fatalf("the node was sent %+v, want the read-through", command)
	}
	// Given to the node, its runner waits on nothing (D234 §7); to a person, it runs there.
	if assigned := newestReadThrough(t, harness.listedJobs(t, admin)); assigned.State != "running" {
		t.Errorf("the read-through once the node has it = %+v, want running on it", assigned)
	}
	cancel := "/jobs/" + strconv.FormatInt(queued.ID, 10) + "/cancel"
	if status := harness.do(t, http.MethodPost, cancel, admin, nil).status; status != http.StatusNoContent {
		t.Fatalf("POST %s: %d, want 204", cancel, status)
	}
	stop := harness.nextCommand(t)
	if stop.Type != nodeproto.CommandCancel || stop.JobID != queued.ID {
		t.Errorf("the node was then sent %+v, want a cancel of job %d", stop, queued.ID)
	}
	if err := testutil.Receive(t, ran, "the runner to end"); err != nil {
		t.Fatal(err)
	}
	if cancelled := newestReadThrough(t, harness.listedJobs(t, admin)); cancelled.State != "cancelled" {
		t.Errorf("the read-through after the cancel = %+v, want cancelled", cancelled)
	}
	harness.play(t, admin, chrome(1))
	if next := newestReadThrough(t, harness.listedJobs(t, admin)); next.ID == queued.ID || next.State != "queued" {
		t.Errorf("the newest read-through = %+v, want a new one queued", next)
	}
	if status := harness.do(t, http.MethodPost, cancel, admin, nil).status; status != http.StatusConflict {
		t.Errorf("cancelling a cancelled job: %d, want 409", status)
	}
}

func TestJobs_OnlyAnAdminSeesOrCancelsThem(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	_, err := harness.authService.CreateUser(t.Context(), auth.NewUser{
		Name: "guest", Password: "guest-password", Role: auth.RoleUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	guest := harness.signIn(t, "guest", "guest-password")
	if status := harness.do(t, http.MethodGet, "/jobs", guest, nil).status; status != http.StatusForbidden {
		t.Errorf("GET /jobs as a user: %d, want 403", status)
	}
	if status := harness.do(t, http.MethodPost, "/jobs/1/cancel", guest, nil).status; status != http.StatusForbidden {
		t.Errorf("a cancel as a user: %d, want 403", status)
	}
}

func TestJobs_ACancelledSubtitleExtractionIsStoppedOnTheNode(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSubtitles(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	harness.play(t, admin, chrome(2))
	extraction := newestOf(t, harness.listedJobs(t, admin), jobs.KindExtractSubtitle)
	runner := harness.queue.NewRunner(jobs.KindExtractSubtitle, harness.subtitles.HandleExtractSubtitle)
	ran := make(chan error, 1)
	go func() { ran <- runner.RunDue(t.Context()) }()
	if command := harness.nextCommand(t); command.Type != nodeproto.CommandExtractSubtitle {
		t.Fatalf("the node was sent %+v, want the extraction", command)
	}
	cancel := "/jobs/" + strconv.FormatInt(extraction.ID, 10) + "/cancel"
	if status := harness.do(t, http.MethodPost, cancel, admin, nil).status; status != http.StatusNoContent {
		t.Fatalf("POST %s: %d, want 204", cancel, status)
	}
	if stop := harness.nextCommand(t); stop.Type != nodeproto.CommandCancel || stop.JobID != extraction.ID {
		t.Errorf("the node was then sent %+v, want a cancel of job %d", stop, extraction.ID)
	}
	if err := testutil.Receive(t, ran, "the runner to end"); err != nil {
		t.Fatal(err)
	}
}

func TestJobs_TheDocumentNamesEveryKindOfJob(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	served := harness.do(t, http.MethodGet, "/openapi.json", "", nil)
	var document struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum []string `json:"enum"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(served.body), &document); err != nil {
		t.Fatalf("GET /openapi.json = %d: %v", served.status, err)
	}
	documented := document.Components.Schemas["JobBody"].Properties["kind"].Enum
	for _, kind := range jobs.Kinds() {
		if !slices.Contains(documented, string(kind)) {
			t.Errorf("the job view's kinds %v lack %q, which the queue has", documented, kind)
		}
	}
}

func TestJobs_AJobWaitingOnADestinationThatIsDownSaysWhatWhySinceWhenAndTheNextCheck(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.roots = []string{"/media"} // shelveFilms enrolled node 1, "local"
	harness.shelveAudioStreams(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	harness.play(t, admin, chrome(1))
	// No one waits for it any more, so it waits for its node: a play-driven one whose node is away
	// ends instead (D241 §4).
	harness.clock.Advance(time.Hour)
	if _, err := harness.queue.DemoteUnwanted(t.Context()); err != nil {
		t.Fatal(err)
	}
	harness.nodes.isAway.Store(true)
	reads := harness.queue.NewRunner(jobs.KindReadThrough, harness.readThrough.HandleReadThrough)
	if err := reads.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	read := newestReadThrough(t, harness.listedJobs(t, admin))
	wantNode := api.DestinationBody{
		Destination: "node:1", Name: "node local", Reason: "not connected", DownAt: fakeNodeLastHeard.UnixMilli(),
	}
	if read.State != "queued" || read.Destination != "node:1" || read.WaitingFor == nil ||
		*read.WaitingFor != wantNode {
		t.Errorf("a read-through for a node that is away: %+v, waiting for %+v; want it waiting for node local, "+
			"not connected since it was last heard, with no next check: its hello brings it back (D231 §12)",
			read, read.WaitingFor)
	}
	harness.queue.Park(t.Context(), jobs.Parking{Destination: jobs.DestinationTMDBAPI, Reason: "unreachable"})
	fetch := jobs.Request{
		Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: 7}, Reason: "test",
		Destination: jobs.DestinationTMDBAPI,
	}
	if _, err := harness.queue.Enqueue(t.Context(), fetch); err != nil {
		t.Fatal(err)
	}
	waiting := newestOf(t, harness.listedJobs(t, admin), jobs.KindFetchMetadata).WaitingFor
	next := harness.clock.Now().Add(30 * time.Second).UnixMilli()
	if waiting == nil || waiting.Name != "TMDB" || waiting.Reason != "unreachable" ||
		waiting.NextCheckAt == nil || *waiting.NextCheckAt != next {
		t.Errorf("a fetch while TMDB is down waits for %+v; want TMDB, unreachable, next checked in 30 s", waiting)
	}
}

// The job view says where the node holding a job's work last said it is (D240 §6).
func TestJobs_AJobGivenToANodeSaysWhereItsNodeSaysItIs(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.play(t, admin, chrome(1))
	runner := harness.queue.NewRunner(jobs.KindReadThrough, func(context.Context, jobs.Job) error {
		return jobs.Assign(harness.clock.Now().Add(time.Hour))
	})
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	harness.queue.SetReportedProgress(func(int64) (string, bool) { return "paused", true })
	shown := newestReadThrough(t, harness.listedJobs(t, admin))
	if shown.State != "running" || shown.OnNode != "paused" {
		t.Errorf("the read-through its node paused shows %+v; want running, paused on its node", shown)
	}
}

// A scan's jobs name what they work on: a listing its scan, an examining the batch it reads (D240);
// and a run of chunks names its store (D247 §10).
func TestJobs_AScansJobsNameTheirScanAndBatch(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	for _, request := range []jobs.Request{
		{Kind: jobs.KindScan, Target: jobs.Target{ScanID: 4}, Reason: "test"},
		{Kind: jobs.KindExamine, Target: jobs.Target{ScanBatchID: 9}, Reason: "test"},
		{Kind: jobs.KindMakeChunks, Target: jobs.Target{ChunkStoreID: 3}, Reason: "test"},
	} {
		if _, err := harness.queue.Enqueue(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	listed := harness.listedJobs(t, admin)
	if scan := newestOf(t, listed, jobs.KindScan); scan.ScanID != 4 || scan.ScanBatchID != 0 {
		t.Errorf("the scan job shows %+v, want scan 4", scan)
	}
	if examine := newestOf(t, listed, jobs.KindExamine); examine.ScanBatchID != 9 || examine.ScanID != 0 {
		t.Errorf("the examine job shows %+v, want batch 9", examine)
	}
	if run := newestOf(t, listed, jobs.KindMakeChunks); run.ChunkStoreID != 3 || run.FileID != 0 {
		t.Errorf("the run of chunks shows %+v, want chunk store 3", run)
	}
}
