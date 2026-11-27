// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// lastOf is the last chunk a run made and its failure, without where it was decoded.
func lastOf(last int, _ encode.Decoding, err error) (int, error) { return last, err }

// gpuFailingFFmpeg is FFmpeg on a card that cannot decode the film: a run decoding on the GPU fails
// as FFmpeg does, after making the chunks of madeOnGPU; a run decoding on the CPU is fake's. It keeps
// every run's arguments.
type gpuFailingFFmpeg struct {
	fake      *fakeFFmpeg
	madeOnGPU *fakeFFmpeg
	runs      [][]string
}

func (ffmpeg *gpuFailingFFmpeg) RunWatched(ctx context.Context, run execx.WatchedRun) (execx.Ran, error) {
	ffmpeg.runs = append(ffmpeg.runs, run.Arguments)
	if !slices.Contains(run.Arguments, "-hwaccel") {
		return ffmpeg.fake.RunWatched(ctx, run)
	}
	failure := &execx.RunError{
		Tool: execx.FFmpeg, ExitStatus: 1, Err: errors.New("exit status 1"),
		StderrTail: "Impossible to convert between the formats supported by the filter 'Parsed_setpts_0' " +
			"and the filter 'auto_scale_0'",
	}
	if ffmpeg.madeOnGPU != nil {
		ffmpeg.madeOnGPU.err = failure
		return ffmpeg.madeOnGPU.RunWatched(ctx, run)
	}
	return execx.Ran{}, failure
}

// vaapiMakerOver is a maker encoding on VAAPI into a fresh prepared directory, with FFmpeg faked.
func vaapiMakerOver(t *testing.T, runner Runner) *Maker {
	t.Helper()
	return vaapiMakerLoggingTo(t, runner, slog.New(slog.DiscardHandler))
}

// vaapiMakerLoggingTo is vaapiMakerOver's maker, logging to logger.
func vaapiMakerLoggingTo(t *testing.T, runner Runner, logger *slog.Logger) *Maker {
	t.Helper()
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	return NewMaker(runner, shelf, directory, encode.EncoderVAAPI, logger)
}

// hevcQuality is a run of a quality of an HEVC film.
func hevcQuality(path string) nodeproto.MakeChunks {
	command := run(nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 1200, Height: 480})
	command.VideoCodec = "hevc"
	command.Video = nodeproto.BytesFile{RootPath: "/films", Path: []byte(path)}
	return command
}

// On VAAPI a quality of a codec the card may decode is decoded there first; when FFmpeg fails on it
// before a chunk, as it does at the first picture of a film the card cannot decode, the run is made
// again at once decoded on the CPU, exactly as before, and says so, and every later run of the film
// decodes on the CPU from the start, while another film still tries the GPU (the owner, 2026-11-27).
func TestMake_AQualityTheCardCannotDecodeIsMadeAgainDecodedOnTheCPU(t *testing.T) {
	t.Parallel()
	ffmpeg := &gpuFailingFFmpeg{fake: &fakeFFmpeg{}}
	maker := vaapiMakerOver(t, ffmpeg)
	command := hevcQuality("a.mkv")
	last, decodedOn, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mkv"}, command, nil)
	if err != nil || last != -1 || decodedOn != encode.DecodedOnCPU {
		t.Fatalf("the run ended %d, %q, %v; want it made decoded on the CPU", last, decodedOn, err)
	}
	if len(ffmpeg.runs) != 2 || !slices.Contains(ffmpeg.runs[0], "-hwaccel") {
		t.Fatalf("FFmpeg ran %q; want once decoding on the GPU, then once on the CPU", ffmpeg.runs)
	}
	onCPU := Arguments(execx.Film{Path: "/films/a.mkv"}, valueOfRawPath(ffmpeg.runs[1]), command,
		encode.EncoderVAAPI, encode.DecodedOnCPU)
	if !slices.Equal(ffmpeg.runs[1], onCPU) {
		t.Errorf("the run again was %q; want today's, decoded on the CPU, %q", ffmpeg.runs[1], onCPU)
	}

	_, decodedOn, err = maker.Make(t.Context(), execx.Film{Path: "/films/a.mkv"}, command, nil)
	if err != nil || decodedOn != encode.DecodedOnCPU || len(ffmpeg.runs) != 3 ||
		slices.Contains(ffmpeg.runs[2], "-hwaccel") {
		t.Errorf("the film's next run decoded on %q, %v, its FFmpeg %q; want the CPU from the start", decodedOn,
			err, ffmpeg.runs[2:])
	}
	other := hevcQuality("b.mkv")
	_, decodedOn, err = maker.Make(t.Context(), execx.Film{Path: "/films/b.mkv"}, other, nil)
	if err != nil || len(ffmpeg.runs) != 5 || !slices.Contains(ffmpeg.runs[3], "-hwaccel") ||
		decodedOn != encode.DecodedOnCPU {
		t.Errorf("another film's run decoded on %q, %v, its FFmpeg %q; want the GPU tried first", decodedOn,
			err, ffmpeg.runs[3:])
	}
}

