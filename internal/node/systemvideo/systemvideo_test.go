// SPDX-License-Identifier: AGPL-3.0-or-later

package systemvideo

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

var soundTiming = nodeproto.SystemVideo{Purpose: nodeproto.SystemVideoSoundTiming, Codec: nodeproto.SystemVideoH264AAC}

// newMaker is a maker over the fake FFmpeg, which writes "clip" to the file it is asked to make.
func newMaker(t *testing.T) (*Maker, testutil.FakeFFmpeg) {
	t.Helper()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{FFmpegOutput: "clip"})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	return NewMaker(runner, t.TempDir()), fake
}

func TestMake_FFmpegDrawsTheClipIntoTheSystemFolderAlone(t *testing.T) {
	t.Parallel()
	maker, fake := newMaker(t)
	made := maker.Make(t.Context(), nodeproto.MakeSystemVideos{Videos: []nodeproto.SystemVideo{soundTiming}})
	if made.Failure != "" || made.Folder != maker.Folder() ||
		!slices.Equal(made.FileNames, []string{"sound-timing.h264-aac.mp4"}) {
		t.Fatalf("made %+v; want the sound timing clip in the folder", made)
	}
	arguments := fake.OnlyCall(t).Arguments
	want := "file:" + filepath.Join(maker.Folder(), "sound-timing.h264-aac.mp4")
	if arguments[len(arguments)-1] != want {
		t.Errorf("FFmpeg wrote %q; want %q", arguments[len(arguments)-1], want)
	}
	checkReadsNothing(t, arguments)
	for _, beep := range []string{"440", "554", "659", "880"} {
		if !slices.ContainsFunc(arguments, func(argument string) bool { return strings.Contains(argument, beep) }) {
			t.Errorf("the clip's sound has no %s Hz beep: %q", beep, arguments)
		}
	}
	contents, err := os.ReadFile(filepath.Join(maker.Folder(), "sound-timing.h264-aac.mp4"))
	if err != nil || string(contents) != "clip" {
		t.Errorf("the folder's clip = %q, %v; want what FFmpeg wrote", contents, err)
	}
}

// checkReadsNothing fails unless each of FFmpeg's inputs is a source lavfi generates.
func checkReadsNothing(t *testing.T, arguments []string) {
	t.Helper()
	for index, argument := range arguments {
		if argument != "-i" {
			continue
		}
		if index == 0 || arguments[index-1] != "lavfi" || index+1 >= len(arguments) {
			t.Errorf("an input of %q is not lavfi's; want FFmpeg to read nothing", arguments)
			continue
		}
		source := arguments[index+1]
		if !strings.HasPrefix(source, "color=") && !strings.HasPrefix(source, "aevalsrc=") {
			t.Errorf("FFmpeg's input %q is not a generated source", source)
		}
	}
}

func TestMake_AVideoNotKnownIsRefusedWithoutARun(t *testing.T) {
	t.Parallel()
	maker, fake := newMaker(t)
	made := maker.Make(t.Context(), nodeproto.MakeSystemVideos{Videos: []nodeproto.SystemVideo{
		{Purpose: nodeproto.SystemVideoSoundTiming, Codec: "vp9-opus"},
	}})
	if !strings.Contains(made.Failure, "no recipe") || len(made.FileNames) != 0 {
		t.Errorf("made %+v; want an unknown video refused", made)
	}
	if calls := fake.Calls(t); len(calls) != 0 {
		t.Errorf("FFmpeg ran %d times; want none for a video with no recipe", len(calls))
	}
}

func TestMake_AVideoThereIsKeptAndARebuildMakesItAgain(t *testing.T) {
	t.Parallel()
	maker, fake := newMaker(t)
	path := filepath.Join(maker.Folder(), soundTiming.FileName())
	if err := os.WriteFile(path, []byte("made before"), 0o600); err != nil {
		t.Fatal(err)
	}
	asked := nodeproto.MakeSystemVideos{Videos: []nodeproto.SystemVideo{soundTiming}}
	made := maker.Make(t.Context(), asked)
	if made.Failure != "" || len(made.FileNames) != 1 || len(fake.Calls(t)) != 0 {
		t.Fatalf("made %+v with %d runs; want the file there kept, unmade", made, len(fake.Calls(t)))
	}
	asked.IsRebuild = true
	if made = maker.Make(t.Context(), asked); made.Failure != "" || len(fake.Calls(t)) != 1 {
		t.Fatalf("a rebuild made %+v with %d runs; want it made again", made, len(fake.Calls(t)))
	}
	if contents, err := os.ReadFile(path); err != nil || string(contents) != "clip" {
		t.Errorf("after the rebuild the clip = %q, %v; want FFmpeg's new one", contents, err)
	}
}

func TestMake_AFailedRunSaysWhy(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stderr: "Unknown encoder 'libx264'\n", ExitStatus: 1})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	made := NewMaker(runner, t.TempDir()).Make(t.Context(),
		nodeproto.MakeSystemVideos{Videos: []nodeproto.SystemVideo{soundTiming}})
	if !strings.Contains(made.Failure, "libx264") || len(made.FileNames) != 0 {
		t.Errorf("made %+v; want FFmpeg's words, and no file", made)
	}
}
