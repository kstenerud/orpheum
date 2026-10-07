// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// TestPlayback_AStalledPlayerIsOfferedALowerQualityItsFilmIsCutInto plays a film end to end on a
// slow link: the sweep reads it through for its keyframes, and a first stall in a play report starts
// the quality its link carries and is answered with it, as is every report after it (D253 §1,
// D247 §10). The film is
// thirty seconds of 2.4 Mb/s with a keyframe every ten.
func TestPlayback_AStalledPlayerIsOfferedALowerQualityItsFilmIsCutInto(t *testing.T) {
	t.Parallel()
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
	server, token, filmFileID := playableFilm(t, shelved)
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
	var playing struct {
		MediaToken string `json:"mediaToken"`
	}
	if err := json.Unmarshal([]byte(decided.body), &playing); err != nil || playing.MediaToken == "" {
		t.Fatalf("POST /playback = %d %s", decided.status, decided.body)
	}
	report := func(seq int64) apiReply {
		return requestAPI(t, http.MethodPost, server.baseURL+"/playback/"+playing.MediaToken+"/report", token,
			map[string]any{
				"reportSeq": seq, "sentMonoMs": 1_000_000, "positionMs": 12_000, "positionReadMonoMs": 1_000_000,
				"events": []any{}, "spans": []any{}, "linkKbps": 2000,
				"stalls": []any{map[string]any{"positionMs": 12_000}},
			})
	}
	var answer struct {
		LowerQuality *struct {
			URL         string `json:"url"`
			QualityKbps int    `json:"qualityKbps"`
			IsReady     bool   `json:"isReady"`
		} `json:"lowerQuality"`
	}
	first := report(1)
	if err := json.Unmarshal([]byte(first.body), &answer); err != nil || answer.LowerQuality == nil ||
		answer.LowerQuality.QualityKbps != 1200 ||
		!strings.HasSuffix(answer.LowerQuality.URL, "/bytes/media/chunks/"+playing.MediaToken+"/master.m3u8") {
		t.Errorf("the first stall's report = %d %s; want 1200k offered, its master playlist by the playback",
			first.status, first.body)
	}
	// A report after it is answered with the same offer, so the player learns when it is ready
	// (D253 §1).
	answer.LowerQuality = nil
	if again := report(2); json.Unmarshal([]byte(again.body), &answer) != nil || answer.LowerQuality == nil ||
		answer.LowerQuality.QualityKbps != 1200 {
		t.Errorf("the report after the stall = %d %s; want the 1200k offer again", again.status, again.body)
	}
}
