// SPDX-License-Identifier: AGPL-3.0-or-later

package decide_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// browser is the playable formats of a desktop Chrome's: H.264 and HEVC, the common audio, MP4 as it is.
var browser = decide.PlayableFormats{
	VideoCodecs:     []decide.Video{decide.HEVCSDR, decide.H264SDR},
	AudioCodecs:     []decide.Audio{decide.AACStereo, decide.Opus, decide.FLAC},
	SubtitleFormats: []decide.Subtitle{decide.ASS, decide.WebVTT},
	Containers:      []decide.Container{decide.MP4},
}

func videoStream(codec, profile, pixels, transfer string) nodeproto.ProbeStream {
	return nodeproto.ProbeStream{Kind: "video", Codec: codec, CodecProfile: profile, Video: nodeproto.ProbeVideo{
		PixelFormat: pixels, ColorTransfer: transfer,
	}}
}

func audioStream(codec, profile string, channels int, disposition ...string) nodeproto.ProbeStream {
	return nodeproto.ProbeStream{
		Kind: "audio", Codec: codec, CodecProfile: profile, Disposition: disposition,
		Audio: nodeproto.ProbeAudio{ChannelCount: channels},
	}
}

func probeOf(container string, streams ...nodeproto.ProbeStream) nodeproto.Probe {
	return nodeproto.Probe{Format: nodeproto.ProbeFormat{FormatNames: []string{container}}, Streams: streams}
}

// everyEntry lists every menu entry, so a test can ask what a stream was placed as.
var everyEntry = decide.PlayableFormats{
	VideoCodecs: []decide.Video{decide.H264SDR, decide.HEVCSDR, decide.HEVCHDR10, decide.AV1SDR, decide.AV1HDR10},
	AudioCodecs: []decide.Audio{decide.AACStereo, decide.AACMulti, decide.AC3, decide.EAC3, decide.Opus, decide.FLAC},
	Containers:  []decide.Container{decide.MP4, decide.Matroska, decide.MPEGTS},
}

// placedAs is the one video entry that, listed alone, plays directly, or "".
func placedAs(t *testing.T, stream nodeproto.ProbeStream) decide.Video {
	t.Helper()
	for _, name := range everyEntry.VideoCodecs {
		formats := everyEntry
		formats.VideoCodecs = []decide.Video{name}
		input := decide.Input{Probe: probeOf("mp4", stream), PlayableFormats: formats}
		if decide.Decide(input).Delivery == decide.Direct {
			return name
		}
	}
	return ""
}

func TestDecide_SortsEachVideoStreamIntoOneMenuEntry(t *testing.T) {
	t.Parallel()
	for name, row := range map[string]struct {
		stream nodeproto.ProbeStream
		want   decide.Video
	}{
		"H.264 High, 8-bit, BT.709":        {videoStream("h264", "High", "yuv420p", "bt709"), decide.H264SDR},
		"H.264 Main, full range, untagged": {videoStream("h264", "Main", "yuvj420p", ""), decide.H264SDR},
		"H.264 Constrained Baseline": {
			videoStream("h264", "Constrained Baseline", "yuv420p", ""), decide.H264SDR,
		},
		"H.264 High 10":    {videoStream("h264", "High 10", "yuv420p10le", "bt709"), ""},
		"H.264 High 4:2:2": {videoStream("h264", "High 4:2:2", "yuv422p", "bt709"), ""},
		"H.264 tagged PQ":  {videoStream("h264", "High", "yuv420p", "smpte2084"), ""},
		"HEVC Main":        {videoStream("hevc", "Main", "yuv420p", "bt709"), decide.HEVCSDR},
		"HEVC Main 10 SDR": {videoStream("hevc", "Main 10", "yuv420p10le", "bt709"), decide.HEVCSDR},
		"HEVC Main 10 HDR10": {
			videoStream("hevc", "Main 10", "yuv420p10le", "smpte2084"), decide.HEVCHDR10,
		},
		"HEVC HLG":          {videoStream("hevc", "Main 10", "yuv420p10le", "arib-std-b67"), ""},
		"HEVC PQ at 8 bits": {videoStream("hevc", "Main", "yuv420p", "smpte2084"), ""},
		"HEVC Rext 4:4:4":   {videoStream("hevc", "Rext", "yuv444p", "bt709"), ""},
		"AV1 Main 10 SDR":   {videoStream("av1", "Main", "yuv420p10le", ""), decide.AV1SDR},
		"AV1 HDR10":         {videoStream("av1", "Main", "yuv420p10le", "smpte2084"), decide.AV1HDR10},
		"MPEG-2":            {videoStream("mpeg2video", "Main", "yuv420p", ""), ""},
		"VC-1":              {videoStream("vc1", "Advanced", "yuv420p", ""), ""},
		"RealVideo 1.0":     {videoStream("rv10", "", "yuv420p", ""), ""},
		"RealVideo 2.0":     {videoStream("rv20", "", "yuv420p", ""), ""},
		"RealVideo 3.0":     {videoStream("rv30", "", "yuv420p", ""), ""},
		"RealVideo 4.0":     {videoStream("rv40", "", "yuv420p", ""), ""},
	} {
		if got := placedAs(t, row.stream); got != row.want {
			t.Errorf("%s: placed as %q, want %q", name, got, row.want)
		}
	}
}

