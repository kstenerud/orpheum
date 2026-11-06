// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package container

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The keyframes read from a file's index are the ones ffprobe shows, to the millisecond, for files
// the pinned FFmpeg and mkvmerge make: an MP4 with B-frames, whose keyframes are shown after they
// are decoded, under an edit, the same copied from 0.1 s in, its edit between two pictures, and
// Matroska from FFmpeg and from mkvmerge (D259 §2).
func TestKeyframesMs_AreTheKeyframesFFprobeShowsInRealFiles(t *testing.T) {
	t.Parallel()
	tools := testutil.RequireMKVToolNix(t)
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	inTest := func(name string) string { return filepath.Join(directory, name) }
	run := func(tool execx.Tool, arguments ...string) string {
		t.Helper()
		quiet := append([]string{"-v", "error"}, arguments...)
		out, runErr := runner.Run(t.Context(), tool, confine.Access{}, quiet, 1<<20)
		if runErr != nil {
			t.Fatal(runErr)
		}
		return string(out)
	}
	testutil.GenerateMedia(t, inTest("film.mp4"), "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=12",
		"-c:v", "libx264", "-preset", "ultrafast", "-bf", "2", "-g", "48", "-keyint_min", "48")
	testutil.GenerateMedia(t, inTest("ffmpeg.mkv"), "-i", "file:"+inTest("film.mp4"), "-c", "copy")
	// Copied from 0.1 s in, its edit starts the media between two pictures, after the first keyframe.
	testutil.GenerateMedia(t, inTest("cut.mp4"), "-ss", "0.1", "-i", "file:"+inTest("film.mp4"), "-c", "copy")
	tools.Run(t, "mkvmerge", "-q", "-o", inTest("mkvmerge.mkv"), inTest("film.mp4"))
	for _, name := range []string{"film.mp4", "cut.mp4", "ffmpeg.mkv", "mkvmerge.mkv"} {
		shown := run(execx.FFprobe, "-select_streams", "v:0", "-skip_frame", "nokey", "-show_entries", "frame=pts_time",
			"-of", "csv=p=0", "file:"+inTest(name))
		var want []int64
		for line := range strings.FieldsSeq(shown) {
			seconds, parseErr := strconv.ParseFloat(strings.TrimSuffix(line, ","), 64)
			if parseErr != nil {
				t.Fatalf("%s: ffprobe said %q", name, line)
			}
			want = append(want, int64(math.Floor(seconds*1000+0.5)))
		}
		if name == "cut.mp4" && len(want) > 0 && want[0] > 0 {
			// ffprobe leaves out the keyframe before the edit's start, which it discards; the index
			// counts it at the start (StartingKeyframes).
			want = append([]int64{0}, want...)
		}
		got := keyframesOfFile(t, inTest(name))
		if len(got) != len(want) || len(want) < 5 {
			t.Errorf("%s: read %v from the index; ffprobe shows %v", name, got, want)
			continue
		}
		for index := range want {
			if math.Abs(float64(got[index]-want[index])) > 1 {
				t.Errorf("%s: read %v from the index; ffprobe shows %v", name, got, want)
				break
			}
		}
	}
}

// keyframesOfFile are the keyframes of the picture, stream 0, of the file at path.
func keyframesOfFile(t *testing.T, path string) []int64 {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	keyframes, err := KeyframesMs(file, info.Size(), 0)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return keyframes
}
