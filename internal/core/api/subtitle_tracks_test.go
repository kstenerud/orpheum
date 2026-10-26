// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// germanStreamID is the stream row of film 2's second text track, German SubRip at stream index 4,
// as shelveSecondTextTrack records it.
const germanStreamID = 9

// shelveSecondTextTrack records a second text track in film 2, beside its ASS and its PGS.
func (harness testHarness) shelveSecondTextTrack(t *testing.T) {
	t.Helper()
	librarytest.Stream(t, harness.database, librarytest.StreamSpec{
		ID: 9, BytesFile: 2, StreamIndex: 4, Kind: "subtitle", Codec: "subrip", Language: "de",
	})
}

// servedASS is the ASS a playback serves for each track it offers, by stream row, or the status it
// answers with when it serves none.
func (harness testHarness) servedASS(t *testing.T, user string, decided decision) map[int64]string {
	t.Helper()
	served := map[int64]string{}
	for _, track := range decided.Subtitles {
		answer := harness.getBytePath(t, track.ASSURL, user, true, "")
		served[track.StreamID] = answer.body
		if answer.status != http.StatusOK {
			served[track.StreamID] = strconv.Itoa(answer.status)
		}
	}
	return served
}

// A film's text tracks are extracted by one job, from one read of it, not by one job and one read
// each (D340; the audit of 2026-10-25, N3).
func TestSubtitles_AVideosTextTracksAreExtractedByOneJobFromOneRead(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSubtitles(t)
	harness.shelveSecondTextTrack(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	decided := harness.play(t, user, chrome(2))
	if queued := harness.subtitleJobCount(t); queued != 1 {
		t.Errorf("playing a film with two text tracks queued %d extractions, want one for the film", queued)
	}
	answer := nodeproto.SubtitleText{Tracks: []nodeproto.TrackText{
		{StreamIndex: 2, ASS: []byte("ass en"), VTT: []byte("vtt en")},
		{StreamIndex: 4, ASS: []byte("ass de"), VTT: []byte("vtt de")},
	}}
	command, err := harness.extract(t, 90, answer)
	if err != nil {
		t.Fatalf("the job ended %v, want done", err)
	}
	if sent := command.ExtractSubtitle; sent == nil || !slices.Equal(sent.StreamIndexes, []int{2, 4}) {
		t.Fatalf("the node was sent %+v, want both text tracks, not the PGS, in one command", command)
	}
	served := harness.servedASS(t, user, decided)
	if want := map[int64]string{subtitleStreamID: "ass en", germanStreamID: "ass de"}; !maps.Equal(served, want) {
		t.Errorf("the tracks served %v, want %v", served, want)
	}
}

// A track FFmpeg cannot make fails alone: the others' texts are kept, and the next play extracts it
// alone again (D340).
func TestSubtitles_ATrackThatFailedFailsAloneAndTheNextPlayExtractsOnlyIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSubtitles(t)
	harness.shelveSecondTextTrack(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	decided := harness.play(t, user, chrome(2))
	answer := nodeproto.SubtitleText{Tracks: []nodeproto.TrackText{
		{StreamIndex: 2, ASS: []byte("ass en"), VTT: []byte("vtt en")},
		{StreamIndex: 4, Failure: "ffmpeg: bad"},
	}}
	if _, err := harness.extract(t, 90, answer); err == nil || !strings.Contains(err.Error(), "stream 4: ffmpeg: bad") {
		t.Errorf("the job ended %v, want the German track's failure", err)
	}
	served := harness.servedASS(t, user, decided)
	if want := map[int64]string{subtitleStreamID: "ass en", germanStreamID: "502"}; !maps.Equal(served, want) {
		t.Errorf("after one track failed, the tracks served %v, want %v", served, want)
	}
	harness.finishJobs(t)
	harness.play(t, user, chrome(2))
	command, err := harness.extract(t, 91, trackText(4, "ass de", "vtt de"))
	if err != nil {
		t.Fatalf("the second job ended %v, want done", err)
	}
	if sent := command.ExtractSubtitle; sent == nil || !slices.Equal(sent.StreamIndexes, []int{4}) {
		t.Errorf("played again, the node was sent %+v, want the failed track alone", command)
	}
	served = harness.servedASS(t, user, decided)
	if want := map[int64]string{subtitleStreamID: "ass en", germanStreamID: "ass de"}; !maps.Equal(served, want) {
		t.Errorf("after it was made again, the tracks served %v, want %v", served, want)
	}
}
