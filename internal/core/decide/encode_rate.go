// SPDX-License-Identifier: AGPL-3.0-or-later

package decide

import "github.com/kstenerud/orpheum/internal/nodeproto"

// encodeKbpsMost is the h264 profile's rate, its most: a 1080p picture's (cut-a §6).
const encodeKbpsMost = 8000

// EncodeKbps is the average rate an h264 encode of probe's picture is made at (D256): the top of
// Apple's HLS authoring ladder for its height, at most the profile's 8 Mb/s, and never above the
// original's own rate, since an encode is there to be lighter than the original (the owner,
// 2026-10-07). 8 Mb/s of a 640×272 DVD rip made a 5.3 GB copy that stalled every two seconds on a
// 3 Mb/s link (staging, 2026-10-07). The original's rate is its picture's, or the whole file's when
// the container gives no picture's; one under a quarter of its height's rate is taken for a wrong
// figure, as a file cut short reports, and ignored.
func EncodeKbps(probe nodeproto.Probe) int {
	picture, found := MainVideo(probe)
	if !found {
		return encodeKbpsMost
	}
	kbps := encodeKbpsForHeight(picture.Video.Height)
	originalKbps := int(picture.BitRateBps / 1000)
	if originalKbps <= 0 {
		originalKbps = int(probe.Format.BitRateBps / 1000)
	}
	if originalKbps >= kbps/4 {
		kbps = min(kbps, originalKbps)
	}
	return kbps
}

// TwinKbps is the average rate of the twin, the H.264 copy of probe's picture at its own height that
// a device which cannot play the picture gets in its place (D323 §1): the original's own rate, raised
// for a codec that packs more into a bit than H.264, by half for HEVC and VP9 and twice for AV1, and
// at most its height's on Apple's HLS authoring ladder, the profile's 8 Mb/s at the most. The one
// encode that may carry more than the original, which EncodeKbps never does: at the original's rate
// an H.264 copy of an HEVC picture is visibly worse, and the owner asked for the same quality.
func TwinKbps(probe nodeproto.Probe) int {
	picture, found := MainVideo(probe)
	if !found {
		return encodeKbpsMost
	}
	kbps := encodeKbpsForHeight(picture.Video.Height)
	originalKbps := int(picture.BitRateBps / 1000)
	if originalKbps <= 0 {
		originalKbps = int(probe.Format.BitRateBps / 1000)
	}
	if originalKbps < kbps/4 {
		return kbps
	}
	return min(kbps, originalKbps*twinRatePercent(picture.Codec)/100)
}

// twinRatePercent is how much more rate an H.264 copy takes than an original in codec does for the
// same picture, in percent.
func twinRatePercent(codec string) int {
	switch codec {
	case "hevc", "vp9":
		return 150
	case "av1":
		return 200
	default:
		return 100
	}
}

// encodeKbpsForHeight is the top of Apple's HLS authoring ladder for a picture height lines tall, at
// most the profile's rate; an unknown height gets the profile's rate.
func encodeKbpsForHeight(height int) int {
	switch {
	case height <= 0:
		return encodeKbpsMost
	case height <= 288:
		return 1000
	case height <= 360:
		return 1100
	case height <= 480:
		return 1600
	case height <= 576:
		return 2000
	case height <= 720:
		return 4500
	}
	return encodeKbpsMost
}
