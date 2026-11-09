// SPDX-License-Identifier: AGPL-3.0-or-later

package jobs

import (
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
)

func TestDestinations_ANameSaysWhatItIsAndReadsAsAPersonSaysIt(t *testing.T) {
	t.Parallel()
	nodeName := func(nodeID int64) string { return "node-" + strconv.FormatInt(nodeID, 10) }
	for _, check := range []struct {
		destination Destination
		parts       DestinationParts
		words       string
	}{
		{DestinationTMDBAPI, DestinationParts{}, "TMDB"},
		{DestinationTMDBImages, DestinationParts{}, "TMDB's image server"},
		{DestinationOpenSubtitles, DestinationParts{}, "OpenSubtitles"},
		{DestinationCoreStorage, DestinationParts{}, "the server's own disk"},
		{DestinationAnyNode, DestinationParts{}, "a node"},
		{NodeDestination(3), DestinationParts{NodeID: 3, IsNode: true}, "node node-3"},
		{
			RootDestination(3, "/media/films:new"),
			DestinationParts{NodeID: 3, Path: "/media/films:new", IsRoot: true},
			"/media/films:new on node node-3",
		},
		{
			PreparedDirectoryDestination(4),
			DestinationParts{NodeID: 4, IsPreparedDirectory: true},
			"the prepared directory on node node-4",
		},
		{SlotDestination(5, "remux"), DestinationParts{NodeID: 5, IsSlot: true}, "a free slot on node node-5"},
		{ReaderDestination(5, "zfs:tank"), DestinationParts{NodeID: 5, IsSlot: true}, "a free slot on node node-5"},
		{
			QuickPassDestination(5, "zfs:tank"),
			DestinationParts{NodeID: 5, IsQuickPass: true},
			"a scan's first look at a disk on node node-5",
		},
		{
			LastReaderDestination(5, "zfs:tank"),
			DestinationParts{NodeID: 5, IsLastReader: true},
			"quicker work on a disk of node node-5",
		},
		{
			PlayingDestination(6),
			DestinationParts{NodeID: 6, IsPlaying: true},
			"the film playing on node node-6 to keep up",
		},
	} {
		if parts := PartsOf(check.destination); parts != check.parts {
			t.Errorf("%q reads as %+v, want %+v", check.destination, parts, check.parts)
		}
		if words := DescribeDestination(check.destination, nodeName); words != check.words {
			t.Errorf("%q is described as %q, want %q", check.destination, words, check.words)
		}
	}
}

func TestPark_TheProbeBackoffIsTheSettings(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	settingstest.Set(t, jobs.config, settings.DestinationFirstProbeDelaySeconds, "10")
	settingstest.Set(t, jobs.config, settings.DestinationLongestProbeDelaySeconds, "25")
	for index, want := range []time.Duration{10 * time.Second, 20 * time.Second, 25 * time.Second} {
		parking := Parking{Destination: DestinationTMDBAPI, Reason: "unreachable", IsProbeFailure: index > 0}
		jobs.queue.Park(t.Context(), parking)
		state, _ := jobs.queue.DestinationState(DestinationTMDBAPI)
		if state.NextProbeAt.Sub(jobs.clock.Now()) != want {
			t.Errorf("probe %d is due in %v, want %v: destinationFirstProbeDelaySeconds doubling to "+
				"destinationLongestProbeDelaySeconds", index+1, state.NextProbeAt.Sub(jobs.clock.Now()), want)
		}
	}
}
