// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakePreparedShelf is a prepared shelf whose last eviction left usage.
type fakePreparedShelf struct{ usage prepared.Usage }

func (kept fakePreparedShelf) Evict(context.Context) (prepared.Usage, error) { return kept.usage, nil }

func (kept fakePreparedShelf) LastUsage() (prepared.Usage, bool) { return kept.usage, true }

func (fakePreparedShelf) EvictUnfinished() ([]string, error) { return nil, nil }

func (fakePreparedShelf) SetStoreTiers(map[string]nodeproto.KeepTier) {}

// tieredShelf is a prepared shelf that keeps the tiers it was given.
type tieredShelf struct {
	fakePreparedShelf
	tiers map[string]nodeproto.KeepTier
}

func (shelf *tieredShelf) SetStoreTiers(tiers map[string]nodeproto.KeepTier) { shelf.tiers = tiers }

// A poll's answer that says how strongly the chunk stores are kept gives it to the prepared
// directory's eviction; one that says nothing leaves its last (D309 §8).
func TestAgent_KeepsTheChunkStoresByTheTiersThePollSays(t *testing.T) {
	t.Parallel()
	shelf := &tieredShelf{}
	var logged bytes.Buffer
	agent := &Agent{workers: Workers{PreparedShelf: shelf}, logger: slog.New(slog.NewTextHandler(&logged, nil))}
	said := map[string]nodeproto.KeepTier{"1/chunks-q400k360p/g": nodeproto.KeepInUse}
	for range 2 {
		agent.takeAnswersSettings(t.Context(), nodeproto.Commands{StoreTiers: said})
	}
	agent.takeAnswersSettings(t.Context(), nodeproto.Commands{})
	if shelf.tiers["1/chunks-q400k360p/g"] != nodeproto.KeepInUse {
		t.Errorf("the shelf's tiers = %v; want the poll's", shelf.tiers)
	}
	// Said as it changes, once: the poll answers every half minute.
	if lines := strings.Count(logged.String(), "kept the chunk stores"); lines != 1 ||
		!strings.Contains(logged.String(), "inUse=1") {
		t.Errorf("the log = %q; want the change said once, with one store in use", logged.String())
	}
}

// The node posts its status as often as the core's hello reply says, and then as each poll's answer
// says when it changes; an answer that says none leaves the last (the constants sweep, 2026-10-21).
func TestAgent_PostsItsStatusAsOftenAsTheCoreSays(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	core.mutex.Lock()
	core.statusEverySeconds = 4
	core.mutex.Unlock()
	cadences := make(chan time.Duration, 4)
	workers := Workers{Scanner: fakeScanner{}, Artworker: fakeArtworker{}}
	workers.SetStatusEvery = func(every time.Duration) { cadences <- every }
	startAgentWithWorkers(t, socket, workers)
	if every := testutil.Receive(t, cadences, "the hello's cadence"); every != 4*time.Second {
		t.Errorf("after the hello the node posts every %v; want the 4 s its reply said", every)
	}
	var logged bytes.Buffer
	agent := &Agent{logger: slog.New(slog.NewTextHandler(&logged, nil)), workers: Workers{
		SetStatusEvery: func(every time.Duration) { cadences <- every },
	}}
	agent.statusEvery.Store(nodeproto.StartingStatusEverySeconds)
	for _, said := range []int{nodeproto.StartingStatusEverySeconds, 0, 2, 2, 0} {
		agent.takeAnswersSettings(t.Context(), nodeproto.Commands{Tuning: nodeproto.Tuning{StatusEverySeconds: said}})
	}
	if every := testutil.Receive(t, cadences, "the poll's cadence"); every != 2*time.Second {
		t.Errorf("after the polls the node posts every %v; want the 2 s they said", every)
	}
	select {
	case every := <-cadences:
		t.Errorf("the node set its cadence again, to %v, though no poll changed it", every)
	default:
	}
}

// A running read-through says how far it is as often as the core's tuning last said, and at the starting
// cadence before it says (D234 §5; the constants sweep, 2026-10-21).
func TestAgent_SaysARunsProgressAsOftenAsTheCoreSays(t *testing.T) {
	t.Parallel()
	agent := New("", nodeproto.Hello{}, nil, Workers{}, nil, slog.New(slog.DiscardHandler))
	if every := agent.progressEvery(); every != nodeproto.StartingProgressEverySeconds*time.Second {
		t.Errorf("before the core says, progress is said every %v; want the starting %d s", every,
			nodeproto.StartingProgressEverySeconds)
	}
	agent.takeTuning(t.Context(), nodeproto.Tuning{ProgressEverySeconds: 5})
	agent.takeTuning(t.Context(), nodeproto.Tuning{})
	if every := agent.progressEvery(); every != 5*time.Second {
		t.Errorf("progress is said every %v; want the 5 s the core said, which a tuning saying none leaves", every)
	}
}

// unfinishedShelf tells evicted of each eviction of the makings that never finished.
type unfinishedShelf struct {
	fakePreparedShelf
	evicted chan struct{}
}

func (shelf unfinishedShelf) EvictUnfinished() ([]string, error) {
	shelf.evicted <- struct{}{}
	return []string{"2/remux/bbbbbbbbbbbbbbbb"}, nil
}

// At each hello the node evicts the makings in its prepared directory that never finished, and what
// a whole-film prepare left (D238 §2, D344 §1).
func TestAgent_EachHelloEvictsTheMakingsThatNeverFinished(t *testing.T) {
	t.Parallel()
	_, socket := startFakeCore(t)
	shelf := unfinishedShelf{evicted: make(chan struct{}, 4)}
	startAgentWithShelf(t, socket, shelf)
	testutil.Receive(t, shelf.evicted, "the first hello's eviction")
}

// Each hello lists the devices the node finds then, since a disk may have come or gone since the
// last (D240 §5).
func TestAgent_TheHelloListsTheDevicesFoundAsItIsSaid(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	startAgentWithWorkers(t, socket, Workers{
		Scanner: fakeScanner{}, Artworker: fakeArtworker{},
		FindDevices: func() ([]nodeproto.Device, error) {
			return []nodeproto.Device{{Key: "zfs:tank", Kind: nodeproto.DeviceSpinning}}, nil
		},
	})
	testutil.Receive(t, core.helloHeard, "the first hello")
	core.mutex.Lock()
	hello := core.hellos[0]
	core.mutex.Unlock()
	if len(hello.Devices) != 1 || hello.Devices[0].Key != "zfs:tank" {
		t.Errorf("the hello listed devices %+v, want the one found", hello.Devices)
	}
}
