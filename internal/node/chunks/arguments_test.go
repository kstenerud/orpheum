// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// valueOf is the argument after flag, or "" when it is not there.
func valueOf(arguments []string, flag string) string {
	if index := slices.Index(arguments, flag); index >= 0 && index+1 < len(arguments) {
		return arguments[index+1]
	}
	return ""
}

// run is a run of chunks 10 to 12, which start at 98.849, 108.859 and 112.613 s and end at
// 122.581 s, as 12 Patient Sailors's grid has them (slow-links.md §10); a copied picture's seeks to
// 103.854 s, midway to the keyframe after its first.
func run(stream nodeproto.ChunkStream) nodeproto.MakeChunks {
	command := nodeproto.MakeChunks{
		VideoStreamIndex: 0, VideoCodec: "h264", Stream: stream, StoreDir: "7/chunks-x/0123456789abcdef",
		FirstChunk: 10, BoundsMs: []int64{98_849, 108_859, 112_613, 122_581},
	}
	if stream.Kind == nodeproto.StreamPicture {
		command.PictureSeekMs = 103_854
	}
	return command
}

// A run reads from its first chunk to its last's end, and cuts at each chunk after the first and
// at the end, so what FFmpeg writes past it is a file of its own; a picture's cuts come a little
// early, so a keyframe at the time is never missed (slow-links.md §10). A copied picture seeks past
// its first keyframe, which FFmpeg lands on, and not the one before it (idiosyncrasies.md); a
// decoded stream seeks to its start, which a decode trims to.
func TestArguments_ReadTheRunsChunksAndCutAtEachAndAtTheEnd(t *testing.T) {
	t.Parallel()
	arguments := Arguments(execx.Film{Path: "/films/a.mp4"}, "/prepared/7/chunks-x/0123456789abcdef/raw-1",
		run(nodeproto.ChunkStream{Kind: nodeproto.StreamPicture}), encode.EncoderX264)
	if start, end := valueOf(arguments, "-ss"), valueOf(arguments, "-to"); start != "103.854" || end != "122.581" {
		t.Errorf("the run reads %s to %s, want from its seek, 103.854, to 122.581", start, end)
	}
	quality := Arguments(execx.Film{Path: "/a"}, "/r", run(nodeproto.ChunkStream{
		Kind: nodeproto.StreamQuality, BitRateKbps: 1200,
		Height: 480,
	}), encode.EncoderX264)
	if start := valueOf(quality, "-ss"); start != "98.849" {
		t.Errorf("an encoded quality reads from %s, want its start, 98.849", start)
	}
	if slices.Index(arguments, "-ss") > slices.Index(arguments, "-i") {
		t.Error("the start is an output option; it must seek the input")
	}
	if cuts := valueOf(arguments, "-segment_times"); cuts != "9.960,13.714,23.682" {
		t.Errorf("the cuts are %s, want 9.960,13.714,23.682: each chunk after the first, and the end, 50 ms early",
			cuts)
	}
	if first := valueOf(arguments, "-segment_start_number"); first != "10" {
		t.Errorf("the first raw file is %s, want 10", first)
	}
	if codec, tag := valueOf(arguments, "-c:v"), valueOf(arguments, "-tag:v"); codec != "copy" || tag != "" {
		t.Errorf("the picture is %s, tagged %q; want copied, untagged for H.264", codec, tag)
	}
	if options := valueOf(arguments, "-segment_format_options"); !strings.Contains(options, "use_editlist=0") ||
		!strings.Contains(options, "frag_keyframe") || !strings.Contains(options, "empty_moov") {
		t.Errorf("the raw files are written with %q; want fragmented MP4 without an edit list", options)
	}
}

// A copied picture's run past the film's first chunk is led in from the chunk before: it seeks past
// that chunk's keyframe, cuts at its own first chunk's start too, and numbers the lead-in's raw file
// as the chunk before, which the maker drops, so FFmpeg times an open GOP's pictures shown before
// the run's first keyframe as the film does (idiosyncrasies.md).
func TestArguments_ACopiedPictureIsLedInFromTheChunkBefore(t *testing.T) {
	t.Parallel()
	command := run(nodeproto.ChunkStream{Kind: nodeproto.StreamPicture})
	command.LeadInMs, command.PictureSeekMs = 88_839, 93_844
	arguments := Arguments(execx.Film{Path: "/films/a.mkv"}, "/r", command, encode.EncoderX264)
	if start := valueOf(arguments, "-ss"); start != "93.844" {
		t.Errorf("the run seeks to %s, want past its lead-in's keyframe, 93.844", start)
	}
	if cuts := valueOf(arguments, "-segment_times"); cuts != "9.960,19.970,23.724,33.692" {
		t.Errorf("the cuts are %s, want 9.960,19.970,23.724,33.692: the run's first chunk's start, each "+
			"after it, and the end, 50 ms early, from the lead-in", cuts)
	}
	if first := valueOf(arguments, "-segment_start_number"); first != "9" {
		t.Errorf("the first raw file is %s, want 9, the lead-in", first)
	}
}

