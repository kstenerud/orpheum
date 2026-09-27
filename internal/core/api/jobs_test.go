// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
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

// prepareJob is the newest prepare job in the view.
func prepareJob(t *testing.T, listed []JobBody) JobBody {
	t.Helper()
	return newestOf(t, listed, jobs.KindPrepare)
}

// newestOf is the newest job of a kind in the view.
func newestOf(t *testing.T, listed []JobBody, kind jobs.Kind) JobBody {
	t.Helper()
	for _, job := range listed {
		if job.Kind == string(kind) {
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
	queued := prepareJob(t, harness.listedJobs(t, admin))
	if queued.State != "queued" || queued.Reason == "" {
		t.Errorf("the prepare in the view = %+v, want queued with its reason", queued)
	}
	playback := harness.server.dependencies.Load().Playback
	runner := harness.queue.NewRunner(jobs.KindPrepare, playback.HandlePrepare)
	ran := make(chan error, 1)
	go func() { ran <- runner.RunDue(t.Context()) }()
	if command := <-harness.nodes.commanded; command.Type != nodeproto.CommandPrepare {
		t.Fatalf("the node was sent %+v, want the prepare", command)
	}
	if running := prepareJob(t, harness.listedJobs(t, admin)); running.State != "running" {
		t.Errorf("the prepare once the node has it = %+v, want running", running)
	}
	cancel := "/jobs/" + strconv.FormatInt(queued.ID, 10) + "/cancel"
	if status := harness.do(t, http.MethodPost, cancel, admin, nil).status; status != http.StatusNoContent {
		t.Fatalf("POST %s: %d, want 204", cancel, status)
	}
	stop := <-harness.nodes.commanded
	if stop.Type != nodeproto.CommandCancel || stop.Cancel == nil || stop.Cancel.Job != queued.ID {
		t.Errorf("the node was then sent %+v, want a cancel of job %d", stop, queued.ID)
	}
	if err := <-ran; err != nil {
		t.Fatal(err)
	}
	if cancelled := prepareJob(t, harness.listedJobs(t, admin)); cancelled.State != "cancelled" {
		t.Errorf("the prepare after the cancel = %+v, want cancelled", cancelled)
	}
	if again := harness.play(t, admin, chrome(2)); !again.IsPreparing {
		t.Errorf("the next play: %+v, want the copy prepared again, not waited on", again)
	}
	if next := prepareJob(t, harness.listedJobs(t, admin)); next.ID == queued.ID || next.State != "queued" {
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
	_, err := harness.service.CreateUser(t.Context(), auth.NewUser{
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
	runner := harness.queue.NewRunner(jobs.KindExtractSubtitle, playback.HandleSubtitle)
	ran := make(chan error, 1)
	go func() { ran <- runner.RunDue(t.Context()) }()
	if command := <-harness.nodes.commanded; command.Type != nodeproto.CommandExtractSubtitle {
		t.Fatalf("the node was sent %+v, want the extraction", command)
	}
	cancel := "/jobs/" + strconv.FormatInt(extraction.ID, 10) + "/cancel"
	if status := harness.do(t, http.MethodPost, cancel, admin, nil).status; status != http.StatusNoContent {
		t.Fatalf("POST %s: %d, want 204", cancel, status)
	}
	if stop := <-harness.nodes.commanded; stop.Type != nodeproto.CommandCancel || stop.Cancel.Job != extraction.ID {
		t.Errorf("the node was then sent %+v, want a cancel of job %d", stop, extraction.ID)
	}
	if err := <-ran; err != nil {
		t.Fatal(err)
	}
}
