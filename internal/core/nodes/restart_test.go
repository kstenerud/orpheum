// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// resolvedBy is an assigned job's runner that hands the test each answer a job is resolved with.
type resolvedBy chan error

func (resolved resolvedBy) ResolveBy(_ context.Context, id int64, decide func(jobs.Job) error) (bool, error) {
	resolved <- decide(jobs.Job{ID: id, Kind: jobs.KindPrepare})
	return true, nil
}

// core is a core's node service and exchanges, the attempts kept in cache.
type core struct {
	*fixture
	exchanges *Exchanges
	resolved  resolvedBy
	logged    *bytes.Buffer
}

// startCore starts a core over database and cache, as the server does: its exchanges load the
// attempts cache.db kept, and resolve prepares into resolved.
func startCore(t *testing.T, harness *fixture, cache *store.Cache) core {
	t.Helper()
	started := core{fixture: harness, resolved: make(resolvedBy, 2), logged: &bytes.Buffer{}}
	started.exchanges = NewExchanges(ExchangeParts{
		Sender: harness.service, Rooms: harness.service, Clock: harness.clock, Random: rand.Reader,
		Logger: slog.New(slog.NewJSONHandler(started.logged, nil)), Cache: cache, Settings: harness.settings,
		MaxSilence: time.Minute,
	})
	started.exchanges.Handle(jobs.KindPrepare, Assignee{
		Runner: started.resolved,
		Answer: func(context.Context, jobs.Job, Attempt, nodeproto.Result) error { return nil },
		Unanswered: func(_ context.Context, job jobs.Job, _ Attempt, err error) error {
			return UnansweredOutcome(job, err, nil)
		},
	})
	if err := started.exchanges.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	harness.service.SetResultSink(started.exchanges)
	return started
}

// restartedFixture is the node service of a core that started again over the same persistent.db: it
// has heard no node since.
func restartedFixture(t *testing.T, before *fixture) *fixture {
	t.Helper()
	service := NewService(ServiceParts{
		Database: before.database, Clock: before.clock, Settings: before.settings,
		Destinations: before.destinations, Logger: slog.New(slog.DiscardHandler), PollHold: time.Second,
	})
	server := httptest.NewServer(service.Handler())
	t.Cleanup(server.Close)
	return &fixture{
		database: before.database, settings: before.settings, service: service, destinations: before.destinations,
		server: server, clock: before.clock,
	}
}

// A core and its node that start again together, as at an update or a reboot: the work the core
// gave the node's last start, kept in cache.db, is interrupted at the node's hello naming its new
// start, the tool's failure tried again at no cost of an attempt, and no broken invariant; its
// status, not holding it, finds nothing lost (D238 §3, D234 §6). The node's hello from its last
// start, the core alone having started again, interrupts nothing (D233 §2). Each core's first hello
// from the node is its first since it started: it has no start of its own to compare.
func TestHello_ANodeThatStartedAgainWithTheCoreInterruptsTheWorkItsLastStartWasGiven(t *testing.T) {
	t.Parallel()
	cache := storetest.OpenCache(t)
	before := startCore(t, newFixture(t, time.Second), cache)
	reply := before.hello(t, nodeproto.Hello{Name: "local", StartID: "first"})
	prepare := nodeproto.Command{Work: nodeproto.Work{Type: nodeproto.CommandPrepare, Prepare: &nodeproto.Prepare{}}}
	assigned := before.exchanges.Assign(t.Context(), reply.NodeID, jobs.Job{ID: 7, Kind: jobs.KindPrepare}, prepare)
	if !jobqueue.IsAssigned(assigned) {
		t.Fatalf("Assign answered %v, want job 7 assigned", assigned)
	}
	jobqueue.HandOver(t.Context(), assigned)

	coreAlone := startCore(t, restartedFixture(t, before.fixture), cache)
	coreAlone.hello(t, nodeproto.Hello{Name: "local", StartID: "first"})
	select {
	case answer := <-coreAlone.resolved:
		t.Fatalf("the hello of the start job 7 was given to resolved it with %v; want it held", answer)
	default:
	}

	after := startCore(t, restartedFixture(t, before.fixture), cache)
	after.hello(t, nodeproto.Hello{Name: "local", StartID: "second"})
	answer := testutil.Receive(t, after.resolved, "job 7's answer")
	if !errors.Is(answer, ErrNodeRestarted) {
		t.Errorf("job 7, its node started again with the core, was resolved with %v; want its work lost, "+
			"tried again", answer)
	}
	after.postStatus(t, reply.NodeID, nodeproto.Status{SentAt: 1})
	select {
	case again := <-after.resolved:
		t.Errorf("the status of the node that started again resolved a job again with %v", again)
	default:
	}
	if count := after.exchanges.BrokenInvariants(reply.NodeID); count != 0 ||
		strings.Contains(after.logged.String(), "a broken invariant") {
		t.Errorf("%d broken invariants counted, logged %s; want none: a restart is not one", count, after.logged)
	}
	if kept, err := cache.Read().ListNodeAttempts(t.Context()); err != nil || len(kept) != 0 {
		t.Errorf("cache.db keeps %+v, %v; want job 7's attempt forgotten", kept, err)
	}
}