// Every raw file holds its one stream: a picture carrying a timecode, as an MXF's or a camera's MOV
// does, gets no timecode track beside it, which the MP4 muxer adds unless told not to (D360).
func TestArguments_WriteNoTimecodeTrackBesideTheStream(t *testing.T) {
	t.Parallel()
	for _, kind := range []nodeproto.ChunkStreamKind{nodeproto.StreamPicture, nodeproto.StreamQuality} {
		arguments := Arguments(execx.Film{Path: "/a"}, "/r",
			run(nodeproto.ChunkStream{Kind: kind, BitRateKbps: 1200, Height: 480}), encode.EncoderX264)
		if options := valueOf(arguments, "-segment_format_options"); !strings.Contains(options, "write_tmcd=0") {
			t.Errorf("a %s run's raw files are written with %q; want no timecode track", kind, options)
		}
	}
}

// A copied picture's times start at zero at its first keyframe, which side of it the seek is: an
// MPEG-TS run seeks before the keyframe, and its copy would otherwise start as far after zero as the
// seek was before it, its raw files listed that much late (idiosyncrasies.md).
func TestArguments_ACopiedPictureIsTimedFromItsFirstKeyframe(t *testing.T) {
	t.Parallel()
	transport := run(nodeproto.ChunkStream{Kind: nodeproto.StreamPicture})
	transport.IsPictureSeekBeforeKeyframe, transport.PictureSeekMs = true, 93_844
	arguments := Arguments(execx.Film{Path: "/a.ts"}, "/r", transport, encode.EncoderX264)
	if shift := valueOf(arguments, "-avoid_negative_ts"); shift != "make_zero" {
		t.Errorf("the copy's times are shifted by %q; want make_zero", shift)
	}
	if start := valueOf(arguments, "-ss"); start != "93.844" {
		t.Errorf("the run reads from %s, want its seek before the keyframe, 93.844", start)
	}
	if cuts := valueOf(arguments, "-segment_times"); cuts != "9.960,13.714,23.682" {
		t.Errorf("the cuts are %s, want 9.960,13.714,23.682, from the run's first keyframe", cuts)
	}
}

// A copied HEVC picture is tagged hvc1, which Safari requires (cut-a §6).
func TestArguments_TagACopiedHEVCPicture(t *testing.T) {
	t.Parallel()
	hevc := run(nodeproto.ChunkStream{Kind: nodeproto.StreamPicture})
	hevc.VideoCodec = "hevc"
	if tag := valueOf(Arguments(execx.Film{Path: "/a"}, "/r", hevc, encode.EncoderX264), "-tag:v"); tag != "hvc1" {
		t.Errorf("a copied HEVC picture is tagged %q, want hvc1", tag)
	}
}

// A quality is encoded at its rate and height, with keyframes at its chunks' starts, relative to
// the run's, and nowhere else, so its chunks cut where the copied picture's do (D247 §10).
func TestArguments_EncodeAQualityWithKeyframesAtItsChunksStarts(t *testing.T) {
	t.Parallel()
	arguments := Arguments(execx.Film{Path: "/a"}, "/r",
		run(nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 1200, Height: 480}), encode.EncoderX264)
	if keyframes := valueOf(arguments, "-force_key_frames"); keyframes != "0.000,10.010,13.764" {
		t.Errorf("the keyframes are at %s, want 0.000,10.010,13.764", keyframes)
	}
	rate, peak, scale := valueOf(arguments, "-b:v"), valueOf(arguments, "-maxrate"), valueOf(arguments, "-vf")
	if rate != "1200k" || peak != "1800k" || scale != "scale=-2:480" {
		t.Errorf("the quality is %s, peaking at %s, %s; want 1200k, 1800k, scale=-2:480", rate, peak, scale)
	}
	if valueOf(arguments, "-sc_threshold") != "0" || valueOf(arguments, "-g") != "100000" {
		t.Error("x264 may put keyframes of its own at scene changes or a GOP's end")
	}
}

