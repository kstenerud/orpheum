// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// containerFilm is a film of one container a scan reads (D360), as the pinned FFmpeg writes it: its
// extension, the arguments that encode its picture and sound into it, the codec its probe names
// first, and how a browser playing H.264 and stereo AAC in Matroska plays it.
type containerFilm struct {
	extension string
	// picture and sound are the size and the sample rate of its source, the most each container takes.
	picture, sound string
	encoding       []string
	probedCodec    string
	profile        string // "" plays it as it is
	reasons        []string
}

// realMedia's films: RealVideo with its keyframes four seconds apart, as RealVideo's are sparse, and
// RealAudio 1.0 (14.4K), the RealMedia codecs the pinned FFmpeg can write (the owner, 2026-10-27).
func realMediaFilms() []containerFilm {
	everyAxis := []string{"container", "video_codec", "audio_codec"}
	return []containerFilm{
		{
			extension: "rm", picture: "320x240", sound: "8000", probedCodec: "rv20", profile: "h264",
			reasons:  everyAxis,
			encoding: []string{"-c:v", "rv20", "-g", "100", "-c:a", "real_144", "-ac", "1", "-f", "rm"},
		},
		{
			extension: "rmvb", picture: "176x144", sound: "8000", probedCodec: "rv10", profile: "h264",
			reasons:  everyAxis,
			encoding: []string{"-c:v", "rv10", "-g", "100", "-c:a", "real_144", "-ac", "1", "-f", "rm"},
		},
	}
}

// writeContainerFilms writes each film, eight seconds long, into a folder of its own under media, its
// tone a pitch of its own so no two films' payloads are the same, and returns each one's path.
func writeContainerFilms(t *testing.T, media string, films []containerFilm) []string {
	t.Helper()
	paths := make([]string, len(films))
	for index, film := range films {
		name := fmt.Sprintf("Film %s (2020)", film.extension)
		paths[index] = filepath.Join(media, name, name+"."+film.extension)
		if err := os.MkdirAll(filepath.Dir(paths[index]), 0o700); err != nil {
			t.Fatal(err)
		}
		testutil.GenerateFilm(t, paths[index], append([]string{
			"-f", "lavfi", "-i", "testsrc2=size=" + film.picture + ":rate=25:duration=8",
			"-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=%d:sample_rate=%s:duration=8", 300+20*index, film.sound),
		}, film.encoding...)...)
	}
	return paths
}

