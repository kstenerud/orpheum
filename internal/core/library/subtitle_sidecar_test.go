// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

func TestSidecarTrackOf_ReadsTheLanguageAndFlagsAfterTheVideosName(t *testing.T) {
	t.Parallel()
	forced, deaf, isDefault := DispositionBit("forced"), DispositionBit("hearing_impaired"), DispositionBit("default")
	const video = "/media/Copper Kettle (2015)/Copper Kettle (2015).mkv"
	for name, want := range map[string]sidecarTrack{
		"Copper Kettle (2015).srt":              {codec: "subrip"},
		"Copper Kettle (2015).en.srt":           {codec: "subrip", langRaw: "en", lang: "en"},
		"Copper Kettle (2015).ger.forced.srt":   {codec: "subrip", langRaw: "ger", lang: "de", disposition: forced},
		"Copper Kettle (2015).forced.pt-BR.ass": {codec: "ass", langRaw: "pt-BR", lang: "pt-BR", disposition: forced},
		"Copper Kettle (2015).en.SDH.vtt":       {codec: "webvtt", langRaw: "en", lang: "en", disposition: deaf},
		"Copper Kettle (2015).es.hi.default.srt": {
			codec: "subrip", langRaw: "es", lang: "es", disposition: deaf | isDefault,
		},
		"Copper Kettle (2015).commentary.en.srt": {codec: "subrip", langRaw: "en", lang: "en"},
		"Copper Kettle (2015).fr.cc.ssa":         {codec: "ssa", langRaw: "fr", lang: "fr", disposition: deaf},
		"Copper Kettle (2015).en.fr.srt":         {codec: "subrip", langRaw: "en", lang: "en"},
		"Something else entirely.en.srt":     {codec: "subrip"},
	} {
		if got := sidecarTrackOf("/media/Copper Kettle (2015)/"+name, video); got != want {
			t.Errorf("%s: %+v, want %+v", name, got, want)
		}
	}
}

// subtitleSidecar is a subtitle file's record, with the payload hash a scan gives it.
func subtitleSidecar(path, content string) nodeproto.Record {
	record := sidecar(path, nodeproto.SidecarSubtitle, content)
	record.Sidecar.Identity = &nodeproto.Identity{
		Scheme: "payload-v1", Anchor: "whole-file", PayloadHash: hashOf(content), PayloadBytes: int64(len(content)),
	}
	return record
}

func TestIngest_ASubtitleFileIsATrackOfItsVideoUntilItsVideoIsGone(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	// A subtitle file that belongs to no video is recorded, with no track, and the scan goes on.
	world.scan([]nodeproto.Record{subtitleSidecar("Metropolis (1995)/Metropolis (1995).en.forced.srt", "1\n")}, true)
	records := []nodeproto.Record{
		subtitleSidecar("Metropolis (1995)/Metropolis (1995).en.forced.srt", "1\n"),
		observed("Metropolis (1995)/Metropolis (1995).mkv", "heat"),
	}
	world.scan(records, true)
	files := world.live()
	sidecarFile := files[root+"/Metropolis (1995)/Metropolis (1995).en.forced.srt"]
	video := files[root+"/Metropolis (1995)/Metropolis (1995).mkv"]
	if hex.EncodeToString(sidecarFile.PayloadHash) != hashOf("1\n") {
		t.Errorf("the subtitle file's payload hash is %x, want its bytes' (D179)", sidecarFile.PayloadHash)
	}
	track := func() (sqlcgen.GetSidecarStreamRow, error) {
		return store.OrNotFound(world.database.Read().GetSidecarStream(t.Context(), sidecarFile.ID))
	}
	first, err := track()
	if err != nil {
		t.Fatalf("the subtitle file has no track: %v", err)
	}
	if first.VideoFile != video.ID || first.Codec == nil || *first.Codec != "subrip" || first.Language == nil ||
		*first.Language != "en" || first.DispositionFlags != DispositionBit("forced") {
		t.Errorf("the track = %+v, want an English forced subrip track of the video", first)
	}
	world.scan(records, true)
	if again, err := track(); err != nil || again.ID != first.ID {
		t.Errorf("after a rescan the track is %+v, %v; want the same row, whose id a job names", again, err)
	}
	world.scan(records[:1], true)
	if _, err := track(); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("with its video gone the subtitle file still has a track: %v", err)
	}
}