// A quality is encoded in its source's time base, on every encoder, so its MP4 track counts in the
// copied picture's timescale and hls.js takes no new time offset at a switch between them; each
// picture once at its own time, as the segment muxer would otherwise have FFmpeg hold a constant rate
// counted in the source's ticks (idiosyncrasies.md). A copy keeps its own times as it is.
func TestArguments_EncodeAQualityInItsSourcesTimeBase(t *testing.T) {
	t.Parallel()
	for _, encoder := range []encode.Encoder{
		encode.EncoderX264, encode.EncoderQuickSync, encode.EncoderNVENC, encode.EncoderVAAPI,
	} {
		arguments := Arguments(execx.Film{Path: "/a.ts"}, "/r",
			run(nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 1200, Height: 480}), encoder)
		timeBase, rate := valueOf(arguments, "-enc_time_base:v"), valueOf(arguments, "-fps_mode:v")
		if timeBase != "demux" || rate != "vfr" {
			t.Errorf("%s encodes at time base %q, frame rate mode %q; want demux, vfr", encoder, timeBase, rate)
		}
	}
	copied := Arguments(execx.Film{Path: "/a.ts"}, "/r", run(nodeproto.ChunkStream{Kind: nodeproto.StreamPicture}),
		encode.EncoderX264)
	if slices.Contains(copied, "-enc_time_base:v") || slices.Contains(copied, "-fps_mode:v") {
		t.Errorf("a copied picture is given an encoder's time base: %q", copied)
	}
}

// A picture whose B-frames carry no display times, copied or encoded, is re-timed by dts2pts, a
// copy's on the output and an encode's on its input, and read a second past its end, an
// encode keyframed at that end too so what is read past it is cut off; an ordinary picture is not,
// and a sound of a film whose picture is re-timed is read as any sound
// (design/plans/audio-switch-and-retiming-2026-11-01.md §2.4).
func TestArguments_ReTimeAPictureWhoseBFramesCarryNoDisplayTimes(t *testing.T) {
	t.Parallel()
	// A copy's filter is on its one output stream; an encode's on the picture's input stream, by its
	// index, moved back by setts so its first keyframe is shown at its bound.
	// A copy is cut 400 ms early, as dts2pts shows its keyframes each its own while after its decode
	// time (retimedCutEarlyMs); an encode, whose keyframes are forced at its bounds, 50 ms.
	for _, test := range []struct {
		kind nodeproto.ChunkStreamKind
		want [2]string
		cuts string
	}{
		{nodeproto.StreamPicture, [2]string{"-bsf:v", "dts2pts"}, "9.610,13.364,23.332"},
		{
			nodeproto.StreamQuality,
			[2]string{"-bsf:2", "dts2pts,setts=pts=PTS-STARTPTS+STARTDTS"},
			"9.960,13.714,23.682",
		},
	} {
		kind, want := test.kind, test.want
		marked := run(nodeproto.ChunkStream{Kind: kind, BitRateKbps: 1200, Height: 480})
		marked.NeedsPictureRetiming, marked.VideoStreamIndex = true, 2
		arguments := Arguments(execx.Film{Path: "/a"}, "/r", marked, encode.EncoderX264)
		filter, input := slices.Index(arguments, want[0]), slices.Index(arguments, "-i")
		if filter < 0 || valueOf(arguments, want[0]) != want[1] {
			t.Errorf("a marked %s run is %q; want it re-timed by %s %s", kind, arguments, want[0], want[1])
		}
		if isOnInput := filter < input; isOnInput != (kind == nodeproto.StreamQuality) {
			t.Errorf("a marked %s run filters its %s; want an encode's input and a copy's output", kind,
				map[bool]string{true: "input", false: "output"}[isOnInput])
		}
		if end := valueOf(arguments, "-to"); end != "123.581" {
			t.Errorf("a marked %s run reads to %s, want 123.581, a second past its end", kind, end)
		}
		if cuts := valueOf(arguments, "-segment_times"); cuts != test.cuts {
			t.Errorf("a marked %s run is cut at %s; want %s, at each chunk after the first and at its end",
				kind, cuts, test.cuts)
		}

		ordinary := run(nodeproto.ChunkStream{Kind: kind, BitRateKbps: 1200, Height: 480})
		unmarked := Arguments(execx.Film{Path: "/a"}, "/r", ordinary, encode.EncoderX264)
		if strings.Contains(strings.Join(unmarked, " "), "dts2pts") || valueOf(unmarked, "-to") != "122.581" {
			t.Errorf("an ordinary %s run is %q; want it neither re-timed nor read past its end", kind, unmarked)
		}
	}
	quality := run(nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 1200, Height: 480})
	quality.NeedsPictureRetiming = true
	keyframes := valueOf(Arguments(execx.Film{Path: "/a"}, "/r", quality, encode.EncoderX264), "-force_key_frames")
	if keyframes != "0.000,10.010,13.764,23.732" {
		t.Errorf("a marked encode's keyframes are at %s, want 0.000,10.010,13.764,23.732: its chunks' starts "+
			"and its end", keyframes)
	}
	sound := run(nodeproto.ChunkStream{Kind: nodeproto.StreamSound, Sound: &nodeproto.SoundTrack{}, SoundKbps: 96})
	sound.NeedsPictureRetiming = true
	arguments := Arguments(execx.Film{Path: "/a"}, "/r", sound, encode.EncoderX264)
	if strings.Contains(strings.Join(arguments, " "), "dts2pts") || valueOf(arguments, "-to") != "122.581" {
		t.Errorf("a sound run is %q; want it neither re-timed nor read past its end", arguments)
	}
}

