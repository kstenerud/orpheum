// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// listedJobs is the job view as an admin's GET /jobs answers it.
func (harness testHarness) listedJobs(t *testing.T, token string) []JobBody {
	t.Helper()
	answer := harness.do(t, http.MethodGet, "/jobs", token, nil)
	var listed JobsOutput
	if err := json.Unmarshal([]byte(answer.body), &listed.Body); err != nil || answer.status != http.StatusOK {
		t.Fatalf("GET /jobs = %d %s", answer.status, answer.body)
	}
	return listed.Body.Jobs
}

// newestPrepareJob is the newest prepare job in the view.
func newestPrepareJob(t *testing.T, listed []JobBody) JobBody {
	t.Helper()
	return newestOf(t, listed, jobs.KindPrepare)
}

// newestOf is the newest job of a kind in the view.
func newestOf(t *testing.T, listed []JobBody, kind jobs.Kind) JobBody {
	t.Helper()
	for _, job := range listed {
		if string(job.Kind) == string(kind) {
			return job
		}
	}
	t.Fatalf("no %s job in %+v", kind, listed)
	return JobBody{}
}

func TestJobs_TheViewListsAPrepareAndACancelStopsItOnTheNodeSoTheNextPlayPreparesAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	harness.play(t, admin, chrome(2))
	queued := newestPrepareJob(t, harness.listedJobs(t, admin))
	if queued.State != "queued" || queued.Reason == "" {
		t.Errorf("the prepare in the view = %+v, want queued with its reason", queued)
	}
	playback := harness.server.dependencies.Load().Playback
	runner := harness.queue.NewRunner(jobs.KindPrepare, playback.HandlePrepare)
	ran := make(chan error, 1)
	go func() { ran <- runner.RunDue(t.Context()) }()
	if command := harness.nextCommand(t); command.Type != nodeproto.CommandPrepare {
		t.Fatalf("the node was sent %+v, want the prepare", command)
	}
	if assigned := newestPrepareJob(t, harness.listedJobs(t, admin)); assigned.State != "assigned" {
		t.Errorf("the prepare once the node has it = %+v, want assigned to it, nothing waiting (D234 §7)", assigned)
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
	if cancelled := newestPrepareJob(t, harness.listedJobs(t, admin)); cancelled.State != "cancelled" {
		t.Errorf("the prepare after the cancel = %+v, want cancelled", cancelled)
	}
	if again := harness.play(t, admin, chrome(2)); !again.IsPreparing {
		t.Errorf("the next play: %+v, want the copy prepared again, not waited on", again)
	}
	if next := newestPrepareJob(t, harness.listedJobs(t, admin)); next.ID == queued.ID || next.State != "queued" {
		t.Errorf("the newest prepare = %+v, want a new one queued", next)
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
	playback := harness.server.dependencies.Load().Playback
	runner := harness.queue.NewRunner(jobs.KindExtractSubtitle, playback.HandleExtractSubtitle)
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
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	harness.play(t, admin, chrome(2))
	harness.nodes.isAway.Store(true)
	playback := harness.server.dependencies.Load().Playback
	if err := harness.queue.NewRunner(jobs.KindPrepare, playback.HandlePrepare).RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	prepare := newestPrepareJob(t, harness.listedJobs(t, admin))
	wantNode := DestinationBody{
		Destination: "node:1", Name: "node local", Reason: "not connected", Since: fakeNodeLastHeard.UnixMilli(),
	}
	if prepare.State != "queued" || prepare.Destination != "node:1" || prepare.WaitingFor == nil ||
		*prepare.WaitingFor != wantNode {
		t.Errorf("a prepare for a node that is away: %+v, waiting for %+v; want it waiting for node local, not "+
			"connected since it was last heard, with no next check: its hello brings it back (D231 §12)",
			prepare, prepare.WaitingFor)
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
