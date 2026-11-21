// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"fmt"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
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

// TestIngest_AStreamFFmpegCallsAnAttachedPictureIsTheStoresCover checks that the store's cover
// rule, stream.is_attached_picture, which its queries read, is the attached_pic disposition the
// probe names and DispositionBit stores: Go and SQL tell a cover from a picture alike.
func TestIngest_AStreamFFmpegCallsAnAttachedPictureIsTheStoresCover(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	record := observedRecord("Metropolis.mkv", "metropolis")
	record.Observed.Probe.Streams = []nodeproto.ProbeStream{
		{StreamIndex: 0, Kind: "video", Codec: "mjpeg", Disposition: []string{"attached_pic"}},
		{StreamIndex: 1, Kind: "video", Codec: "h264", Disposition: []string{"default"}},
		{StreamIndex: 2, Kind: "video", Codec: "png", Disposition: []string{"default", "attached_pic"}},
	}
	world.scan([]nodeproto.ScanRecord{record}, true)
	fileID := world.liveFilesByPath()[root+"/Metropolis.mkv"].ID
	covers := storetest.QueryRaw(t, world.database.Path(), fmt.Sprintf(
		"SELECT stream_index, is_attached_picture FROM stream WHERE bytes_file = %d ORDER BY stream_index", fileID))
	if want := []string{"0|1", "1|0", "2|1"}; !slices.Equal(covers, want) {
		t.Errorf("each stream's index and whether it is a cover: %v; want %v, the two attached pictures covers",
			covers, want)
	}
}
