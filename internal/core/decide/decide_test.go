// SPDX-License-Identifier: GPL-3.0-or-later

package decide_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// browser is a menu like a desktop Chrome's: H.264 and HEVC, the common audio, MP4 as it is.
var browser = decide.Menu{
	Video:      []decide.Video{decide.HEVCSDR, decide.H264SDR},
	Audio:      []decide.Audio{decide.AACStereo, decide.Opus, decide.FLAC},
	Subtitle:   []decide.Subtitle{decide.ASS, decide.WebVTT},
	Containers: []decide.Container{decide.MP4},
}

func video(codec, profile, pixels, transfer string) nodeproto.ProbeStream {
	return nodeproto.ProbeStream{Kind: "video", Codec: codec, Profile: profile, Video: nodeproto.ProbeVideo{
		PixelFormat: pixels, ColorTransfer: transfer,
	}}
}

func audio(codec, profile string, channels int, disposition ...string) nodeproto.ProbeStream {
	return nodeproto.ProbeStream{
		Kind: "audio", Codec: codec, Profile: profile, Disposition: disposition,
		Audio: nodeproto.ProbeAudio{Channels: channels},
	}
}

func file(container string, streams ...nodeproto.ProbeStream) nodeproto.Probe {
	return nodeproto.Probe{Format: nodeproto.ProbeFormat{Names: []string{container}}, Streams: streams}
}

// every lists every menu entry, so a test can ask what a stream was placed as.
var every = decide.Menu{
	Video:      []decide.Video{decide.H264SDR, decide.HEVCSDR, decide.HEVCHDR10, decide.AV1SDR, decide.AV1HDR10},
	Audio:      []decide.Audio{decide.AACStereo, decide.AACMulti, decide.AC3, decide.EAC3, decide.Opus, decide.FLAC},
	Containers: []decide.Container{decide.MP4, decide.Matroska, decide.MPEGTS},
}

