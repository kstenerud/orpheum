// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// TestPlayback_ACutFilmStartsOnItsLadderAndItsStallsStartNothing plays a film end to end: the sweep
// reads it through for its keyframes, its playback starts on its ladder, at the top with no link
// remembered, its chunks' master playlist by the playback, and a stall in a play report on a slow
// link starts nothing, its player moving along the ladder by itself (D260).
func TestPlayback_ACutFilmStartsOnItsLadderAndItsStallsStartNothing(t *testing.T) {
	t.Parallel()
	film := cutFilmPlaying(t, playableFilm)
	server, token, playing := film.server, film.token, film.playing
	stalled := requestAPI(t, http.MethodPost, server.baseURL+"/playback/"+playing.MediaToken+"/report", token,
		map[string]any{
			"reportSeq": 1, "sentMonoMs": 1_000_000, "positionMs": 12_000, "positionReadMonoMs": 1_000_000,
			"events": []any{}, "spans": []any{}, "linkKbps": 2000,
			"stalls": []any{map[string]any{"positionMs": 12_000}},
		})
	if stalled.status != http.StatusOK || strings.Contains(stalled.body, "lowerQuality") {
		t.Errorf("a stall on the ladder's report = %d %s; want nothing offered", stalled.status, stalled.body)
	}
}

// ladderFilm is a cut film playing on its ladder, end to end: its server, the owner's session, and
// the playback's token and ladder offer.
type ladderFilm struct {
	server  runningServer
	token   string
	playing ladderPlaying
}

// ladderPlaying is a playback's start as POST /playback says it, on its film's ladder.
type ladderPlaying struct {
	MediaToken string `json:"mediaToken"`
	OnLadder   *struct {
		URL         string `json:"url"`
		QualityKbps int    `json:"qualityKbps"`
	} `json:"onLadder"`
	Tracks struct {
		Pictures []struct {
			Stream     string `json:"stream"`
			IsOriginal bool   `json:"isOriginal"`
		} `json:"pictures"`
	} `json:"tracks"`
}

// A film whose node encodes on a GPU, as the fake FFmpeg's every trial says it does, is cut at
// chunkQualitiesHardware's qualities, and its start names the pictures its ladder offers, the
// film's own first, by stream (D323 §3; the tracks the player names, the owner, 2026-10-20).
func TestPlayback_AGPUNodesFilmIsCutAtTheHardwareLadderAndItsStartNamesItsPictures(t *testing.T) {
	t.Parallel()
	film := cutFilmPlaying(t, playableFilm)
	server, token, playing := film.server, film.token, film.playing
	if pictures := playing.Tracks.Pictures; len(pictures) == 0 || pictures[0].Stream != "picture" ||
		!pictures[0].IsOriginal {
		t.Errorf("the start's pictures = %+v; want the ladder's, the film's own first, by stream", pictures)
	}
	changed := requestAPI(t, http.MethodPut, server.baseURL+"/config", token, map[string]any{
		"valuesByKey": map[string]any{"chunkQualitiesHardware": "1600k@600p"},
	})
	if changed.status != http.StatusOK && changed.status != http.StatusNoContent {
		t.Fatalf("PUT /config = %d %s", changed.status, changed.body)
	}
	master := requestAPI(t, http.MethodGet, strings.TrimSuffix(server.baseURL, api.Prefix)+playing.OnLadder.URL,
		token, nil)
	if master.status != http.StatusOK || !strings.Contains(master.body, "q1600k600p/stream.m3u8") ||
		strings.Contains(master.body, "q2000k540p") {
		t.Errorf("the master playlist = %d:\n%s\nwant chunkQualitiesHardware's 1600k@600p alone below the film's own",
			master.status, master.body)
	}
}

// cutFilmPlaying serves a film by served, waits until the sweep read it through for its keyframes,
// and starts its playback, which is on its ladder at the top with no link remembered (D260). The
// film is thirty seconds of 2.4 Mb/s with a keyframe every ten.
func cutFilmPlaying(
	t *testing.T, served func(t *testing.T, shelved filmFixture) (runningServer, string, int64),
) ladderFilm {
	t.Helper()
	const probe = `{"format":{"format_name":"matroska","duration":"30.000"},"streams":[` +
		`{"index":0,"codec_type":"video","codec_name":"h264","profile":"High","pix_fmt":"yuv420p",` +
		`"width":1920,"height":1080},` +
		`{"index":1,"codec_type":"audio","codec_name":"aac","profile":"LC","channels":2}]}`
	packets := "#tb 0: 1/1000\n"
	for second := range 30 {
		flags := ", F=0x0"
		if second%10 == 0 {
			flags = ""
		}
		packets += fmt.Sprintf("0, %d, %d, 1000, 300000, 0x1%s\n", second*1000, second*1000, flags)
	}
	measured := testutil.EBUR128Summary("stereo_1", "-23.0", "8.0", "-3.0")
	shelved := filmAnswered(t, "nfo", testutil.FakeReply{Stdout: probe, FFmpegStdout: packets, Stderr: measured})
	server, token, filmFileID := served(t, shelved)
	database := openDatabase(t, shelved.config)
	film, err := database.Read().GetPlayableFile(t.Context(), filmFileID)
	if err != nil {
		t.Fatal(err)
	}
	pollUntil(t, "the film's timeline was kept", func() bool {
		_, timelineErr := database.Read().GetVideoTimeline(t.Context(),
			sqlcgen.GetVideoTimelineParams{PayloadHash: film.PayloadHash, VideoStreamIndex: 0})
		return timelineErr == nil
	})
	decided := requestAPI(t, http.MethodPost, server.baseURL+"/playback", token, map[string]any{
		"fileId": filmFileID, "playableFormats": map[string]any{
			"videoCodecs": []string{"h264-sdr"}, "audioCodecs": []string{"aac-stereo"}, "subtitleFormats": []string{},
			"containers": []string{"mkv"},
		},
	})
	var playing ladderPlaying
	if err := json.Unmarshal([]byte(decided.body), &playing); err != nil || playing.MediaToken == "" ||
		playing.OnLadder == nil || playing.OnLadder.QualityKbps != 2400 ||
		!strings.HasSuffix(playing.OnLadder.URL, "/bytes/media/chunks/"+playing.MediaToken+"/master.m3u8") {
		t.Fatalf("POST /playback = %d %s; want it on its ladder at the film's own 2400k, by the playback",
			decided.status, decided.body)
	}
	return ladderFilm{server: server, token: token, playing: playing}
}