// valueOfRawPath is the raw directory a run's arguments name, where its segment list is.
func valueOfRawPath(arguments []string) string {
	list := valueOf(arguments, "-segment_list")
	return list[:len(list)-len("/"+rawListName)]
}

// A run decoding on the GPU that FFmpeg fails after it made chunks keeps them and says the failure,
// as any failed run does, and is not made again; the film's next run decodes on the CPU.
func TestMake_ARunDecodedOnTheGPUThatFailedAfterAChunkIsNotMadeAgain(t *testing.T) {
	t.Parallel()
	ffmpeg := &gpuFailingFFmpeg{fake: &fakeFFmpeg{}, madeOnGPU: &fakeFFmpeg{
		raw: map[int][]byte{10: rawOf(24000, fragmentOf(0, 240, 0))}, numbers: []int{10},
	}}
	maker := vaapiMakerOver(t, ffmpeg)
	command := hevcQuality("a.mkv")
	last, decodedOn, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mkv"}, command, nil)
	if err == nil || last != 10 || decodedOn != encode.DecodedOnGPU || len(ffmpeg.runs) != 1 {
		t.Errorf("the run ended %d, %q, %v after %d runs; want chunk 10 kept, the failure said, once on the GPU",
			last, decodedOn, err, len(ffmpeg.runs))
	}
	if _, _, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mkv"}, command, nil); err != nil ||
		slices.Contains(ffmpeg.runs[len(ffmpeg.runs)-1], "-hwaccel") {
		t.Errorf("the film's next run: %v, %q; want it decoded on the CPU", err, ffmpeg.runs[len(ffmpeg.runs)-1])
	}
}

// Only FFmpeg's own failure moves a film to the CPU: a run stopped, or stalled, is the player's or
// the film's, and the next run still decodes on the GPU.
func TestMake_AStoppedOrStalledRunOnTheGPUIsNotMadeAgain(t *testing.T) {
	t.Parallel()
	for name, failure := range map[string]error{
		"stalled": &execx.StalledError{},
		"killed":  &execx.RunError{Tool: execx.FFmpeg, ExitStatus: -1, Err: errors.New("signal: killed")},
	} {
		fake := &fakeFFmpeg{err: failure}
		maker := vaapiMakerOver(t, fake)
		command := hevcQuality("a.mkv")
		_, decodedOn, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mkv"}, command, nil)
		if !errors.Is(err, failure) || decodedOn != encode.DecodedOnGPU ||
			!slices.Contains(fake.ran.Arguments, "-hwaccel") {
			t.Errorf("a %s run ended decoded on %q, %v, its last FFmpeg %q; want it on the GPU, its error, "+
				"not made again",
				name, decodedOn, err, fake.ran.Arguments)
		}
		fake.err = nil
		_, decodedOn, err = maker.Make(t.Context(), execx.Film{Path: "/films/a.mkv"}, command, nil)
		if err != nil || decodedOn != encode.DecodedOnGPU {
			t.Errorf("after a %s run the film's next decoded on %q, %v; want the GPU", name, decodedOn, err)
		}
	}
}

// Only a quality decodes a picture: a copied picture or a sound says nowhere, and on x264, Quick
// Sync and NVENC a quality decodes on the CPU, as before; on VAAPI a codec FFmpeg has no VAAPI
// decoder for decodes on the CPU from the start.
func TestMake_WhereARunsPictureIsDecoded(t *testing.T) {
	t.Parallel()
	picture := run(nodeproto.ChunkStream{Kind: nodeproto.StreamPicture})
	fake := &fakeFFmpeg{}
	_, decodedOn, err := vaapiMakerOver(t, fake).Make(t.Context(), execx.Film{Path: "/a"}, picture, nil)
	if err != nil || decodedOn != "" {
		t.Errorf("a copied picture's run said it decoded on %q, %v; want nowhere", decodedOn, err)
	}
	for _, encoder := range []encode.Encoder{encode.EncoderX264, encode.EncoderQuickSync, encode.EncoderNVENC} {
		if encoder.DecodesOnGPU("hevc") {
			t.Errorf("%s decodes on the GPU; want only VAAPI", encoder)
		}
	}
	realVideo := hevcQuality("a.rm")
	realVideo.VideoCodec = "rv40"
	_, decodedOn, err = vaapiMakerOver(t, fake).Make(t.Context(), execx.Film{Path: "/a.rm"}, realVideo, nil)
	if err != nil || decodedOn != encode.DecodedOnCPU || slices.Contains(fake.ran.Arguments, "-hwaccel") {
		t.Errorf("RealVideo 4 on VAAPI decoded on %q, %v, its FFmpeg %q; want the CPU, as FFmpeg has no VAAPI "+
			"decoder for it", decodedOn, err, fake.ran.Arguments)
	}
}