// placedAs is the one video entry a menu of that entry alone plays directly, or "".
func placedAs(t *testing.T, stream nodeproto.ProbeStream) decide.Video {
	t.Helper()
	for _, name := range every.Video {
		menu := every
		menu.Video = []decide.Video{name}
		if decide.Decide(decide.Input{Probe: file("mp4", stream), Menu: menu}).Kind == decide.Direct {
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
		"H.264 High, 8-bit, BT.709":        {video("h264", "High", "yuv420p", "bt709"), decide.H264SDR},
		"H.264 Main, full range, untagged": {video("h264", "Main", "yuvj420p", ""), decide.H264SDR},
		"H.264 Constrained Baseline":       {video("h264", "Constrained Baseline", "yuv420p", ""), decide.H264SDR},
		"H.264 High 10":                    {video("h264", "High 10", "yuv420p10le", "bt709"), ""},
		"H.264 High 4:2:2":                 {video("h264", "High 4:2:2", "yuv422p", "bt709"), ""},
		"H.264 tagged PQ":                  {video("h264", "High", "yuv420p", "smpte2084"), ""},
		"HEVC Main":                        {video("hevc", "Main", "yuv420p", "bt709"), decide.HEVCSDR},
		"HEVC Main 10 SDR":                 {video("hevc", "Main 10", "yuv420p10le", "bt709"), decide.HEVCSDR},
		"HEVC Main 10 HDR10":               {video("hevc", "Main 10", "yuv420p10le", "smpte2084"), decide.HEVCHDR10},
		"HEVC HLG":                         {video("hevc", "Main 10", "yuv420p10le", "arib-std-b67"), ""},
		"HEVC PQ at 8 bits":                {video("hevc", "Main", "yuv420p", "smpte2084"), ""},
		"HEVC Rext 4:4:4":                  {video("hevc", "Rext", "yuv444p", "bt709"), ""},
		"AV1 Main 10 SDR":                  {video("av1", "Main", "yuv420p10le", ""), decide.AV1SDR},
		"AV1 HDR10":                        {video("av1", "Main", "yuv420p10le", "smpte2084"), decide.AV1HDR10},
		"MPEG-2":                           {video("mpeg2video", "Main", "yuv420p", ""), ""},
		"VC-1":                             {video("vc1", "Advanced", "yuv420p", ""), ""},
	} {
		if got := placedAs(t, row.stream); got != row.want {
			t.Errorf("%s: placed as %q, want %q", name, got, row.want)
		}
	}
}

func TestDecide_SortsEachAudioStreamIntoOneMenuEntry(t *testing.T) {
	t.Parallel()
	picture := video("h264", "High", "yuv420p", "")
	for name, row := range map[string]struct {
		stream nodeproto.ProbeStream
		want   decide.Audio
	}{
		"AAC LC stereo":       {audio("aac", "LC", 2), decide.AACStereo},
		"AAC, profile unsaid": {audio("aac", "", 1), decide.AACStereo},
		"HE-AAC 5.1":          {audio("aac", "HE-AAC", 6), decide.AACMulti},
		"xHE-AAC":             {audio("aac", "xHE-AAC", 2), ""},
		"AC-3":                {audio("ac3", "", 6), decide.AC3},
		"E-AC-3":              {audio("eac3", "", 8), decide.EAC3},
		"Opus":                {audio("opus", "", 2), decide.Opus},
		"FLAC":                {audio("flac", "", 2), decide.FLAC},
		"TrueHD":              {audio("truehd", "", 8), ""},
		"DTS":                 {audio("dts", "DTS-HD MA", 6), ""},
		"MP3":                 {audio("mp3", "", 2), ""},
	} {
		got := decide.Audio("")
		for _, entry := range every.Audio {
			menu := every
			menu.Audio = []decide.Audio{decide.AACStereo, entry}
			if entry != decide.AACStereo {
				menu.Audio = []decide.Audio{entry}
			}
			if decide.Decide(decide.Input{Probe: file("mp4", picture, row.stream), Menu: menu}).Kind == decide.Direct {
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
	mkv := file("matroska", video("h264", "High", "yuv420p", "bt709"), audio("truehd", "", 8, "default"),
		nodeproto.ProbeStream{Kind: "subtitle", Codec: "ass"})
	got := decide.Decide(decide.Input{Probe: mkv, Menu: browser, Link: decide.Local})
	want := decide.Decision{
		Kind: decide.HLS, Profile: decide.Remux,
		Reasons: []decide.Reason{decide.ReasonContainer, decide.ReasonAudioCodec},
	}
	if got.Kind != want.Kind || got.Profile != want.Profile || !slices.Equal(got.Reasons, want.Reasons) {
		t.Errorf("Decide = %+v, want %+v", got, want)
	}
}

func TestDecide_PlaysDirectlyWhatFitsEveryAxis(t *testing.T) {
	t.Parallel()
	mp4 := file("mp4", video("h264", "High", "yuv420p", ""), audio("aac", "LC", 2))
	if got := decide.Decide(decide.Input{Probe: mp4, Menu: browser, Link: decide.Local}); got.Kind != decide.Direct ||
		len(got.Reasons) != 0 {
		t.Errorf("Decide = %+v, want direct with no reason", got)
	}
}

func TestDecide_ReEncodesWhatTheClientCannotDecode(t *testing.T) {
	t.Parallel()
	mpeg2 := file("mp4", video("mpeg2video", "Main", "yuv420p", ""), audio("aac", "LC", 2))
	got := decide.Decide(decide.Input{Probe: mpeg2, Menu: browser})
	if got.Profile != decide.H264At8M || !slices.Equal(got.Reasons, []decide.Reason{decide.ReasonVideoCodec}) {
		t.Errorf("Decide = %+v, want h264-8m for video_codec", got)
	}
}

func TestDecide_JudgesTheTrackAPlayerStartsWith(t *testing.T) {
	t.Parallel()
	picture := video("h264", "High", "yuv420p", "")
	defaultAAC := file("mp4", picture, audio("dts", "", 6), audio("aac", "LC", 2, "default"))
	if got := decide.Decide(decide.Input{Probe: defaultAAC, Menu: browser}); got.Kind != decide.Direct {
		t.Errorf("a default AAC track after a DTS one: %+v, want direct", got)
	}
	firstDTS := file("mp4", picture, audio("dts", "", 6), audio("aac", "LC", 2))
	if got := decide.Decide(decide.Input{Probe: firstDTS, Menu: browser}); got.Kind != decide.HLS {
		t.Errorf("no default and DTS first: %+v, want prepared", got)
	}
	coverArt := nodeproto.ProbeStream{Kind: "video", Codec: "mjpeg", Disposition: []string{"attached_pic"}}
	withCover := file("mp4", coverArt, picture, audio("aac", "LC", 2))
	if got := decide.Decide(decide.Input{Probe: withCover, Menu: browser}); got.Kind != decide.Direct {
		t.Errorf("cover art before the picture: %+v, want direct, judged on the picture", got)
	}
}

func TestDecide_AFastFileIsReEncodedForARemoteLinkOnly(t *testing.T) {
	t.Parallel()
	fast := file("mp4", video("h264", "High", "yuv420p", ""), audio("aac", "LC", 2))
	fast.Format.BitRateBps = 40_000_000
	remote := decide.Decide(decide.Input{Probe: fast, Menu: browser, Link: decide.Remote, RemoteCeilingKbps: 10_000})
	if remote.Profile != decide.H264At8M || !slices.Equal(remote.Reasons, []decide.Reason{decide.ReasonBandwidth}) {
		t.Errorf("remote: %+v, want h264-8m for bandwidth", remote)
	}
	local := decide.Decide(decide.Input{Probe: fast, Menu: browser, Link: decide.Local, RemoteCeilingKbps: 10_000})
	if local.Kind != decide.Direct {
		t.Errorf("local: %+v, want direct", local)
	}
}

func TestDecide_TheBoostPreparesOnlyASourceWithACentre(t *testing.T) {
	t.Parallel()
	picture := video("h264", "High", "yuv420p", "")
	surround := file("mp4", picture, audio("aac", "LC", 6))
	menu := browser
	menu.Audio = append(menu.Audio, decide.AACMulti)
	got := decide.Decide(decide.Input{Probe: surround, Menu: menu, Boost: true})
	if got.Profile != decide.Remux || !slices.Equal(got.Reasons, []decide.Reason{decide.ReasonDialogueBoost}) {
		t.Errorf("5.1 with the boost: %+v, want remux for dialogue_boost", got)
	}
	stereo := file("mp4", picture, audio("aac", "LC", 2))
	if got := decide.Decide(decide.Input{Probe: stereo, Menu: browser, Boost: true}); got.Kind != decide.Direct {
		t.Errorf("stereo with the boost: %+v, want direct: there is no centre to raise", got)
	}
}

func TestDecide_APersonsChoiceWins(t *testing.T) {
	t.Parallel()
	mkv := file("matroska", video("h264", "High", "yuv420p", ""), audio("truehd", "", 8))
	forced := decide.Decide(decide.Input{Probe: mkv, Menu: browser, Prefer: decide.PreferDirect})
	if got := forced; got.Kind != decide.Direct {
		t.Errorf("prefer direct: %+v, want direct", got)
	}
	mp4 := file("mp4", video("h264", "High", "yuv420p", ""), audio("aac", "LC", 2))
	got := decide.Decide(decide.Input{Probe: mp4, Menu: browser, Prefer: decide.PreferPrepare})
	if got.Kind != decide.HLS || !slices.Equal(got.Reasons, []decide.Reason{decide.ReasonUserChoice}) {
		t.Errorf("prefer prepare: %+v, want hls for user_choice", got)
	}
}

func TestMenu_RefusesAMenuWithoutTheBaselinesOrWithAnUnknownName(t *testing.T) {
	t.Parallel()
	if err := browser.Validate(); err != nil {
		t.Errorf("a browser's menu: %v", err)
	}
	for name, menu := range map[string]decide.Menu{
		"no h264-sdr":      {Video: []decide.Video{decide.HEVCSDR}, Audio: []decide.Audio{decide.AACStereo}},
		"no aac-stereo":    {Video: []decide.Video{decide.H264SDR}, Audio: []decide.Audio{decide.Opus}},
		"an unknown codec": {Video: []decide.Video{decide.H264SDR, "vp9"}, Audio: []decide.Audio{decide.AACStereo}},
		"an unknown container": {
			Video: []decide.Video{decide.H264SDR}, Audio: []decide.Audio{decide.AACStereo},
			Containers: []decide.Container{"avi"},
		},
	} {
		if err := menu.Validate(); !errors.Is(err, decide.ErrMenu) {
			t.Errorf("%s: %v, want ErrMenu", name, err)
		}
	}
}
