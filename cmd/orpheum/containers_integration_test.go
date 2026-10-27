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

// realMedia's films: RealVideo with its keyframes two seconds apart, as RealVideo's are sparse, and
// RealAudio 1.0 (14.4K), the RealMedia codecs the pinned FFmpeg can write (the owner, 2026-10-27).
func realMediaFilms() []containerFilm {
	everyAxis := []string{"container", "video_codec", "audio_codec"}
	return []containerFilm{
		{
			extension: "rm", picture: "320x240", sound: "8000", probedCodec: "rv20", profile: "h264",
			reasons:  everyAxis,
			encoding: []string{"-c:v", "rv20", "-g", "50", "-c:a", "real_144", "-ac", "1", "-f", "rm"},
		},
		{
			extension: "rmvb", picture: "176x144", sound: "8000", probedCodec: "rv10", profile: "h264",
			reasons:  everyAxis,
			encoding: []string{"-c:v", "rv10", "-g", "50", "-c:a", "real_144", "-ac", "1", "-f", "rm"},
		},
	}
}

// sweptFilms are the containers people kept films in over the decades that D360 added, each in
// codecs typical of it: Windows Media's, the disc and broadcast formats', the camcorders', the MP4
// and Matroska relatives' and the older and rarer ones'. A picture with no audio is an IVF's or a
// Y4M's, which hold none.
func sweptFilms() []containerFilm {
	everyAxis := []string{"container", "video_codec", "audio_codec"}
	containerAndPicture := []string{"container", "video_codec"}
	container := []string{"container"}
	mpeg2With := func(extension, audio, muxer string) containerFilm {
		return sweptFilm(extension, "mpeg2video", "h264", everyAxis,
			"-c:v", "mpeg2video", "-c:a", audio, "-f", muxer)
	}
	digitalVideo := sweptFilm("dv", "dvvideo", "h264", everyAxis,
		"-c:v", "dvvideo", "-pix_fmt", "yuv420p", "-c:a", "pcm_s16le", "-ac", "2", "-f", "dv")
	digitalVideo.picture = "720x576" // DV is PAL's or NTSC's picture, nothing else
	threeG2 := sweptFilm("3g2", "h263", "h264", containerAndPicture, "-c:v", "h263", "-c:a", "aac", "-f", "3g2")
	threeG2.picture = "176x144" // H.263's QCIF
	return []containerFilm{
		sweptFilm("asf", "wmv2", "h264", everyAxis, "-c:v", "wmv2", "-c:a", "wmav2", "-f", "asf"),
		sweptFilm("dvr-ms", "wmv1", "h264", everyAxis, "-c:v", "wmv1", "-c:a", "wmav1", "-f", "asf"),
		mpeg2With("wtv", "mp2", "wtv"),
		sweptFilm("ogm", "theora", "h264", everyAxis, "-c:v", "libtheora", "-c:a", "libvorbis", "-f", "ogg"),
		mpeg2With("mxf", "pcm_s16le", "mxf"),
		digitalVideo,
		sweptFilm("nut", "mpeg4", "h264", everyAxis, "-c:v", "mpeg4", "-c:a", "mp3", "-f", "nut"),
		sweptFilm("f4v", "h264", "remux", container, "-c:v", "libx264", "-c:a", "aac", "-f", "f4v"),
		threeG2,
		sweptFilm("mk3d", "h264", "", nil, "-c:v", "libx264", "-c:a", "aac", "-f", "matroska"),
		sweptFilm("mpe", "mpeg1video", "h264", everyAxis, "-c:v", "mpeg1video", "-c:a", "mp2", "-f", "mpeg"),
		mpeg2With("m2p", "mp2", "vob"),
		mpeg2With("vro", "ac3", "vob"),
		mpeg2With("evo", "ac3", "vob"),
		sweptFilm("qt", "h264", "remux", container, "-c:v", "libx264", "-c:a", "aac", "-f", "mov"),
		sweptFilm("ivf", "vp9", "h264", containerAndPicture, "-c:v", "libvpx-vp9", "-an", "-f", "ivf"),
		sweptFilm("y4m", "rawvideo", "h264", containerAndPicture, "-pix_fmt", "yuv420p", "-an", "-f", "yuv4mpegpipe"),
		sweptFilm("xvid", "mpeg4", "h264", everyAxis, "-c:v", "mpeg4", "-vtag", "XVID", "-c:a", "mp3", "-f", "avi"),
		// A keyframe a second, as broadcasts have them.
		sweptFilm("trp", "h264", "remux", container, "-c:v", "libx264", "-g", "25", "-c:a", "aac", "-f", "mpegts"),
		mpeg2With("tp", "mp2", "mpegts"),
		mpeg2With("m2t", "ac3", "mpegts"),
		mpeg2With("tod", "ac3", "mpegts"),
	}
}

// sweptFilm is a film of a small picture and a 48 kHz sound.
func sweptFilm(extension, probedCodec, profile string, reasons []string, encoding ...string) containerFilm {
	return containerFilm{
		extension: extension, picture: "160x120", sound: "48000", encoding: encoding,
		probedCodec: probedCodec, profile: profile, reasons: reasons,
	}
}

// writeContainerFilms writes each film, four seconds long, into a folder of its own under media, its
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
			"-f", "lavfi", "-i", "testsrc2=size=" + film.picture + ":rate=25:duration=4",
			"-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=%d:sample_rate=%s:duration=4", 300+20*index, film.sound),
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
	films := append(realMediaFilms(), sweptFilms()...)
	config, configPath := testConfig(t)
	media := t.TempDir()
	paths := writeContainerFilms(t, media, films)
	config.Node.RootPaths = []string{media}
	config.Node.FFmpegDir = ffmpegDir
	nodeID := enrolNodeWithLibrary(t, config, media,
		libraryShape{name: "Films", kind: "movies", metadataSource: "nfo"})
	captured := newCapturedLog("ready", filmIdentified)
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(captured.String())
		}
	})
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