// A quality decoded on the GPU is FFmpeg's command line decoded on the CPU but for VAAPI's decode
// options, just before the film, and its filter: its seek, its read, its re-timing, its keyframes,
// its time base, its cuts and its segments are the same, so its chunks start, are timed and are cut
// where the CPU's are, and one run's chunks follow another's in one store (the owner, 2026-11-27).
func TestArguments_AQualityDecodedOnTheGPUDiffersOnlyInItsDecodeAndItsFilter(t *testing.T) {
	t.Parallel()
	film := execx.Film{Path: "/films/a.mkv"}
	for _, command := range []nodeproto.MakeChunks{hevcQuality("a.mkv"), retimedQuality()} {
		onGPU := Arguments(film, "/r", command, encode.EncoderVAAPI, encode.DecodedOnGPU)
		onCPU := Arguments(film, "/r", command, encode.EncoderVAAPI, encode.DecodedOnCPU)
		input := slices.Index(onGPU, "-protocol_whitelist")
		decode := []string{"-hwaccel", "vaapi", "-hwaccel_device", "va", "-hwaccel_output_format", "vaapi"}
		if input < len(decode) || !slices.Equal(onGPU[input-len(decode):input], decode) {
			t.Errorf("decoded on the GPU: %q; want VAAPI's decode options just before the film is opened", onGPU)
			continue
		}
		withoutDecode := slices.Concat(onGPU[:input-len(decode)], onGPU[input:])
		filter := slices.Index(withoutDecode, "-vf") + 1
		if want := "setpts=PTS-STARTPTS,scale_vaapi=w=-2:h=480:format=nv12"; withoutDecode[filter] != want {
			t.Errorf("decoded on the GPU the filter is %q, want %q", withoutDecode[filter], want)
		}
		withoutDecode[filter] = onCPU[filter]
		if !slices.Equal(withoutDecode, onCPU) {
			t.Errorf("decoded on the GPU %q\non the CPU %q\nwant them the same but for the decode and the filter",
				onGPU, onCPU)
		}
	}
	for _, encoder := range []encode.Encoder{encode.EncoderX264, encode.EncoderQuickSync, encode.EncoderNVENC} {
		if !slices.Equal(Arguments(film, "/r", hevcQuality("a.mkv"), encoder, encode.DecodedOnGPU),
			Arguments(film, "/r", hevcQuality("a.mkv"), encoder, encode.DecodedOnCPU)) {
			t.Errorf("%s's run changes with where it is said to decode; want it untouched", encoder)
		}
	}
	picture := run(nodeproto.ChunkStream{Kind: nodeproto.StreamPicture})
	if slices.Contains(Arguments(film, "/r", picture, encode.EncoderVAAPI, encode.DecodedOnGPU), "-hwaccel") {
		t.Error("a copied picture's run decodes on the GPU; want it to decode nothing")
	}
}

// retimedQuality is a quality of an HEVC film whose B-frames carry no display times.
func retimedQuality() nodeproto.MakeChunks {
	command := hevcQuality("a.mkv")
	command.NeedsPictureRetiming = true
	return command
}

// The run made again says only that it decoded on the CPU, so the failed try is logged with what
// FFmpeg said: without it nobody can tell a card that cannot decode a film from one never asked.
func TestMake_AFailedDecodeOnTheGPUIsLoggedWithWhatFFmpegSaid(t *testing.T) {
	t.Parallel()
	var logged bytes.Buffer
	maker := vaapiMakerLoggingTo(t, &gpuFailingFFmpeg{fake: &fakeFFmpeg{}},
		slog.New(slog.NewTextHandler(&logged, nil)))
	command := hevcQuality("a.mkv")
	for range 2 {
		if _, _, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mkv"}, command, nil); err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(strings.TrimSpace(logged.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("logged %q; want one line, for the one run that failed on the GPU", lines)
	}
	wants := []string{"logEvent=gpuDecodeFailed", "videoCodec=hevc", "isMadeAgain=true", "Impossible to convert"}
	for _, want := range wants {
		if !strings.Contains(lines[0], want) {
			t.Errorf("logged %q; want %q in it", lines[0], want)
		}
	}
}
