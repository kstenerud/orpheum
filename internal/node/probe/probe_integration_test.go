// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package probe

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
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
	_, err = runner.Run(t.Context(), execx.FFmpeg, confine.Access{}, []string{
		"-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=25:duration=1",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-ac", "2",
		"-metadata:s:a:0", "language=ger", "-metadata:s:a:0", "title=Deutsch",
		"file:" + film,
	}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}

	record, err := NewProber(runner, time.Minute).Probe(t.Context(), film)
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
