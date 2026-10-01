// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// requestedSweeps records the sweeps asked for, as the job queue would run them.
type requestedSweeps struct{ names []string }

func (sweeps *requestedSweeps) RequestSweeps(names ...string) {
	sweeps.names = append(sweeps.names, names...)
}

func TestAskForAPassOnNewFiles_AFilmFoundMidScanAsksForTheIdentificationPasses(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	nodeID, _ := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/media/films"})
	fakeClock := clocktest.NewFake(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	seeded := settingstest.Seeded(t, database)
	ingester := library.NewIngester(database, fakeClock, seeded, slog.New(slog.DiscardHandler))
	sweeps := &requestedSweeps{}
	askForAPassOnNewFiles(ingester, sweeps)
	var reports librarytest.Reports
	film := nodeproto.Record{Type: nodeproto.RecordObserved, Observed: &nodeproto.Observed{
		Path: nodeproto.Path{Text: "Film (2020)/Film (2020).mkv"}, SizeBytes: 1, ModifiedAt: 1,
		Identity: nodeproto.Identity{
			Scheme: "payload-v1", Anchor: "whole-file", PayloadBytes: 1,
			PayloadHash: "5feceb66ffc86f38d952786c6d696c79c2dbc239dd4e91b46729d73a27fb57e9",
		},
	}}
	stillScanning := reports.Scan(nodeproto.ScanBegin{RootPath: "/media/films"}, []nodeproto.Record{film}, false)
	if err := ingester.IngestRecords(t.Context(), nodeID, stillScanning); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sweeps.names, []string{sweepPasses}) {
		t.Errorf("a scan still going that stored a film asked for %v, want the %q sweep", sweeps.names, sweepPasses)
	}
}
