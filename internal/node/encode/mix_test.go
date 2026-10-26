// SPDX-License-Identifier: AGPL-3.0-or-later

package encode_test

import (
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A track with a centre is mixed to stereo by the standard mix, or raised by the boosted one; one
// without has its one mix, asked for boosted or not (D217 §3).
func TestMixFilter_ATrackWithACentreHasAStandardAndABoostedMix(t *testing.T) {
	t.Parallel()
	surround := nodeproto.SoundTrack{AudioTrackIndex: 0, ChannelCount: 6}
	stereo := nodeproto.SoundTrack{AudioTrackIndex: 1, ChannelCount: 2}
	if got := encode.MixFilter(surround, false); got != encode.StandardMix {
		t.Errorf("5.1's standard mix is %q, want %q", got, encode.StandardMix)
	}
	if got := encode.MixFilter(surround, true); !strings.Contains(got, "1.414*FC") {
		t.Errorf("5.1's boosted mix is %q, want the centre raised", got)
	}
	for _, isBoosted := range []bool{false, true} {
		if got := encode.MixFilter(stereo, isBoosted); got != encode.StereoAsIsMix {
			t.Errorf("stereo's mix, boosted %t, is %q, want it as it is", isBoosted, got)
		}
	}
}

// Each mix of a measured track carries its levelling gain after the mix it was measured as; an
// unmeasured one carries none (D226 §4).
func TestMixFilter_EachMixOfAMeasuredTrackCarriesItsLevellingGain(t *testing.T) {
	t.Parallel()
	for _, each := range []struct {
		track     nodeproto.SoundTrack
		isBoosted bool
		want      string
	}{
		{nodeproto.SoundTrack{ChannelCount: 6, CarriedGainDB: 12.7}, false, encode.StandardMix + ",volume=12.7dB"},
		{nodeproto.SoundTrack{ChannelCount: 6, CarriedGainDB: 12.7}, true, ",volume=12.7dB"},
		{nodeproto.SoundTrack{ChannelCount: 2, CarriedGainDB: -3.4}, false, encode.StereoAsIsMix + ",volume=-3.4dB"},
	} {
		if got := encode.MixFilter(each.track, each.isBoosted); !strings.HasSuffix(got, each.want) {
			t.Errorf("%+v's mix, boosted %t, is %q; want it to end %q", each.track, each.isBoosted, got, each.want)
		}
	}
	if got := encode.MixFilter(nodeproto.SoundTrack{ChannelCount: 2}, false); strings.Contains(got, "volume") {
		t.Errorf("an unmeasured track's mix is %q; want no gain", got)
	}
}
