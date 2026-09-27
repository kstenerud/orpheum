// SPDX-License-Identifier: GPL-3.0-or-later

package prepare_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakeFFmpeg writes a segment where the command line's output goes and reports its progress.
type fakeFFmpeg struct {
	fail error
}

func (fake fakeFFmpeg) RunStreaming(
	_ context.Context, _ execx.Tool, _ confine.Access, arguments []string, stdout io.Writer,
) error {
	output := filepath.Dir(filepath.Dir(arguments[len(arguments)-1]))
	if err := os.MkdirAll(filepath.Join(output, "video"), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(output, "video", "seg00000.m4s"), []byte("12345"), 0o600); err != nil {
		return err
	}
	if _, err := io.WriteString(stdout, "frame=10\nout_time_us=1500000\nprogress=continue\n"); err != nil {
		return err
	}
	return fake.fail
}

// roomy is a prepared directory with room for every prepare.
type roomy struct{}

func (roomy) Room() error                  { return nil }
func (roomy) Writing(string) (done func()) { return func() {} }

// full is a prepared directory whose disk is past its limit with nothing to evict, and which
// records whether anything was written.
type full struct{}

func (full) Room() error                  { return errors.New("the disk is past its limit") }
func (full) Writing(string) (done func()) { panic("nothing is written where there is no room") }

func TestPreparer_APrepareTheDiskHasNoRoomForIsRefusedAndRunsNothing(t *testing.T) {
	t.Parallel()
	preparer, err := prepare.New(fakeFFmpeg{fail: errors.New("FFmpeg ran")}, t.TempDir(), full{}, prepare.EncoderX264)
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
	preparer, err := prepare.New(fakeFFmpeg{}, directory, roomy{}, prepare.EncoderX264)
	if err != nil {
		t.Fatal(err)
	}
	var progress []int64
	command := nodeproto.Prepare{Profile: nodeproto.ProfileRemux, RenditionDir: "7/remux/0123456789abcdef"}
	size, err := preparer.Prepare(t.Context(), "/media/film.mkv", command, func(ms int64) {
		progress = append(progress, ms)
	})
	if err != nil {
		t.Fatal(err)
	}
	if size != 5 || !slices.Equal(progress, []int64{1500}) {
		t.Errorf("size %d, progress %v; want 5 bytes and 1500 ms", size, progress)
	}
	segment := filepath.Join(directory, "7", "remux", "0123456789abcdef", "video", "seg00000.m4s")
	if _, err := os.Stat(segment); err != nil {
		t.Errorf("the rendition is not under 7/remux: %v", err)
	}
}

func TestPreparer_RefusesADirectoryThatIsNotARenditions(t *testing.T) {
	t.Parallel()
	preparer, err := prepare.New(fakeFFmpeg{}, t.TempDir(), roomy{}, prepare.EncoderX264)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{
		"../7/remux", "7/remux", "7/remux/../x", "7/other/0123456789abcdef", "0/remux/0123456789abcdef",
		"7/remux/0123456789ABCDEF",
	} {
		_, refused := preparer.Prepare(t.Context(), "/a", nodeproto.Prepare{RenditionDir: dir}, func(int64) {})
		if !errors.Is(refused, prepare.ErrBadDir) {
			t.Errorf("%q: %v, want ErrBadDir", dir, refused)
		}
	}
	failing, err := prepare.New(
		fakeFFmpeg{fail: errors.New("ffmpeg failed")}, t.TempDir(), roomy{}, prepare.EncoderX264)
	if err != nil {
		t.Fatal(err)
	}
	named := nodeproto.Prepare{RenditionDir: "7/remux/0123456789abcdef"}
	if _, err := failing.Prepare(t.Context(), "/a", named, func(int64) {}); err == nil {
		t.Error("a failed run reported success")
	}
	if _, err := prepare.New(fakeFFmpeg{}, "relative", roomy{}, prepare.EncoderX264); err == nil {
		t.Error("a relative prepared directory was taken")
	}
}
