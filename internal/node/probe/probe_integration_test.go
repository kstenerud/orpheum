// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package probe

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// TestProbe_AFileThePinnedFFmpegMade generates a file with the pinned ffmpeg and probes it with
// the pinned ffprobe: the arguments are ones this ffprobe accepts, and its output parses.
func TestProbe_AFileThePinnedFFmpegMade(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	film := filepath.Join(t.TempDir(), "film: a test (2020).mkv")
	testutil.GenerateMedia(t, film,
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=25:duration=1",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-ac", "2",
		"-metadata:s:a:0", "language=ger", "-metadata:s:a:0", "title=Deutsch")

	record, err := NewProber(runner, time.Minute).Probe(t.Context(), execx.Film{Path: film})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(record.Format.FormatNames, "matroska") || len(record.Streams) != 2 {
		t.Fatalf("record = %+v, want a Matroska file with two streams", record)
	}
	video, audio := record.Streams[0], record.Streams[1]
	if video.Codec != "h264" || video.Video.Width != 160 || video.Video.Height != 120 {
		t.Errorf("video = %+v", video)
	}
	if audio.Codec != "aac" || audio.ContainerLanguage != "ger" || audio.ContainerTitle != "Deutsch" ||
		audio.Audio.ChannelCount != 2 {
		t.Errorf("audio = %+v", audio)
	}
	if record.Format.DurationMs < 900 || record.Format.DurationMs > 1200 {
		t.Errorf("duration %d ms, want about a second", record.Format.DurationMs)
	}
}

// TestProbe_MarksAPictureWhoseBFramesCarryNoDisplayTimes probes, with the pinned ffprobe, clips made
// like file 5667: H.264 and HEVC with B-frames, then copied with each picture shown at its decode time.
// Those are marked for re-timing; the clips they were copied from, their B-frames timed, are not
// (design/plans/audio-switch-and-retiming-2026-11-01.md §2.4).
func TestProbe_MarksAPictureWhoseBFramesCarryNoDisplayTimes(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	prober := NewProber(runner, time.Minute)
	for _, codec := range []string{"h264", "hevc"} {
		original := filepath.Join(t.TempDir(), "a "+codec+" film (2020).mp4")
		encoder := []string{"-c:v", "libx264", "-preset", "ultrafast", "-bf", "3"}
		if codec == "hevc" {
			encoder = []string{"-c:v", "libx265", "-tag:v", "hev1", "-x265-params", "bframes=3:log-level=error"}
		}
		testutil.GenerateMedia(t, original, append([]string{
			"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration=4",
			"-f", "lavfi", "-i", "sine=frequency=440:duration=4", "-c:a", "aac",
		}, encoder...)...)
		flattened := filepath.Join(t.TempDir(), "a flattened "+codec+" film (2020).mp4")
		testutil.GenerateMedia(t, flattened, "-i", "file:"+original, "-map", "0", "-c", "copy",
			"-bsf:v", "setts=dts=DTS-STARTDTS:pts=DTS-STARTDTS")
		for path, want := range map[string]bool{original: false, flattened: true} {
			record, err := prober.Probe(t.Context(), execx.Film{Path: path})
			if err != nil {
				t.Fatal(err)
			}
			if len(record.Streams) != 2 || record.Streams[0].Codec != codec {
				t.Fatalf("%s: record = %+v, want its %s picture and its sound", filepath.Base(path), record, codec)
			}
			if got := record.Streams[0].Video.NeedsRetiming; got != want {
				t.Errorf("%s: the picture needs re-timing: %v, want %v", filepath.Base(path), got, want)
			}
		}
	}
}