func TestDecide_SortsEachAudioStreamIntoOneMenuEntry(t *testing.T) {
	t.Parallel()
	picture := videoStream("h264", "High", "yuv420p", "")
	for name, row := range map[string]struct {
		stream nodeproto.ProbeStream
		want   decide.Audio
	}{
		"AAC LC stereo":       {audioStream("aac", "LC", 2), decide.AACStereo},
		"AAC, profile unsaid": {audioStream("aac", "", 1), decide.AACStereo},
		"HE-AAC 5.1":          {audioStream("aac", "HE-AAC", 6), decide.AACMulti},
		"xHE-AAC":             {audioStream("aac", "xHE-AAC", 2), ""},
		"AC-3":                {audioStream("ac3", "", 6), decide.AC3},
		"E-AC-3":              {audioStream("eac3", "", 8), decide.EAC3},
		"Opus":                {audioStream("opus", "", 2), decide.Opus},
		"FLAC":                {audioStream("flac", "", 2), decide.FLAC},
		"TrueHD":              {audioStream("truehd", "", 8), ""},
		"DTS":                 {audioStream("dts", "DTS-HD MA", 6), ""},
		"MP3":                 {audioStream("mp3", "", 2), ""},
		"RealAudio Cook":      {audioStream("cook", "", 2), ""},
		"RealAudio SIPR":      {audioStream("sipr", "", 1), ""},
		"ATRAC3":              {audioStream("atrac3", "", 2), ""},
		"RealAudio Lossless":  {audioStream("ralf", "", 2), ""},
		"RealAudio 14.4":      {audioStream("ra_144", "", 1), ""},
		"RealAudio 28.8":      {audioStream("ra_288", "", 1), ""},
	} {
		got := decide.Audio("")
		for _, entry := range everyEntry.AudioCodecs {
			formats := everyEntry
			formats.AudioCodecs = []decide.Audio{decide.AACStereo, entry}
			if entry != decide.AACStereo {
				formats.AudioCodecs = []decide.Audio{entry}
			}
			input := decide.Input{Probe: probeOf("mp4", picture, row.stream), PlayableFormats: formats}
			if decide.Decide(input).Delivery == decide.Direct {
				got = entry
				break
			}
		}
		if got != row.want {
			t.Errorf("%s: placed as %q, want %q", name, got, row.want)
		}
	}
}

