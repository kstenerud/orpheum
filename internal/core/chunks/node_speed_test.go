// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// triedAt says every node encodes on its processor, fast enough to offer its ladder, and that its
// speed trials measured these.
type triedAt []nodeproto.SpeedTrial

func (triedAt) EncodesOnAGPU(int64) bool { return false }

func (triedAt) EncodeSpeedPercent(int64) int { return 0 }

func (trials triedAt) SpeedTrials(int64) []nodeproto.SpeedTrial { return trials }

// kindTried is a kind's two trials, its top quality, 8 Mb/s at its own height, made at topPercent
// of real time, and the lowest, 400k at 360p, at lowestPercent, as a node's trials say them.
func kindTried(codec string, bitDepth, height, topPercent, lowestPercent int) []nodeproto.SpeedTrial {
	return []nodeproto.SpeedTrial{
		{
			Codec: codec, BitDepth: bitDepth, Height: height, MadeKbps: 8000, MadeHeight: height,
			SpeedPercent: topPercent, DecodedOn: "cpu",
		},
		{
			Codec: codec, BitDepth: bitDepth, Height: height, MadeKbps: 400, MadeHeight: 360,
			SpeedPercent: lowestPercent, DecodedOn: "cpu",
		},
	}
}

// A start is held to the highest quality its node's trials of the film's kind expect it to make at
// three times real time or faster, a quality between the two measured judged by the time its pixels
// put on the line between them, the lowest where none is; the film's own picture copied, which
// nothing is made of, is not held; a fast node, or one whose trials measured nothing, starts as
// before; and the hold never raises a start the link or the upload put lower, the upload's ceiling
// kept where it held the start first (start-quality-by-node-speed.md §2).
func TestStartOnLadder_AStartIsHeldToAQualityItsNodeMakesFast(t *testing.T) {
	t.Parallel()
	for _, start := range []struct {
		name          string
		trials        triedAt
		linkKbps      int64
		atMostKbps    int64
		isDecodable   bool
		want          string
		bound         StartBound
		speedPercent  int
		isHeld        bool
		ceilingKbps   int64
		averageAtMost int64
	}{
		{name: "no trials", want: "q4000k1080p", bound: StartBoundTop},
		{
			name: "a failed and an untried kind", want: "q4000k1080p", bound: StartBoundTop,
			trials: triedAt{
				{Codec: "h264", BitDepth: 8, Height: 1080, MadeKbps: 8000, MadeHeight: 1080, Failure: "broken"},
				{Codec: "h264", BitDepth: 8, Height: 1080, MadeKbps: 400, MadeHeight: 360, SpeedPercent: 900},
				{Codec: "vp9", BitDepth: 8, Height: 1080, NotTried: "no encoder"},
			},
		},
		{
			name: "fast", trials: kindTried("h264", 8, 1080, 1000, 2000), want: "q4000k1080p",
			bound: StartBoundTop, speedPercent: 1000,
		},
		// The top at 2x and the lowest at 6x: 720p's pixels cost 0.29 s a second, 342 %.
		{
			name: "slow at the top", trials: kindTried("h264", 8, 1080, 200, 600), want: "q3000k720p",
			bound: StartBoundNodeSpeed, speedPercent: 342,
		},
		{
			name: "copied", trials: kindTried("h264", 8, 1080, 200, 600), isDecodable: true, want: "picture",
			bound: StartBoundTop,
		},
		{
			name: "slow at every quality", trials: kindTried("h264", 8, 1080, 100, 200), want: "q400k360p",
			bound: StartBoundNodeSpeed, speedPercent: 200,
		},
		{
			name: "the link lower", trials: kindTried("h264", 8, 1080, 200, 600), linkKbps: 2000,
			want: "q1200k480p", bound: StartBoundLink, speedPercent: 502,
		},
		{
			name: "the upload lower", trials: kindTried("h264", 8, 1080, 200, 600), linkKbps: 20_000,
			atMostKbps: 1500, want: "q1200k480p", bound: StartBoundStartAtMost, speedPercent: 502,
			isHeld: true, ceilingKbps: 1296,
		},
		{
			name: "the node lower than the upload", trials: kindTried("h264", 8, 1080, 100, 200), linkKbps: 20_000,
			atMostKbps: 1500, want: "q400k360p", bound: StartBoundNodeSpeed, speedPercent: 200,
			isHeld: true, ceilingKbps: 1296,
		},
	} {
		service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
		service.encoders = start.trials
		offer, err := service.StartOnLadder(t.Context(), Playback{
			FileID: 1, IsSourcePictureDecodable: start.isDecodable, HasStartingAudio: true,
			StartAtMostKbps: start.atMostKbps,
		}, 95_000, start.linkKbps)
		if err != nil {
			t.Fatal(err)
		}
		if offer.StreamName != start.want || offer.StartBound != start.bound ||
			offer.ExpectedSpeedPercent != start.speedPercent || offer.IsHeldByUpload != start.isHeld ||
			offer.UploadCeilingKbps != start.ceilingKbps {
			t.Errorf("%s: the offer = %+v; want %s, bound by %s, expected at %d %%, held by the upload %t "+
				"under %d kb/s", start.name, offer, start.want, start.bound, start.speedPercent, start.isHeld,
				start.ceilingKbps)
		}
	}
}

