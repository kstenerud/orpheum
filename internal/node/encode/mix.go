// SPDX-License-Identifier: AGPL-3.0-or-later

package encode

import (
	"strconv"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// The two stereo mixes of a track with a centre, from 5.1 (a 7.1 source is first folded to it).
// The standard mix is ITU-R BS.775's; the boosted one raises the centre, where dialogue is, by
// 6 dB over it. "<" renormalises each side's gains to sum to one, so neither can clip. The
// boost's gain is provisional until the listening test on a television sets it (cut-a §6).
// StandardMix and StereoAsIsMix are what a stereo device hears, so internal/node/readthrough measures
// them (D226 §1).
const (
	StandardMix   = "aformat=channel_layouts=5.1,pan=stereo|FL<FL+0.707*FC+0.707*BL|FR<FR+0.707*FC+0.707*BR"
	boostedMix    = "aformat=channel_layouts=5.1,pan=stereo|FL<FL+1.414*FC+0.707*BL|FR<FR+1.414*FC+0.707*BR"
	StereoAsIsMix = "aformat=channel_layouts=stereo"
)

// MixFilter is the filter a chunk of one of a track's sounds is made with (D247 §10, D217 §3): its
// boosted mix, for a track with a centre, or its standard one, each carrying the track's levelling
// gain after the mix it was measured as (D226 §4). A track without a centre has no boosted mix; it
// is asked for its one mix.
func MixFilter(track nodeproto.SoundTrack, isBoosted bool) string {
	gain := ""
	if track.CarriedGainDB != 0 {
		gain = ",volume=" + strconv.FormatFloat(track.CarriedGainDB, 'f', 1, 64) + "dB"
	}
	switch {
	case track.ChannelCount <= 2:
		return StereoAsIsMix + gain
	case isBoosted:
		return boostedMix + gain
	default:
		return StandardMix + gain
	}
}
