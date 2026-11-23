// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A key frame is one picture of the film it is given and nothing else, by the file protocol alone:
// the input sought to its place, keyframes alone decoded and the first kept, not decoded on to the
// place, so a single frame is decoded; of the picture stream named, 640 across at its pixels' shape,
// one WebP with none of the video's metadata (D545).
func TestKeyFrame_FFmpegDecodesOnlyTheKeyframeAtOrBeforeThePlaceAt640Across(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: "a webp"})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := NewDeriver(runner, time.Minute).KeyFrame(t.Context(),
		execx.Film{Path: "/media/film.mkv", Size: 1 << 30}, 2, 754*time.Second+250*time.Millisecond)
	if err != nil || string(frame) != "a webp" {
		t.Fatalf("frame %q, %v; want FFmpeg's output", frame, err)
	}
	arguments := fake.OnlyCall(t).Arguments
	for flag, want := range map[string]string{
		"-protocol_whitelist": "file", "-i": "file:/media/film.mkv", "-ss": "754.250", "-skip_frame": "nokey",
		"-map": "0:2", "-fps_mode": "passthrough", "-frames:v": "1", "-map_metadata": "-1", "-c:v": "libwebp",
		"-f": "webp",
	} {
		if got := argumentAfter(arguments, flag); got != want {
			t.Errorf("FFmpeg ran with %s %q, want %q: the film by the file protocol alone, sought to its "+
				"place, keyframes alone, one WebP of stream 2 with no metadata (%q)", flag, got, want, arguments)
		}
	}
	input := slices.Index(arguments, "-i")
	for _, flag := range []string{"-ss", "-skip_frame", "-noaccurate_seek"} {
		if at := slices.Index(arguments, flag); at < 0 || at > input {
			t.Errorf("FFmpeg ran with %q; want %s the input's, before it: the seek lands on the keyframe "+
				"at or before the place and nothing after it is decoded", arguments, flag)
		}
	}
	if slices.Contains(arguments, "-t") {
		t.Errorf("FFmpeg ran with %q; want no stretch read: one frame ends the run", arguments)
	}
	filters := argumentAfter(arguments, "-vf")
	if !strings.HasPrefix(filters, squarePixels+",") || !strings.HasSuffix(filters, "scale=640:-2:flags=lanczos") {
		t.Errorf("FFmpeg filtered with %q; want the pixels squared, then 640 across, its height even", filters)
	}
}

// A film with no picture at or before the place writes nothing, which is its answer; a run that fails
// is its failure.
func TestKeyFrame_NothingWrittenIsNoKeyFrameAndAFailureIsItsOwn(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewDeriver(runner, time.Minute).KeyFrame(t.Context(), execx.Film{Path: "/media/black.mkv"}, 0, time.Hour)
	if !errors.Is(err, ErrNoKeyFrame) {
		t.Errorf("a run that wrote nothing: %v; want ErrNoKeyFrame", err)
	}
	failing := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{ExitStatus: 1, Stderr: "Invalid data"})
	if runner, err = execx.NewRunner(failing.Directory); err != nil {
		t.Fatal(err)
	}
	_, err = NewDeriver(runner, time.Minute).KeyFrame(t.Context(), execx.Film{Path: "/media/broken.mkv"}, 0, 0)
	if err == nil || errors.Is(err, ErrNoKeyFrame) {
		t.Errorf("a run that failed: %v; want its failure, not a film with no key frame", err)
	}
}
