// SPDX-License-Identifier: AGPL-3.0-or-later

package prepare_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakeFFmpeg writes a segment where the command line's output goes and reports its progress.
type fakeFFmpeg struct {
	fail error
	// granted, when set, records what each run was granted.
	granted *[]confine.Access
	// linkTo, when set, is a file the rendition is also given a symbolic link to.
	linkTo string
	// watches, when set, records how each run was watched.
	watches *[]execx.Watch
}

func (fake fakeFFmpeg) RunWatched(_ context.Context, run execx.WatchedRun) (execx.Ran, error) {
	err := fake.run(run.Access, run.Arguments)
	if fake.watches != nil {
		*fake.watches = append(*fake.watches, run.Watch)
	}
	run.Watch.OnProgress(execx.Progress{Position: 1500 * time.Millisecond, FrameCount: 10, RealtimeFactor: 4.25})
	if err != nil {
		return execx.Ran{}, err
	}
	return execx.Ran{}, fake.fail
}

func (fake fakeFFmpeg) run(access confine.Access, arguments []string) error {
	if fake.granted != nil {
		*fake.granted = append(*fake.granted, access)
	}
	output := filepath.Dir(filepath.Dir(arguments[len(arguments)-1]))
	if err := os.MkdirAll(filepath.Join(output, "video"), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(output, "video", "seg00000.m4s"), []byte("12345"), 0o600); err != nil {
		return err
	}
	if fake.linkTo != "" {
		if err := os.Symlink(fake.linkTo, filepath.Join(output, "video", "link.m4s")); err != nil {
			return err
		}
	}
	return nil
}

// roomyKeeper is a prepared directory with room for every prepare.
type roomyKeeper struct{}

func (roomyKeeper) MakeRoom(context.Context) error            { return nil }
func (roomyKeeper) MarkWriting(string) (finishWriting func()) { return func() {} }

func (roomyKeeper) Shelve(nodeproto.RenditionManifest) error { return nil }

// shelvingKeeper is a prepared directory with room for every prepare, which records each manifest
// shelved and whether its rendition was still being written then.
type shelvingKeeper struct {
	isWriting    bool
	shelved      []nodeproto.RenditionManifest
	whileWriting []bool
}

func (*shelvingKeeper) MakeRoom(context.Context) error { return nil }

func (keeper *shelvingKeeper) MarkWriting(string) (finishWriting func()) {
	keeper.isWriting = true
	return func() { keeper.isWriting = false }
}

func (keeper *shelvingKeeper) Shelve(manifest nodeproto.RenditionManifest) error {
	keeper.shelved = append(keeper.shelved, manifest)
	keeper.whileWriting = append(keeper.whileWriting, keeper.isWriting)
	return nil
}

// fullKeeper is a prepared directory whose disk is past its limit with nothing to evict, and which
// records whether anything was written.
type fullKeeper struct{}

func (fullKeeper) MakeRoom(context.Context) error { return errors.New("the disk is past its limit") }

func (fullKeeper) MarkWriting(string) (finishWriting func()) {
	panic("nothing is written where there is no room")
}

func (fullKeeper) Shelve(nodeproto.RenditionManifest) error {
	panic("nothing is shelved where there is no room")
}

func TestPreparer_APrepareTheDiskHasNoRoomForIsRefusedAndRunsNothing(t *testing.T) {
	t.Parallel()
	preparer, err := prepare.New(
		fakeFFmpeg{fail: errors.New("FFmpeg ran")}, t.TempDir(), fullKeeper{}, prepare.EncoderX264)
	if err != nil {
		t.Fatal(err)
	}
	command := nodeproto.Prepare{Profile: nodeproto.ProfileRemux, RenditionDir: "3/remux/0123456789abcdef"}
	_, err = preparer.Prepare(t.Context(), "/media/a.mkv", command, func(int64) {})
	if err == nil || !strings.Contains(err.Error(), "past its limit") {
		t.Errorf("a prepare with no room: %v, want refused with why, before FFmpeg runs (D217 §5)", err)
	}
}