// A sound past the film's first chunk starts a chunk early, at its lead-in, whose raw file is the
// one before the run's first; its cuts are at the bounds, where the next frame starts (slow-links.md
// §10).
func TestArguments_StartASoundAChunkEarlyAndCutItAtTheBounds(t *testing.T) {
	t.Parallel()
	sound := run(nodeproto.ChunkStream{
		Kind: nodeproto.StreamSound, IsBoosted: true, SoundKbps: 96,
		Sound: &nodeproto.SoundTrack{AudioTrackIndex: 1, ChannelCount: 6},
	})
	sound.LeadInMs = 88_839
	arguments := Arguments(execx.Film{Path: "/a"}, "/r", sound, encode.EncoderX264)
	if start := valueOf(arguments, "-ss"); start != "86.839" {
		t.Errorf("the sound's decode starts at %s, want two seconds before its lead-in, 86.839", start)
	}
	if first := valueOf(arguments, "-segment_start_number"); first != "9" {
		t.Errorf("the first raw file is %s, want 9, the lead-in's", first)
	}
	if cuts := valueOf(arguments, "-segment_times"); cuts != "10.010,20.020,23.774,33.742" {
		t.Errorf("the cuts are %s, want 10.010,20.020,23.774,33.742", cuts)
	}
	if mapped, mix := valueOf(arguments, "-map"), valueOf(arguments, "-filter:a"); mapped != "0:a:1" ||
		mix != "atrim=start=2.000,asetpts=PTS-STARTPTS,"+encode.MixFilter(*sound.Stream.Sound, true) {
		t.Errorf("the sound is %s filtered by %q; want the two seconds before its lead-in dropped, then "+
			"track 1's boosted mix", mapped, mix)
	}
	if rate := valueOf(arguments, "-b:a"); rate != "96k" {
		t.Errorf("the sound is encoded at %s; want the run's 96k (chunkSoundKbps)", rate)
	}
}

