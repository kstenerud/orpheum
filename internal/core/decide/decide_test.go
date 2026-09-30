// SPDX-License-Identifier: GPL-3.0-or-later

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
	got := decide.Decide(decide.Input{Probe: mkv, PlayableFormats: browser, Network: decide.Local})
	want := decide.Decision{
		Delivery: decide.HLS, Profile: decide.Remux,
		NotDirectReasons: []decide.Reason{decide.ReasonContainer, decide.ReasonAudioCodec},
	}
	if got.Delivery != want.Delivery || got.Profile != want.Profile ||
		!slices.Equal(got.NotDirectReasons, want.NotDirectReasons) {
		t.Errorf("Decide = %+v, want %+v", got, want)
	}
}

func TestDecide_PlaysDirectlyWhatFitsEveryAxis(t *testing.T) {
	t.Parallel()
	mp4 := probeOf("mp4", videoStream("h264", "High", "yuv420p", ""), audioStream("aac", "LC", 2))
	got := decide.Decide(decide.Input{Probe: mp4, PlayableFormats: browser, Network: decide.Local})
	if got.Delivery != decide.Direct || len(got.NotDirectReasons) != 0 {
		t.Errorf("Decide = %+v, want direct with no reason", got)
	}
}

func TestDecide_ReEncodesWhatTheClientCannotDecode(t *testing.T) {
	t.Parallel()
	mpeg2 := probeOf("mp4", videoStream("mpeg2video", "Main", "yuv420p", ""), audioStream("aac", "LC", 2))
	got := decide.Decide(decide.Input{Probe: mpeg2, PlayableFormats: browser})
	if got.Profile != decide.H264At8M || !slices.Equal(got.NotDirectReasons, []decide.Reason{decide.ReasonVideoCodec}) {
		t.Errorf("Decide = %+v, want h264-8m for video_codec", got)
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

func TestDecide_AFastFileIsReEncodedForARemoteLinkOnly(t *testing.T) {
	t.Parallel()
	fast := probeOf("mp4", videoStream("h264", "High", "yuv420p", ""), audioStream("aac", "LC", 2))
	fast.Format.BitRateBps = 40_000_000
	remote := decide.Decide(decide.Input{
		Probe: fast, PlayableFormats: browser, Network: decide.Remote, RemoteCeilingKbps: 10_000,
	})
	if remote.Profile != decide.H264At8M ||
		!slices.Equal(remote.NotDirectReasons, []decide.Reason{decide.ReasonBandwidth}) {
		t.Errorf("remote: %+v, want h264-8m for bandwidth", remote)
	}
	local := decide.Decide(decide.Input{
		Probe: fast, PlayableFormats: browser, Network: decide.Local, RemoteCeilingKbps: 10_000,
	})
	if local.Delivery != decide.Direct {
		t.Errorf("local: %+v, want direct", local)
	}
}

func TestDecide_TheBoostPreparesOnlyASourceWithACentre(t *testing.T) {
	t.Parallel()
	picture := videoStream("h264", "High", "yuv420p", "")
	surround := probeOf("mp4", picture, audioStream("aac", "LC", 6))
	formats := browser
	formats.AudioCodecs = append(formats.AudioCodecs, decide.AACMulti)
	got := decide.Decide(decide.Input{Probe: surround, PlayableFormats: formats, IsStereoOutput: true})
	if got.Profile != decide.Remux || !slices.Equal(got.NotDirectReasons, []decide.Reason{decide.ReasonDialogueBoost}) {
		t.Errorf("5.1 on stereo output: %+v, want remux for dialogue_boost", got)
	}
	stereo := probeOf("mp4", picture, audioStream("aac", "LC", 2))
	got = decide.Decide(decide.Input{Probe: stereo, PlayableFormats: browser, IsStereoOutput: true})
	if got.Delivery != decide.Direct {
		t.Errorf("stereo on stereo output: %+v, want direct: there is no centre to raise", got)
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
