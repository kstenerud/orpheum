// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

func TestIngest_AFileObservedAgainKeepsItsStreamRowsAndDropsOnlyWhatItLost(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{observedRecord("Metropolis.mkv", "metropolis")}, true)
	fileID := world.liveFilesByPath()[root+"/Metropolis.mkv"].ID
	idsByIndex := func() map[int64]int64 {
		t.Helper()
		streams, err := world.database.Read().ListStreamsOfBytesFile(t.Context(), fileID)
		if err != nil {
			t.Fatal(err)
		}
		ids := map[int64]int64{}
		for _, stream := range streams {
			ids[*stream.StreamIndex] = stream.ID
		}
		return ids
	}
	first := idsByIndex()

	world.scan([]nodeproto.ScanRecord{observedRecord("Metropolis.mkv", "metropolis")}, true)
	if again := idsByIndex(); len(again) != 2 || again[0] != first[0] || again[1] != first[1] {
		t.Errorf("after a rescan the stream ids are %v, want %v: an address or a job names them", again, first)
	}

	remuxed := observedRecord("Metropolis.mkv", "metropolis")
	remuxed.Observed.Probe.Streams = remuxed.Observed.Probe.Streams[:1]
	remuxed.Observed.Probe.Streams[0].Codec = "hevc"
	world.scan([]nodeproto.ScanRecord{remuxed}, true)
	streams, err := world.database.Read().ListStreamsOfBytesFile(t.Context(), fileID)
	if err != nil {
		t.Fatal(err)
	}
	if len(streams) != 1 || streams[0].ID != first[0] || *streams[0].Codec != "hevc" {
		t.Errorf("after the audio went and the video changed: %+v, want the video's row, changed in place", streams)
	}
}