// A sound interleaved in super-blocks, as RealMedia's cook is, decodes from ten seconds before its
// lead-in, past the super-blocks FFmpeg loses where its seek lands, and one within ten seconds of the
// film's start is read from it unsought, as a seek to the start loses the first (idiosyncrasies.md).
func TestArguments_AnInterleavedSoundSeeksTenSecondsEarlyAndNeverToTheStart(t *testing.T) {
	t.Parallel()
	sound := run(nodeproto.ChunkStream{
		Kind: nodeproto.StreamSound, SoundKbps: 96, Sound: &nodeproto.SoundTrack{ChannelCount: 2},
	})
	sound.LeadInMs, sound.IsSoundInterleaved = 88_839, true
	arguments := Arguments(execx.Film{Path: "/a"}, "/r", sound, encode.EncoderX264)
	if start, filter := valueOf(arguments, "-ss"), valueOf(arguments, "-filter:a"); start != "78.839" ||
		!strings.HasPrefix(filter, "atrim=start=10.000,asetpts=PTS-STARTPTS,") {
		t.Errorf("the sound decodes from %s filtered by %q; want from 78.839, dropping ten seconds", start, filter)
	}
	sound.LeadInMs, sound.FirstChunk, sound.BoundsMs = 0, 0, []int64{0, 3_236, 6_400}
	arguments = Arguments(execx.Film{Path: "/a"}, "/r", sound, encode.EncoderX264)
	if slices.Contains(arguments, "-ss") || valueOf(arguments, "-to") != "6.400" ||
		!strings.HasPrefix(valueOf(arguments, "-filter:a"), "atrim=start=0.000,") {
		t.Errorf("the film's first chunks' sound reads %v; want it read from the start unsought", arguments)
	}
	sound.BoundsMs = []int64{8_000, 11_000}
	arguments = Arguments(execx.Film{Path: "/a"}, "/r", sound, encode.EncoderX264)
	if slices.Contains(arguments, "-ss") || !strings.HasPrefix(valueOf(arguments, "-filter:a"), "atrim=start=8.000,") {
		t.Errorf("a sound eight seconds in reads %v; want it read from the start unsought, dropping 8 s", arguments)
	}
	// A sound copied takes no interleaved path, said or not: it seeks to its run's start (D368).
	copied := run(nodeproto.ChunkStream{
		Kind: nodeproto.StreamSoundCopied, Sound: &nodeproto.SoundTrack{ChannelCount: 2},
	})
	copied.FirstChunk, copied.BoundsMs, copied.IsSoundInterleaved = 0, []int64{21, 3_236}, true
	arguments = Arguments(execx.Film{Path: "/a"}, "/r", copied, encode.EncoderX264)
	if start := valueOf(arguments, "-ss"); start != "0.021" {
		t.Errorf("a sound copied seeks to %q; want its run's start, 0.021, as any copy", start)
	}
}

// A sound starting less than two seconds into the film decodes from its start, read unsought, as a
// seek to the start loses an AVI's first packets (idiosyncrasies.md), and drops only what comes
// before its run.
func TestArguments_ASoundNearTheFilmsStartDecodesFromItsStart(t *testing.T) {
	t.Parallel()
	sound := run(nodeproto.ChunkStream{
		Kind: nodeproto.StreamSound, SoundKbps: 96, Sound: &nodeproto.SoundTrack{ChannelCount: 2},
	})
	sound.BoundsMs = []int64{1_500, 11_500}
	sound.FirstChunk = 0
	arguments := Arguments(execx.Film{Path: "/a"}, "/r", sound, encode.EncoderX264)
	if filter := valueOf(arguments, "-filter:a"); slices.Contains(arguments, "-ss") ||
		!strings.HasPrefix(filter, "atrim=start=1.500,asetpts=PTS-STARTPTS,") {
		t.Errorf("the sound decodes from %q filtered by %q; want from the start unsought, dropping 1.500 s",
			valueOf(arguments, "-ss"), filter)
	}
}

// No run seeks to the film's start, which it reads from unsought: FFmpeg moves a seek into a film with
// B-frames 3/23 s earlier, before the start, and an AVI seeking there lands past its first keyframe
// and its sound's first half second, which put a sound half a second early (idiosyncrasies.md).
func TestArguments_NoRunSeeksToTheFilmsStart(t *testing.T) {
	t.Parallel()
	for name, stream := range map[string]nodeproto.ChunkStream{
		"a quality": {Kind: nodeproto.StreamQuality, BitRateKbps: 300, Height: 96},
		"a sound": {
			Kind: nodeproto.StreamSound, SoundKbps: 96, Sound: &nodeproto.SoundTrack{ChannelCount: 2},
		},
		"a sound copied":     {Kind: nodeproto.StreamSoundCopied, Sound: &nodeproto.SoundTrack{ChannelCount: 2}},
		"an MPEG-TS picture": {Kind: nodeproto.StreamPicture},
	} {
		command := run(stream)
		command.FirstChunk, command.BoundsMs = 0, []int64{0, 2_000, 4_000}
		command.IsPictureSeekBeforeKeyframe, command.PictureSeekMs = stream.Kind == nodeproto.StreamPicture, 0
		arguments := Arguments(execx.Film{Path: "/a"}, "/r", command, encode.EncoderX264)
		if slices.Contains(arguments, "-ss") || valueOf(arguments, "-to") != "4.000" {
			t.Errorf("%s of the film's first chunks reads %q; want from the start unsought, to 4.000", name, arguments)
		}
	}
}

