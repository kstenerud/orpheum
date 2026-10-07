// SPDX-License-Identifier: GPL-3.0-or-later

package jobqueue_test

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
)

// A person waiting for a job keeps it wanted, a later time standing over an earlier one; once its
// time passes it falls to the priority given, and is returned to be told to whoever holds it; a job
// no one said is wanted is left as it is (D234 §3).
func TestDemoteUnwanted_GivesTheJobsNoOneWaitsForAnyMoreTheirNewPriority(t *testing.T) {
	t.Parallel()
	fixed := newFixture(t, nil, nil)
	fixed.enqueue(t, jobqueue.Request[target]{Kind: "prepare", Target: "film", Priority: 1000})
	fixed.enqueue(t, jobqueue.Request[target]{Kind: "prepare", Target: "show", Priority: 1000})
	fixed.enqueue(t, jobqueue.Request[target]{Kind: "prepare", Target: "sweep", Priority: 1000})
	now := fixed.clock.Now()
	for wanted, until := range map[target]time.Time{"film": now.Add(10 * time.Minute), "show": now.Add(time.Hour)} {
		if err := fixed.queue.KeepWanted(t.Context(), "prepare", wanted, until); err != nil {
			t.Fatal(err)
		}
	}
	if err := fixed.queue.KeepWanted(t.Context(), "prepare", "show", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	fixed.clock.Advance(20 * time.Minute)
	demoted, err := fixed.queue.DemoteUnwanted(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(demoted) != 1 || demoted[0].Target != "film" {
		t.Fatalf("demoted %+v, want the film alone: the show is wanted for an hour, the sweep never was", demoted)
	}
	if film, show := fixed.row(t, "film"), fixed.row(t, "show"); film.Priority != 0 || film.WantedUntil != nil ||
		show.Priority != 1000 || show.WantedUntil == nil {
		t.Errorf("the film is %d (%v), the show %d (%v); want the film fallen to 0, the show still wanted",
			film.Priority, film.WantedUntil, show.Priority, show.WantedUntil)
	}
	if again, err := fixed.queue.DemoteUnwanted(t.Context(), 0); err != nil || len(again) != 0 {
		t.Errorf("a second look demoted %+v (%v) again, want nothing", again, err)
	}
}

// A job no person waits for any more, released as its page is left, falls at the next demotion,
// though its time had not passed; one never said wanted is not made wanted by a release (D258 §2).
func TestReleaseWanted_ANoLongerWantedJobFallsAtTheNextDemotion(t *testing.T) {
	t.Parallel()
	fixed := newFixture(t, nil, nil)
	fixed.enqueue(t, jobqueue.Request[target]{Kind: "prepare", Target: "film", Priority: 1000})
	fixed.enqueue(t, jobqueue.Request[target]{Kind: "prepare", Target: "sweep", Priority: 1000})
	wantedUntil := fixed.clock.Now().Add(10 * time.Minute)
	if err := fixed.queue.KeepWanted(t.Context(), "prepare", "film", wantedUntil); err != nil {
		t.Fatal(err)
	}
	for _, left := range []target{"film", "sweep"} {
		if err := fixed.queue.ReleaseWanted(t.Context(), "prepare", left); err != nil {
			t.Fatal(err)
		}
	}
	demoted, err := fixed.queue.DemoteUnwanted(t.Context(), 0)
	if err != nil || len(demoted) != 1 || demoted[0].Target != "film" {
		t.Fatalf("demoted %+v (%v); want the film alone, released", demoted, err)
	}
	if sweep := fixed.row(t, "sweep"); sweep.Priority != 1000 || sweep.WantedUntil != nil {
		t.Errorf("the sweep is %d (%v); want it as it was", sweep.Priority, sweep.WantedUntil)
	}
}
