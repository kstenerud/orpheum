// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"context"
	"crypto/rand"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// requestedSweeps records the sweeps asked for, as the job queue would run them.
type requestedSweeps struct{ names []string }

func (sweeps *requestedSweeps) RequestSweeps(names ...string) {
	sweeps.names = append(sweeps.names, names...)
}

// A film found mid-scan owes its own library a pass, which the owed passes' sweep runs once however
// many batches asked; the other libraries' passes wait for their own reasons (audit 2026-10-25, C8).
func TestAskForAPassOnNewFiles_AFilmFoundMidScanAsksForTheIdentificationPasses(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	nodeID, _ := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/media/films"})
	librarytest.LibraryOn(t, database, nodeID,
		librarytest.Library{Name: "Shows", Kind: "series", MetadataSource: "nfo", Root: "/media/shows"})
	fakeClock := clocktest.NewFake(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	seeded := settingstest.Seeded(t, database)
	ingester := library.NewIngester(database, fakeClock, seeded, slog.New(slog.DiscardHandler))
	pass := identify.NewPass(database, fakeClock, slog.New(slog.DiscardHandler), nil, seeded)
	sweeps := &requestedSweeps{}
	wakeOwedPasses(pass, sweeps)
	askForAPassOnNewFiles(ingester, pass)
	film := nodeproto.ScanRecord{Type: nodeproto.RecordObserved, Observed: &nodeproto.Observed{
		Path: nodeproto.Path{Text: "Film (2020)/Film (2020).mkv"}, SizeBytes: 1, ModifiedAt: 1,
		Identity: nodeproto.Identity{
			Scheme: "payload-v1", Anchor: "whole-file", PayloadBytes: 1,
			PayloadHash: "5feceb66ffc86f38d952786c6d696c79c2dbc239dd4e91b46729d73a27fb57e9",
		},
	}}
	stillScanning := librarytest.Scan(
		nodeproto.ScanBegin{RootPath: "/media/films"}, []nodeproto.ScanRecord{film}, false)
	if err := ingester.IngestScan(t.Context(), nodeID, "/media/films", stillScanning); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sweeps.names, []string{sweepOwedPasses}) {
		t.Errorf("a scan still going that stored a film asked for %v, want the %q sweep", sweeps.names, sweepOwedPasses)
	}
	if passed, err := pass.IdentifyOwed(t.Context()); err != nil || passed != 1 {
		t.Errorf("the owed passes ran %d, %v; want the films' library's alone", passed, err)
	}
}

func TestWakeOwedPasses_ALibraryOwingAPassAsksForTheOwedPassesSweep(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	fakeClock := clocktest.NewFake(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	pass := identify.NewPass(database, fakeClock, slog.New(slog.DiscardHandler), nil, settingstest.Seeded(t, database))
	sweeps := &requestedSweeps{}
	wakeOwedPasses(pass, sweeps)
	pass.OwePass(1)
	if !slices.Equal(sweeps.names, []string{sweepOwedPasses}) {
		t.Errorf("a library owing a pass asked for %v, want the %q sweep", sweeps.names, sweepOwedPasses)
	}
}

// toldSender is a node that records what it is told, and has no work sent to it.
type toldSender struct{ told chan nodeproto.Command }

func (sender toldSender) Enqueue(_ int64, command nodeproto.Command) { sender.told <- command }

func (toldSender) Send(context.Context, int64, nodeproto.Command) error { return nil }

func (toldSender) ParkUnlessConnected(context.Context, int64) error { return nil }

func (toldSender) Withdraw(int64, string) bool { return false }

func (toldSender) StartOf(int64) string { return "" }

func (toldSender) Unanswered(_ context.Context, _ int64, err error) error { return err }

func (toldSender) Unpark(context.Context, jobs.Destination) {}

func (toldSender) Resources(context.Context, int64) (nodes.Resources, bool, error) {
	return nodes.Resources{}, false, nil
}

// tankRooms are a connected node whose one disk, tank, holds /media.
type tankRooms struct{ toldSender }

func (tankRooms) Resources(context.Context, int64) (nodes.Resources, bool, error) {
	return nodes.Resources{
		Capacity:     nodeproto.Capacity{EncodeSlotCount: 4, RemuxSlotCount: 4},
		Devices:      nodeproto.Devices{{Key: "zfs:tank", MountPaths: []string{"/media"}}},
		ReaderLimits: map[string]int{"zfs:tank": 4},
	}, true, nil
}

// filmsOnQuickPass are the scans on their quick pass: /media/films's, on node 1.
type filmsOnQuickPass struct{}

func (filmsOnQuickPass) RootsOnQuickPass(_ context.Context, nodeID int64) ([]string, error) {
	if nodeID != 1 {
		return nil, nil
	}
	return []string{"/media/films"}, nil
}

// The server's exchanges hold a background read-through on a disk back while a scan of it is on its
// quick pass, as the scans say (D255 §3).
func TestHoldSlowReadsForQuickPasses_ABackgroundReadThroughWaitsForTheScansQuickPass(t *testing.T) {
	t.Parallel()
	now := clocktest.NewFake(time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC))
	rooms := tankRooms{toldSender{told: make(chan nodeproto.Command, 1)}}
	exchanges := nodes.NewExchanges(nodes.ExchangeParts{
		Sender: rooms, Rooms: rooms, Clock: now, Random: rand.Reader, Logger: slog.New(slog.DiscardHandler),
		Cache: storetest.OpenCache(t), Settings: settingstest.Seeded(t, storetest.OpenMigrated(t)),
		MaxSilence: time.Hour,
	})
	holdSlowReadsForQuickPasses(exchanges, filmsOnQuickPass{})
	video := nodeproto.BytesFile{RootPath: "/media", Path: []byte("films/F.mkv")}
	readThrough := nodeproto.Command{Work: nodeproto.Work{
		Type: nodeproto.CommandReadThrough, ReadThrough: &nodeproto.ReadThrough{Video: video},
	}}
	answer := exchanges.Assign(t.Context(), 1, jobs.Job{ID: 1, Kind: jobs.KindReadThrough}, readThrough)
	if parking, isParked := jobqueue.ParkingOf(answer); !isParked ||
		parking.Destination != jobs.QuickPassDestination(1, "zfs:tank") {
		t.Errorf("a background read-through on tank during its quick pass was answered %v; want it waiting", answer)
	}
}