// A sound copied is copied as it is, every channel, from its lead-in, at its times in the film; its
// first packet put at the film's start, so the muxer, which times its cuts from a run's first packet,
// cuts it at the film's own times, the bounds, wherever FFmpeg's copy began (D368 §1;
// idiosyncrasies.md).
func TestArguments_CopyASoundAtItsTimesInTheFilmAndCutItAtTheBounds(t *testing.T) {
	t.Parallel()
	copied := run(nodeproto.ChunkStream{
		Kind: nodeproto.StreamSoundCopied, Sound: &nodeproto.SoundTrack{AudioTrackIndex: 1, ChannelCount: 6},
	})
	copied.LeadInMs = 88_839
	arguments := Arguments(execx.Film{Path: "/a"}, "/r", copied, encode.EncoderX264)
	if start := valueOf(arguments, "-ss"); start != "88.839" {
		t.Errorf("the copy reads from %s, want its lead-in, 88.839", start)
	}
	if mapped, codec := valueOf(arguments, "-map"), valueOf(arguments, "-c:a"); mapped != "0:a:1" || codec != "copy" {
		t.Errorf("the sound is %s as %s; want track 1 copied", mapped, codec)
	}
	if slices.Contains(arguments, "-ac") || slices.Contains(arguments, "-filter:a") {
		t.Errorf("the copy is mixed or filtered: %q", arguments)
	}
	keep := slices.Index(arguments, "-copyts")
	if keep < 0 || keep > slices.Index(arguments, "-i") || !slices.Contains(arguments, "-start_at_zero") {
		t.Errorf("the copy's times are not the film's, its start at zero: %q", arguments)
	}
	if first, shift := valueOf(arguments, "-bsf:a"), valueOf(arguments, "-avoid_negative_ts"); first !=
		`setts=ts=if(eq(N\,0)\,-1\,TS)` || shift != "disabled" {
		t.Errorf("the first packet is retimed by %q, the times shifted by %q; want it a tick before the film, "+
			"nothing else moved", first, shift)
	}
	if cuts := valueOf(arguments, "-segment_times"); cuts != "98.849,108.859,112.613,122.581" {
		t.Errorf("the cuts are %s, want the bounds in the film, 98.849,108.859,112.613,122.581", cuts)
	}
	if first := valueOf(arguments, "-segment_start_number"); first != "9" {
		t.Errorf("the first raw file is %s, want 9, the lead-in's", first)
	}
	if options := valueOf(arguments, "-segment_format_options"); !strings.Contains(options, "delay_moov") {
		t.Errorf("the raw files are written with %q; want the init after the first packet, as AC-3 needs", options)
	}
}

// A sound copied whose film's first chunk starts well after the film does, its picture's first
// keyframe counted 10.176 s in, is read from the film's start and cut at chunk 0's start too, as the
// picture is, the lead-in before it named 00000 and dropped: cut only at the bounds after its start,
// chunk 0 held the whole film before its end (A Hollow Continent, staging, 2026-10-28). A first chunk
// within the muxer's listing tolerance of the film's start keeps the sound from it, its priming too.
func TestArguments_ACopiedSoundWhoseFilmsFirstChunkStartsLateIsLedInFromTheFilmsStart(t *testing.T) {
	t.Parallel()
	copied := run(nodeproto.ChunkStream{
		Kind: nodeproto.StreamSoundCopied, Sound: &nodeproto.SoundTrack{ChannelCount: 6},
	})
	copied.FirstChunk, copied.BoundsMs = 0, []int64{10_176, 12_262, 22_522}
	arguments := Arguments(execx.Film{Path: "/a"}, "/r", copied, encode.EncoderX264)
	if slices.Contains(arguments, "-ss") {
		t.Errorf("the copy reads from %s, want the film's start, unsought", valueOf(arguments, "-ss"))
	}
	if cuts := valueOf(arguments, "-segment_times"); cuts != "10.176,12.262,22.522" {
		t.Errorf("the cuts are %s, want chunk 0's start and each bound after it, 10.176,12.262,22.522", cuts)
	}
	if first := valueOf(arguments, "-segment_start_number"); first != "0" {
		t.Errorf("the first raw file is %s, want 0, the lead-in's", first)
	}
	early := run(nodeproto.ChunkStream{
		Kind: nodeproto.StreamSoundCopied, Sound: &nodeproto.SoundTrack{ChannelCount: 2},
	})
	early.FirstChunk, early.BoundsMs = 0, []int64{21, 2_005}
	arguments = Arguments(execx.Film{Path: "/a"}, "/r", early, encode.EncoderX264)
	start, cuts := valueOf(arguments, "-ss"), valueOf(arguments, "-segment_times")
	if start != "0.021" || cuts != "2.005" {
		t.Errorf("a first chunk 21 ms in reads from %s and cuts at %s; want 0.021 and 2.005, no lead-in", start, cuts)
	}
}
