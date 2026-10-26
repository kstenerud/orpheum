// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package subtitle_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/subtitle"
	"github.com/kstenerud/orpheum/internal/testutil"
)

const srt = "1\n00:00:01,000 --> 00:00:02,500\nHello <i>there</i>\n\n2\n00:00:03,000 --> 00:00:04,000\nSecond line\n"

// checkText fails unless text carries both cues as ASS and as WebVTT.
func checkText(t *testing.T, what string, text subtitle.Text) {
	t.Helper()
	ass, vtt := string(text.ASS), string(text.VTT)
	if !strings.Contains(ass, "[Events]") ||
		!strings.Contains(ass, `Dialogue: 0,0:00:01.00,0:00:02.50,Default,,0,0,0,,Hello {\i1}there{\i0}`) ||
		!strings.Contains(ass, "0:00:03.00,0:00:04.00") {
		t.Errorf("%s: the ASS is\n%s", what, ass)
	}
	if !strings.HasPrefix(vtt, "WEBVTT") || !strings.Contains(vtt, "00:01.000 --> 00:02.500\nHello <i>there</i>") ||
		!strings.Contains(vtt, "Second line") {
		t.Errorf("%s: the WebVTT is\n%s", what, vtt)
	}
}

func TestExtract_ATrackAndASidecarBecomeASSAndWebVTT(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	sidecar := filepath.Join(directory, "film.en.srt")
	if writeErr := os.WriteFile(sidecar, []byte(srt), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	film := filepath.Join(directory, "film.mkv")
	_, err = runner.Run(t.Context(), execx.FFmpeg, confine.Access{}, []string{
		"-v", "error", "-f", "lavfi", "-i", "color=c=black:s=64x64:d=5", "-i", "file:" + sidecar,
		"-map", "0", "-map", "1", "-map", "1", "-c:v", "libx264", "-preset", "ultrafast", "-c:s:0", "ass",
		"-c:s:1", "subrip", "file:" + film,
	}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	extractor := subtitle.NewExtractor(runner, 20<<20)
	// Its two tracks, an ASS and a SubRip, from one read of it (D340).
	tracks, err := extractor.ExtractTracks(t.Context(), execx.Film{Path: film}, []int{1, 2})
	if err != nil || len(tracks) != 2 {
		t.Fatalf("the embedded tracks: %+v, %v", tracks, err)
	}
	for _, track := range tracks {
		if track.Err != nil {
			t.Errorf("the embedded track %d: %v", track.StreamIndex, track.Err)
		}
		checkText(t, "embedded track "+strconv.Itoa(track.StreamIndex), track.Text)
	}
	converted, err := extractor.ConvertText(t.Context(), []byte(srt))
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "the sidecar", converted)
	if _, err := extractor.ConvertText(t.Context(), []byte("not a subtitle\n")); err == nil {
		t.Error("a file that is not a subtitle converted")
	}
}