// A film is judged by its own kind's trials, else by the nearest kind tried no easier, the same
// codec at the same or a greater bit depth and size, smallest first; else by the kind its node made
// slowest of all; a kind whose either trial measured nothing is not one (start-quality-by-node-speed.md
// §2). Each kind's top here is measured at its own speed, which names it.
func TestTrialLineFor_TakesTheNearestKindNoEasierElseTheSlowest(t *testing.T) {
	t.Parallel()
	var trials []nodeproto.SpeedTrial
	for _, kind := range []struct {
		codec               string
		bitDepth, height    int
		topPercent, lowestP int
	}{
		{"h264", 8, 1080, 800, 2000},
		{"h264", 10, 1080, 500, 2000},
		{"hevc", 10, 1080, 300, 2000},
		{"hevc", 10, 2160, 100, 2000},
		{"av1", 10, 1080, 400, 2000},
	} {
		trials = append(trials, kindTried(kind.codec, kind.bitDepth, kind.height, kind.topPercent, kind.lowestP)...)
	}
	trials = append(trials, nodeproto.SpeedTrial{
		Codec: "vp9", BitDepth: 8, Height: 1080, MadeKbps: 8000, MadeHeight: 1080, SpeedPercent: 50,
	}, nodeproto.SpeedTrial{
		Codec: "vp9", BitDepth: 8, Height: 1080, MadeKbps: 400, MadeHeight: 360, Failure: "broken",
	})
	picture := func(codec, pixelFormat string, width, height int) nodeproto.ProbeStream {
		return nodeproto.ProbeStream{
			Kind: "video", Codec: codec,
			Video: nodeproto.ProbeVideo{Width: width, Height: height, PixelFormat: pixelFormat},
		}
	}
	for _, film := range []struct {
		name       string
		picture    nodeproto.ProbeStream
		topPercent int
	}{
		{"H.264 8-bit 1080p", picture("h264", "yuv420p", 1920, 1080), 800},
		{"H.264 8-bit 720p", picture("h264", "yuv420p", 1280, 720), 800},
		{"H.264 pixel format not known", picture("h264", "", 1920, 1080), 800},
		{"H.264 10-bit 1080p", picture("h264", "yuv420p10le", 1920, 1080), 500},
		{"H.264 12-bit, none deeper", picture("h264", "yuv420p12le", 1920, 1080), 100},
		{"HEVC 10-bit 1080p", picture("hevc", "yuv420p10le", 1920, 1080), 300},
		{"HEVC 8-bit 1080p, the 10-bit's", picture("hevc", "yuv420p", 1920, 1080), 300},
		{"HEVC 10-bit wider than 1080p's", picture("hevc", "yuv420p10le", 2560, 1080), 100},
		{"HEVC 10-bit 2160p", picture("hevc", "yuv420p10le", 3840, 2160), 100},
		{"H.264 2160p, none so large", picture("h264", "yuv420p", 3840, 2160), 100},
		{"VP9, its trial failed", picture("vp9", "yuv420p", 1920, 1080), 100},
		{"MPEG-2, not tried", picture("mpeg2video", "yuv420p", 1920, 1080), 100},
	} {
		line, isMeasured := trialLineFor(trials, film.picture)
		if !isMeasured || line.topCost != 100/float64(film.topPercent) {
			t.Errorf("%s is judged by %+v (measured %t); want the kind whose top is %d %%", film.name, line,
				isMeasured, film.topPercent)
		}
	}
	if _, isMeasured := trialLineFor(nil, picture("h264", "yuv420p", 1920, 1080)); isMeasured {
		t.Error("with no trials a film is judged; want none, a start as before")
	}
}
