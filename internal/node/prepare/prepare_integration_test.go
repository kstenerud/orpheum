// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package prepare_test

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

const sampleRate = 48000

// surroundFilm makes an eight-second MKV whose 5.1 sound has 300 Hz in the fronts and 1 kHz,
// standing for dialogue, alone in the centre.
func surroundFilm(t *testing.T, runner *execx.Runner) string {
	t.Helper()
	film := filepath.Join(t.TempDir(), "a film (2020).mkv")
	_, err := runner.Run(t.Context(), execx.FFmpeg, confine.Access{}, []string{
		"-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration=8",
		"-f", "lavfi", "-i",
		"aevalsrc=exprs=0.3*sin(2*PI*300*t)|0.3*sin(2*PI*300*t)|0.3*sin(2*PI*1000*t)|0|0|0:" +
			"channel_layout=5.1:sample_rate=48000:duration=8",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "48", "-c:a", "ac3", "-b:a", "384k",
		"file:" + film,
	}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return film
}

// strength is the amplitude of frequency in samples, by Goertzel's algorithm.
func strength(samples []float64, frequency float64) float64 {
	coefficient := 2 * math.Cos(2*math.Pi*frequency/sampleRate)
	var previous, before float64
	for _, sample := range samples {
		previous, before = sample+coefficient*previous-before, previous
	}
	return math.Sqrt(previous*previous+before*before-coefficient*previous*before) / float64(len(samples))
}

// leftChannel decodes a rendition's left channel.
func leftChannel(t *testing.T, runner *execx.Runner, playlist string) []float64 {
	t.Helper()
	raw, err := runner.Run(t.Context(), execx.FFmpeg, confine.Access{}, []string{
		"-v", "error", "-i", playlist, "-af", "pan=mono|c0=FL", "-f", "s16le", "-ar", "48000", "pipe:1",
	}, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]float64, len(raw)/2)
	for index := range samples {
		samples[index] = float64(int16(binary.LittleEndian.Uint16(raw[2*index:]))) //nolint:gosec // PCM
	}
	return samples
}

func TestPrepare_ARemuxIsHLSWithAStandardAndABoostedMixAndTheBoostRaisesTheCentre(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	film := surroundFilm(t, runner)
	directory := t.TempDir()
	preparer, err := prepare.New(runner, directory, roomyKeeper{}, prepare.EncoderX264)
	if err != nil {
		t.Fatal(err)
	}
	var preparedMs int64
	command := nodeproto.Prepare{
		Profile: nodeproto.ProfileRemux, VideoCodec: "h264", RenditionDir: "7/remux/0123456789abcdef",
		Audio: []nodeproto.PrepareAudio{{AudioTrackIndex: 0, Language: "en", ChannelCount: 6}},
	}
	made, err := preparer.Prepare(t.Context(), film, command, func(ms int64) { preparedMs = ms })
	if err != nil {
		t.Fatal(err)
	}
	rendition := filepath.Join(directory, "7", "remux", "0123456789abcdef")
	master, err := os.ReadFile(filepath.Join(rendition, "master.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`URI="audio0/stream.m3u8"`, `URI="audio0-boost/stream.m3u8"`, `LANGUAGE="en"`,
		"video/stream.m3u8", "avc1.",
	} {
		if !strings.Contains(string(master), want) {
			t.Errorf("master.m3u8 lacks %s:\n%s", want, master)
		}
	}
	if made.SizeBytes <= 0 || preparedMs < 7000 {
		t.Errorf("size %d and progress %d ms; want the rendition's bytes and progress to near 8 s",
			made.SizeBytes, preparedMs)
	}
	if made.RealtimeFactor <= 0 || made.VideoEncoder != "copy" {
		t.Errorf("the remux made %+v; want its video copied, at the speed the pinned FFmpeg said (D225 §3)", made)
	}

	// The boost raises the centre's share: 1 kHz against 300 Hz, standard against boosted.
	standard := leftChannel(t, runner, filepath.Join(rendition, "audio0", "stream.m3u8"))
	boosted := leftChannel(t, runner, filepath.Join(rendition, "audio0-boost", "stream.m3u8"))
	standardShare := strength(standard, 1000) / strength(standard, 300)
	boostedShare := strength(boosted, 1000) / strength(boosted, 300)
	t.Logf("centre share: standard %.3f, boosted %.3f", standardShare, boostedShare)
	if raised := boostedShare / standardShare; raised < 1.8 || raised > 2.2 {
		t.Errorf("the boost raised the centre's share by %.2f (%.3f to %.3f); want about 2 (6 dB)",
			raised, standardShare, boostedShare)
	}
}