// Each container a scan reads that D360 added plays: a film the pinned FFmpeg writes in it is read
// by a scan as a video, probed by the node's FFmpeg, decided as its codecs need for a browser
// playing H.264 and stereo AAC in Matroska, and the first chunk of its ladder made, the ladder asked
// for when it plays as it is.
func TestPlayback_EachContainerAddedIsReadProbedDecidedAndItsFirstChunkMade(t *testing.T) {
	ffmpegDir := testutil.FFmpegDirectory(t)
	films := realMediaFilms()
	config, configPath := testConfig(t)
	media := t.TempDir()
	paths := writeContainerFilms(t, media, films)
	config.Node.RootPaths = []string{media}
	config.Node.FFmpegDir = ffmpegDir
	nodeID := enrolNodeWithLibrary(t, config, media,
		libraryShape{name: "Films", kind: "movies", metadataSource: "nfo"})
	captured := newCapturedLog("ready", filmIdentified)
	server := startServing(t, config, captured, filmIdentified, func(ctx context.Context) error {
		return runNodeWith(ctx, config, captured.logger, nodeSeams{statusEvery: time.Second})
	})
	token := signedInOwner(t, server, configPath)
	database := openDatabase(t, config)
	files, err := database.Read().ListLiveFilesByNode(t.Context(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	fileIDs, filings := map[string]int64{}, map[string]string{}
	for _, file := range files {
		filings[filepath.Base(file.Path)] = file.Filing
		// Filed as a scan files a video it read; no film has an NFO to be identified by.
		if slices.Contains([]string{"listed", "unidentified"}, file.Filing) {
			fileIDs[file.Path] = file.ID
		}
	}
	for index, film := range films {
		fileID, isRead := fileIDs[paths[index]]
		if !isRead {
			t.Errorf(".%s: the live files' filings are %v; want it read as a video, not skipped",
				film.extension, filings)
			continue
		}
		pollUntil(t, "."+film.extension+" probed", func() bool {
			stored, playableErr := database.Read().GetPlayableFile(t.Context(), fileID)
			return playableErr == nil && stored.ProbeJSON != nil &&
				strings.Contains(*stored.ProbeJSON, `"`+film.probedCodec+`"`)
		})
		checkPlaysOnItsLadder(t, server, token, fileID, film)
	}
}

// checkPlaysOnItsLadder checks a film is decided as its codecs need, and the first chunk of its
// ladder is made, asking for the ladder when it plays as it is.
func checkPlaysOnItsLadder(t *testing.T, server runningServer, token string, fileID int64, film containerFilm) {
	t.Helper()
	type decision struct {
		Delivery         string   `json:"delivery"`
		Profile          string   `json:"profile"`
		NotDirectReasons []string `json:"notDirectReasons"`
		OnLadder         *struct {
			URL string `json:"url"`
		} `json:"onLadder"`
	}
	decide := func(asked map[string]any) (decision, apiReply) {
		var reply apiReply
		pollUntil(t, "."+film.extension+" read through and decided", func() bool {
			reply = requestAPI(t, http.MethodPost, server.baseURL+"/playback", token, asked)
			return reply.status != http.StatusServiceUnavailable
		})
		var decided decision
		if err := json.Unmarshal([]byte(reply.body), &decided); err != nil {
			t.Fatalf(".%s: POST /playback = %d %s", film.extension, reply.status, reply.body)
		}
		return decided, reply
	}
	asked := chromeMKV(fileID)
	decided, reply := decide(asked)
	wantDelivery := "hls"
	if film.profile == "" {
		wantDelivery = "direct"
	}
	if decided.Delivery != wantDelivery || decided.Profile != film.profile ||
		!slices.Equal(decided.NotDirectReasons, film.reasons) {
		t.Errorf(".%s: POST /playback = %d %s; want %s, %q, for %v", film.extension, reply.status, reply.body,
			wantDelivery, film.profile, film.reasons)
		return
	}
	if decided.Delivery == "direct" {
		asked["preferredDelivery"] = "hls"
		decided, reply = decide(asked)
	}
	if decided.OnLadder == nil {
		t.Errorf(".%s: POST /playback = %d %s; want it on its ladder", film.extension, reply.status, reply.body)
		return
	}
	root := strings.TrimSuffix(server.baseURL, api.Prefix)
	master := requestAPI(t, http.MethodGet, root+decided.OnLadder.URL, token, nil)
	quality := firstURI(master.body)
	if master.status != http.StatusOK || quality == "" {
		t.Errorf(".%s: the master playlist = %d:\n%s\nwant a picture", film.extension, master.status, master.body)
		return
	}
	base := root + decided.OnLadder.URL[:strings.LastIndex(decided.OnLadder.URL, "/")+1]
	listed := requestAPI(t, http.MethodGet, base+quality, token, nil)
	chunk := firstURI(listed.body)
	if listed.status != http.StatusOK || chunk == "" {
		t.Errorf(".%s: the playlist %s = %d:\n%s\nwant a chunk", film.extension, quality, listed.status, listed.body)
		return
	}
	chunkURL := base + quality[:strings.LastIndex(quality, "/")+1] + chunk
	var made apiReply
	pollUntil(t, "."+film.extension+"'s first chunk made", func() bool {
		made = requestAPI(t, http.MethodGet, chunkURL, token, nil)
		return made.status != http.StatusTooEarly
	})
	if made.status != http.StatusOK || made.body == "" {
		t.Errorf(".%s: the first chunk %s = %d, %d bytes; want it made", film.extension, chunkURL, made.status,
			len(made.body))
	}
}

// firstURI is a playlist's first line that is neither a tag nor blank, or "".
func firstURI(playlist string) string {
	for line := range strings.Lines(playlist) {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}