func TestDecide_TheAcceptanceFileIsRemuxedForTheContainerAndTheAudio(t *testing.T) {
	t.Parallel()
	// cut-a §8's M3 file: an MKV with H.264, TrueHD and ASS.
	mkv := probeOf("matroska", videoStream("h264", "High", "yuv420p", "bt709"), audioStream("truehd", "", 8, "default"),
		nodeproto.ProbeStream{Kind: "subtitle", Codec: "ass"})
	got := decide.Decide(decide.Input{Probe: mkv, PlayableFormats: browser})
	want := decide.Decision{
		Delivery: decide.HLS, Profile: decide.Remux,
		NotDirectReasons: []decide.Reason{decide.ReasonContainer, decide.ReasonAudioCodec},
	}
	if got.Delivery != want.Delivery || got.Profile != want.Profile ||
		!slices.Equal(got.NotDirectReasons, want.NotDirectReasons) {
		t.Errorf("Decide = %+v, want %+v", got, want)
	}
}

// A RealMedia film is encoded on its ladder, picture and sound, whatever a device plays: no menu
// entry is its container, RealVideo or RealAudio (D344; the owner, 2026-10-27).
func TestDecide_ARealMediaFilmIsEncodedOnItsLadder(t *testing.T) {
	t.Parallel()
	for _, codecs := range [][2]string{{"rv40", "cook"}, {"rv30", "sipr"}, {"rv20", "ra_144"}, {"rv10", "ra_288"}} {
		film := probeOf("rm", videoStream(codecs[0], "", "yuv420p", ""), audioStream(codecs[1], "", 2))
		got := decide.Decide(decide.Input{Probe: film, PlayableFormats: everyEntry})
		want := []decide.Reason{decide.ReasonContainer, decide.ReasonVideoCodec, decide.ReasonAudioCodec}
		if got.Delivery != decide.HLS || got.Profile != decide.H264 || !slices.Equal(got.NotDirectReasons, want) {
			t.Errorf("RealMedia with %s and %s: %+v, want hls, h264, for %v", codecs[0], codecs[1], got, want)
		}
	}
}

func TestDecide_PlaysDirectlyWhatFitsEveryAxis(t *testing.T) {
	t.Parallel()
	mp4 := probeOf("mp4", videoStream("h264", "High", "yuv420p", ""), audioStream("aac", "LC", 2))
	got := decide.Decide(decide.Input{Probe: mp4, PlayableFormats: browser})
	if got.Delivery != decide.Direct || len(got.NotDirectReasons) != 0 {
		t.Errorf("Decide = %+v, want direct with no reason", got)
	}
}

func TestDecide_ReEncodesWhatTheClientCannotDecode(t *testing.T) {
	t.Parallel()
	mpeg2 := probeOf("mp4", videoStream("mpeg2video", "Main", "yuv420p", ""), audioStream("aac", "LC", 2))
	got := decide.Decide(decide.Input{Probe: mpeg2, PlayableFormats: browser})
	if got.Profile != decide.H264 || !slices.Equal(got.NotDirectReasons, []decide.Reason{decide.ReasonVideoCodec}) {
		t.Errorf("Decide = %+v, want h264 for video_codec", got)
	}
}

func TestDecide_JudgesTheTrackAPlayerStartsWith(t *testing.T) {
	t.Parallel()
	picture := videoStream("h264", "High", "yuv420p", "")
	defaultAAC := probeOf("mp4", picture, audioStream("dts", "", 6), audioStream("aac", "LC", 2, "default"))
	if got := decide.Decide(decide.Input{Probe: defaultAAC, PlayableFormats: browser}); got.Delivery != decide.Direct {
		t.Errorf("a default AAC track after a DTS one: %+v, want direct", got)
	}
	firstDTS := probeOf("mp4", picture, audioStream("dts", "", 6), audioStream("aac", "LC", 2))
	if got := decide.Decide(decide.Input{Probe: firstDTS, PlayableFormats: browser}); got.Delivery != decide.HLS {
		t.Errorf("no default and DTS first: %+v, want prepared", got)
	}
	coverArt := nodeproto.ProbeStream{Kind: "video", Codec: "mjpeg", Disposition: []string{"attached_pic"}}
	withCover := probeOf("mp4", coverArt, picture, audioStream("aac", "LC", 2))
	if got := decide.Decide(decide.Input{Probe: withCover, PlayableFormats: browser}); got.Delivery != decide.Direct {
		t.Errorf("cover art before the picture: %+v, want direct, judged on the picture", got)
	}
}