func TestPreparer_WritesUnderTheRenditionsDirectoryAndReportsProgressAndSize(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	preparer, err := prepare.New(fakeFFmpeg{}, directory, roomyKeeper{}, prepare.EncoderX264)
	if err != nil {
		t.Fatal(err)
	}
	var progress []int64
	command := nodeproto.Prepare{Profile: nodeproto.ProfileRemux, RenditionDir: "7/remux/0123456789abcdef"}
	made, err := preparer.Prepare(t.Context(), "/media/film.mkv", command, func(ms int64) {
		progress = append(progress, ms)
	})
	if err != nil {
		t.Fatal(err)
	}
	if made.SizeBytes != 5 || !slices.Equal(progress, []int64{1500}) {
		t.Errorf("size %d, progress %v; want 5 bytes and 1500 ms", made.SizeBytes, progress)
	}
	segment := filepath.Join(directory, "7", "remux", "0123456789abcdef", "video", "seg00000.m4s")
	if _, err := os.Stat(segment); err != nil {
		t.Errorf("the rendition is not under 7/remux: %v", err)
	}
}

// A made rendition's manifest is shelved beside it, while it is still being written, so no eviction
// takes it for a making that never finished (D238 §2).
func TestPreparer_ShelvesAMadeRenditionsManifestWhileItIsStillBeingWritten(t *testing.T) {
	t.Parallel()
	keeper := &shelvingKeeper{}
	preparer, err := prepare.New(fakeFFmpeg{}, t.TempDir(), keeper, prepare.EncoderX264)
	if err != nil {
		t.Fatal(err)
	}
	command := nodeproto.Prepare{Profile: nodeproto.ProfileRemux, RenditionDir: "7/remux/0123456789abcdef"}
	if _, err := preparer.Prepare(t.Context(), "/media/film.mkv", command, func(int64) {}); err != nil {
		t.Fatal(err)
	}
	if len(keeper.shelved) != 1 || keeper.shelved[0].Prepare.RenditionDir != command.RenditionDir ||
		keeper.shelved[0].SizeBytes != 5 || keeper.shelved[0].VideoEncoder != "copy" || !keeper.whileWriting[0] {
		t.Errorf("shelved %+v, while being written %v; want 7/remux's manifest, 5 bytes copied, while written",
			keeper.shelved, keeper.whileWriting)
	}
}

