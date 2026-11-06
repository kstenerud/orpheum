// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A stereo AAC film in MP4 its device plays as it is.
func ownPlayableFilm() (nodeproto.Probe, decide.PlayableFormats) {
	probe := nodeproto.Probe{
		Format: nodeproto.ProbeFormat{FormatNames: []string{"mp4"}},
		Streams: []nodeproto.ProbeStream{
			{
				StreamIndex: 0, Kind: "video", Codec: "h264", CodecProfile: "High",
				Video: nodeproto.ProbeVideo{PixelFormat: "yuv420p"},
			},
			{
				StreamIndex: 1, Kind: "audio", Codec: "aac", CodecProfile: "LC",
				Audio: nodeproto.ProbeAudio{ChannelCount: 2},
			},
		},
	}
	formats := decide.PlayableFormats{
		VideoCodecs: []decide.Video{decide.H264SDR},
		AudioCodecs: []decide.Audio{decide.AACStereo},
		Containers:  []decide.Container{decide.MP4},
	}
	return probe, formats
}

// A playback whose person levels, of a sound levelling raises, leaves its own file for its ladder
// where the device's sound through Web Audio lags past D436's bound; a lowering, levelling off, a
// sound not measured, or a device that raises itself, each play the film as it is (D436).
func TestDecisionFor_ARaiseIsTheServersWhereWebAudioIsLate(t *testing.T) {
	t.Parallel()
	probe, formats := ownPlayableFilm()
	raised := decide.Levelling{IsMeasured: true, GainDB: 4.5}
	cases := []struct {
		name       string
		isLate     bool
		isLevelled bool
		levelling  decide.Levelling
		want       decide.Delivery
	}{
		{"a raise, Web Audio late", true, true, raised, decide.HLS},
		{"a raise, Web Audio on time", false, true, raised, decide.Direct},
		{"a lowering, Web Audio late", true, true, decide.Levelling{IsMeasured: true, GainDB: -11.6}, decide.Direct},
		{"a raise, levelling off", true, false, raised, decide.Direct},
		{"not measured, Web Audio late", true, true, decide.Levelling{}, decide.Direct},
	}
	for _, each := range cases {
		request := Request{PlayableFormats: formats, IsWebAudioLate: each.isLate}
		started := Started{
			Preferences: preferences.Preferences{Switches: preferences.Switches{IsLevellingOn: each.isLevelled}},
			Levelling:   each.levelling,
		}
		if got := decisionFor(request, probe, started); got.Delivery != each.want {
			t.Errorf("%s: %+v, want %s", each.name, got, each.want)
		}
	}
}

// A film converted only for a raise the device leaves to the server, finding no ladder, plays as it
// is, unraised, rather than refused; one converted for anything else is left to be refused, and one
// on its ladder stays there (D436).
func TestDirectForARaiseOffTheLadder_PlaysTheFilmRatherThanRefuseIt(t *testing.T) {
	t.Parallel()
	onlyRaise := decide.Decision{
		Delivery: decide.HLS, Profile: decide.Remux, NotDirectReasons: []decide.Reason{decide.ReasonLevellingRaise},
	}
	if got := directForARaiseOffTheLadder(onlyRaise, false); got.Delivery != decide.Direct {
		t.Errorf("only a raise, no ladder: %+v, want direct", got)
	}
	if got := directForARaiseOffTheLadder(onlyRaise, true); got.Delivery != decide.HLS {
		t.Errorf("only a raise, on its ladder: %+v, want it left on the ladder", got)
	}
	codec := decide.Decision{
		Delivery: decide.HLS, Profile: decide.Remux,
		NotDirectReasons: []decide.Reason{decide.ReasonAudioCodec, decide.ReasonLevellingRaise},
	}
	if got := directForARaiseOffTheLadder(codec, false); got.Delivery != decide.HLS {
		t.Errorf("a codec the device cannot play as well: %+v, want it left converted", got)
	}
}