// A file plays as it is whatever its bit rate: nothing knows where the client is, and a link too
// slow for it shows itself by its stalls (D247 §11).
func TestDecide_AFileOfAnyBitRatePlaysAsItIs(t *testing.T) {
	t.Parallel()
	fast := probeOf("mp4", videoStream("h264", "High", "yuv420p", ""), audioStream("aac", "LC", 2))
	fast.Format.BitRateBps = 40_000_000
	if got := decide.Decide(decide.Input{Probe: fast, PlayableFormats: browser}); got.Delivery != decide.Direct {
		t.Errorf("a 40 Mb/s file: %+v, want direct", got)
	}
}

// A surround sound the device can play is played as it is, the device mixing it down itself, until
// the person turns the dialogue boost on, which the server must make (D336 §1, §2); a stereo sound
// has no centre to raise, so the boost prepares nothing.
func TestDecide_TheBoostPreparesOnlyASourceWithACentre(t *testing.T) {
	t.Parallel()
	picture := videoStream("h264", "High", "yuv420p", "")
	surround := probeOf("mp4", picture, audioStream("aac", "LC", 6))
	formats := browser
	formats.AudioCodecs = append(formats.AudioCodecs, decide.AACMulti)
	if got := decide.Decide(decide.Input{Probe: surround, PlayableFormats: formats}); got.Delivery != decide.Direct {
		t.Errorf("5.1 the device plays, the boost off: %+v, want direct: it plays as it is (D336 §1)", got)
	}
	got := decide.Decide(decide.Input{Probe: surround, PlayableFormats: formats, IsDialogueBoostOn: true})
	if got.Profile != decide.Remux || !slices.Equal(got.NotDirectReasons, []decide.Reason{decide.ReasonDialogueBoost}) {
		t.Errorf("5.1 with the boost on: %+v, want remux for dialogue_boost", got)
	}
	got = decide.Decide(decide.Input{Probe: surround, PlayableFormats: browser})
	if !slices.Equal(got.NotDirectReasons, []decide.Reason{decide.ReasonAudioCodec}) {
		t.Errorf("5.1 AAC on a device without aac-multi: %+v, want audio_codec: it cannot play the channels", got)
	}
	stereo := probeOf("mp4", picture, audioStream("aac", "LC", 2))
	got = decide.Decide(decide.Input{Probe: stereo, PlayableFormats: browser, IsDialogueBoostOn: true})
	if got.Delivery != decide.Direct {
		t.Errorf("stereo with the boost on: %+v, want direct: there is no centre to raise", got)
	}
}

func TestDecide_APersonsChoiceWins(t *testing.T) {
	t.Parallel()
	mkv := probeOf("matroska", videoStream("h264", "High", "yuv420p", ""), audioStream("truehd", "", 8))
	forced := decide.Decide(decide.Input{Probe: mkv, PlayableFormats: browser, PreferredDelivery: decide.Direct})
	if got := forced; got.Delivery != decide.Direct {
		t.Errorf("prefer direct: %+v, want direct", got)
	}
	mp4 := probeOf("mp4", videoStream("h264", "High", "yuv420p", ""), audioStream("aac", "LC", 2))
	got := decide.Decide(decide.Input{Probe: mp4, PlayableFormats: browser, PreferredDelivery: decide.HLS})
	if got.Delivery != decide.HLS || !slices.Equal(got.NotDirectReasons, []decide.Reason{decide.ReasonUserChoice}) {
		t.Errorf("prefer prepare: %+v, want hls for user_choice", got)
	}
}