func TestPreparer_ARenditionsSizeIsItsRegularFilesAsThePreparedBudgetCountsThem(t *testing.T) {
	t.Parallel()
	large := filepath.Join(t.TempDir(), "large.mkv")
	if err := os.WriteFile(large, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	preparer, err := prepare.New(fakeFFmpeg{linkTo: large}, t.TempDir(), roomyKeeper{}, prepare.EncoderX264)
	if err != nil {
		t.Fatal(err)
	}
	command := nodeproto.Prepare{Profile: nodeproto.ProfileRemux, RenditionDir: "7/remux/0123456789abcdef"}
	made, err := preparer.Prepare(t.Context(), "/media/film.mkv", command, func(int64) {})
	if err != nil {
		t.Fatal(err)
	}
	if made.SizeBytes != 5 {
		t.Errorf("size %d; want 5, the one segment, and nothing for a link, as the prepared budget counts (D217 §5)",
			made.SizeBytes)
	}
}

func TestPreparer_SaysWhatMadeTheVideoAndHowFastFFmpegSaidItRan(t *testing.T) {
	t.Parallel()
	prepareWith := func(profile nodeproto.PrepareProfile) prepare.Made {
		preparer, err := prepare.New(fakeFFmpeg{}, t.TempDir(), roomyKeeper{}, prepare.EncoderQuickSync)
		if err != nil {
			t.Fatal(err)
		}
		command := nodeproto.Prepare{Profile: profile, RenditionDir: "7/" + string(profile) + "/0123456789abcdef"}
		made, err := preparer.Prepare(t.Context(), "/media/film.mkv", command, func(int64) {})
		if err != nil {
			t.Fatal(err)
		}
		return made
	}
	if made := prepareWith(nodeproto.ProfileH264); made.VideoEncoder != "h264_qsv" || made.RealtimeFactor != 4.25 {
		t.Errorf("an encode on a Quick Sync node made %+v; want h264_qsv at FFmpeg's last speed, 4.25 (D225 §3)", made)
	}
	if made := prepareWith(nodeproto.ProfileRemux); made.VideoEncoder != "copy" {
		t.Errorf("a remux made %+v; want its video copied", made)
	}
}

func TestPreparer_OnlyAnEncodeOnTheGPUIsGrantedItsDevices(t *testing.T) {
	t.Parallel()
	prepareWith := func(encoder prepare.Encoder, profile nodeproto.PrepareProfile) confine.Access {
		var granted []confine.Access
		preparer, err := prepare.New(fakeFFmpeg{granted: &granted}, t.TempDir(), roomyKeeper{}, encoder)
		if err != nil {
			t.Fatal(err)
		}
		command := nodeproto.Prepare{Profile: profile, RenditionDir: "7/" + string(profile) + "/0123456789abcdef"}
		if _, err := preparer.Prepare(t.Context(), "/media/film.mkv", command, func(int64) {}); err != nil {
			t.Fatal(err)
		}
		return granted[0]
	}
	if !prepareWith(prepare.EncoderQuickSync, nodeproto.ProfileH264).UsesGPU {
		t.Error("an encode with Quick Sync is not granted the GPU; confined, it would fail")
	}
	if prepareWith(prepare.EncoderQuickSync, nodeproto.ProfileRemux).UsesGPU {
		t.Error("a remux is granted the GPU, which it never uses")
	}
	if prepareWith(prepare.EncoderX264, nodeproto.ProfileH264).UsesGPU {
		t.Error("an x264 encode is granted the GPU, which it never uses")
	}
}

func TestPreparer_RefusesADirectoryThatIsNotARenditions(t *testing.T) {
	t.Parallel()
	preparer, err := prepare.New(fakeFFmpeg{}, t.TempDir(), roomyKeeper{}, prepare.EncoderX264)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{
		"../7/remux", "7/remux", "7/remux/../x", "7/other/0123456789abcdef", "0/remux/0123456789abcdef",
		"7/remux/0123456789ABCDEF",
	} {
		_, refused := preparer.Prepare(t.Context(), "/a", nodeproto.Prepare{RenditionDir: dir}, func(int64) {})
		if !errors.Is(refused, prepare.ErrBadRenditionDir) {
			t.Errorf("%q: %v, want ErrBadRenditionDir", dir, refused)
		}
	}
	failing, err := prepare.New(
		fakeFFmpeg{fail: errors.New("ffmpeg failed")}, t.TempDir(), roomyKeeper{}, prepare.EncoderX264)
	if err != nil {
		t.Fatal(err)
	}
	named := nodeproto.Prepare{RenditionDir: "7/remux/0123456789abcdef"}
	if _, err := failing.Prepare(t.Context(), "/a", named, func(int64) {}); err == nil {
		t.Error("a failed run reported success")
	}
	if _, err := prepare.New(fakeFFmpeg{}, "relative", roomyKeeper{}, prepare.EncoderX264); err == nil {
		t.Error("a relative prepared directory was taken")
	}
}

func TestPreparer_WatchesARunWithADeadlineFromTheSourcesLengthAndTheSpeedLastMeasuredHere(t *testing.T) {
	t.Parallel()
	var watches []execx.Watch
	preparer, err := prepare.New(fakeFFmpeg{watches: &watches}, t.TempDir(), roomyKeeper{}, prepare.EncoderX264)
	if err != nil {
		t.Fatal(err)
	}
	prepareFilm := func(profile nodeproto.PrepareProfile, sourceDurationMs int64) {
		command := nodeproto.Prepare{
			Profile: profile, RenditionDir: "7/" + string(profile) + "/0123456789abcdef",
			SourceDurationMs: sourceDurationMs,
		}
		if _, err := preparer.Prepare(t.Context(), "/media/film.mkv", command, func(int64) {}); err != nil {
			t.Fatal(err)
		}
	}
	const twoHoursMs = 2 * 60 * 60 * 1000
	prepareFilm(nodeproto.ProfileH264, twoHoursMs)
	prepareFilm(nodeproto.ProfileH264, twoHoursMs)
	prepareFilm(nodeproto.ProfileRemux, twoHoursMs)
	prepareFilm(nodeproto.ProfileH264, 0)
	// Three times the expected length: two hours at half of real time before a speed is measured,
	// then at the 4.25 times the fake's FFmpeg said; per profile; a day when the length is unknown.
	speed, lengthMs := 4.25, float64(twoHoursMs)
	measured := time.Duration(lengthMs/speed) * time.Millisecond
	wantDeadlines := []time.Duration{12 * time.Hour, 3 * measured, 12 * time.Hour, 24 * time.Hour}
	for index, want := range wantDeadlines {
		if got := watches[index].Deadline; got != want {
			t.Errorf("prepare %d's deadline is %s, want %s", index, got, want)
		}
	}
	if watches[0].StallWindow != 3*time.Minute || watches[0].StartGrace != 2*time.Minute {
		t.Errorf("a prepare is watched %+v; want three minutes standing still after two to start", watches[0])
	}
}