// A disc image's title is never played as it is, even when a person prefers it and its streams fit:
// the original's bytes are the image's (D350 §4). Its picture is copied when the device plays it.
func TestDecide_ADiscImageIsNeverPlayedDirect(t *testing.T) {
	t.Parallel()
	bluRay := probeOf("mpegts", videoStream("h264", "High", "yuv420p", ""), audioStream("aac", "LC", 2))
	bluRay.Format.Disc = &nodeproto.ProbeDisc{Kind: "bluray", TitleNumber: 800}
	formats := browser
	formats.Containers = append(slices.Clone(formats.Containers), decide.MPEGTS)
	for _, preferred := range []decide.Delivery{"", decide.Direct} {
		got := decide.Decide(decide.Input{Probe: bluRay, PlayableFormats: formats, PreferredDelivery: preferred})
		if got.Delivery != decide.HLS || got.Profile != decide.Remux ||
			!slices.Equal(got.NotDirectReasons, []decide.Reason{decide.ReasonContainer}) {
			t.Errorf("a Blu-ray's title, %q preferred, decided %+v; want its picture copied for the container",
				preferred, got)
		}
	}
}

// A chosen audio track that is not the one the original starts with is prepared, since a browser
// plays the original's first, and it is the chosen track whose codec is asked (D230 §5).
func TestDecide_AChosenAudioTrackOtherThanTheFirstIsPreparedAndItsCodecAsked(t *testing.T) {
	t.Parallel()
	english, french := audioStream("aac", "LC", 2, "default"), audioStream("aac", "LC", 2)
	english.StreamIndex, french.StreamIndex = 1, 2
	picture := videoStream("h264", "High", "yuv420p", "")
	two := probeOf("mp4", picture, english, french)
	first := decide.Decide(decide.Input{Probe: two, PlayableFormats: browser, AudioStreamIndex: new(1)})
	if got := first; got.Delivery != decide.Direct {
		t.Errorf("the first track chosen: %+v, want direct", got)
	}
	got := decide.Decide(decide.Input{Probe: two, PlayableFormats: browser, AudioStreamIndex: new(2)})
	if got.Profile != decide.Remux || !slices.Equal(got.NotDirectReasons, []decide.Reason{decide.ReasonAudioTrack}) {
		t.Errorf("the second track chosen: %+v, want remux for audio_track", got)
	}
	french.Codec = "truehd"
	two = probeOf("mp4", picture, english, french)
	got = decide.Decide(decide.Input{Probe: two, PlayableFormats: browser, AudioStreamIndex: new(2)})
	if !slices.Contains(got.NotDirectReasons, decide.ReasonAudioCodec) {
		t.Errorf("a chosen track the browser cannot decode: %+v, want audio_codec among the reasons", got)
	}
}

func TestPlayableFormats_RefusesAListWithoutTheBaselinesOrWithAnUnknownName(t *testing.T) {
	t.Parallel()
	if err := browser.Validate(); err != nil {
		t.Errorf("a browser's playable formats: %v", err)
	}
	for name, formats := range map[string]decide.PlayableFormats{
		"no h264-sdr":   {VideoCodecs: []decide.Video{decide.HEVCSDR}, AudioCodecs: []decide.Audio{decide.AACStereo}},
		"no aac-stereo": {VideoCodecs: []decide.Video{decide.H264SDR}, AudioCodecs: []decide.Audio{decide.Opus}},
		"an unknown codec": {
			VideoCodecs: []decide.Video{decide.H264SDR, "vp9"}, AudioCodecs: []decide.Audio{decide.AACStereo},
		},
		"an unknown container": {
			VideoCodecs: []decide.Video{decide.H264SDR}, AudioCodecs: []decide.Audio{decide.AACStereo},
			Containers: []decide.Container{"avi"},
		},
	} {
		if err := formats.Validate(); !errors.Is(err, decide.ErrBadPlayableFormats) {
			t.Errorf("%s: %v, want ErrBadPlayableFormats", name, err)
		}
	}
}

func TestIsTextSubtitle_OffersTextTracksAndNoBitmaps(t *testing.T) {
	t.Parallel()
	for codec, want := range map[string]bool{
		"ass": true, "ssa": true, "subrip": true, "webvtt": true, "mov_text": true, "text": true,
		"hdmv_pgs_subtitle": false, "dvd_subtitle": false, "dvb_subtitle": false, "eia_608": false, "": false,
	} {
		if got := decide.IsTextSubtitle(codec); got != want {
			t.Errorf("IsTextSubtitle(%q) = %v, want %v", codec, got, want)
		}
	}
}
