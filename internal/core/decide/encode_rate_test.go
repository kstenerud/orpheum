// SPDX-License-Identifier: GPL-3.0-or-later

package decide_test

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// pictureProbe is a file of fileKbps whose picture is height lines tall at pictureKbps, 0 for unknown.
func pictureProbe(height int, pictureKbps, fileKbps int64) nodeproto.Probe {
	return nodeproto.Probe{
		Format: nodeproto.ProbeFormat{BitRateBps: fileKbps * 1000},
		Streams: []nodeproto.ProbeStream{{
			Kind: "video", BitRateBps: pictureKbps * 1000, Video: nodeproto.ProbeVideo{Height: height},
		}},
	}
}

// An encode's rate follows its picture's height, at most the profile's 8 Mb/s, and never above the
// original's own: the picture's, or the file's when the container gives none. An original rate under
// a quarter of its height's is a wrong figure, ignored; an unknown height gets the profile's rate
// (D256).
func TestEncodeKbps_FollowsThePicturesHeightNeverAboveTheOriginal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		what  string
		probe nodeproto.Probe
		kbps  int
	}{
		{"an unknown height", pictureProbe(0, 0, 0), 8000},
		{"a 1080p original at 20 Mb/s", pictureProbe(1080, 20_000, 21_000), 8000},
		{"a 1080p original at 5 Mb/s", pictureProbe(1080, 5000, 5600), 5000},
		{"a 1080p file of 3 Mb/s, its picture's unknown", pictureProbe(1080, 0, 3000), 3000},
		{"a 720p original at 10 Mb/s", pictureProbe(720, 10_000, 0), 4500},
		{"a 576p DVD at 6 Mb/s", pictureProbe(576, 6000, 0), 2000},
		{"a 480p original at 1.2 Mb/s", pictureProbe(480, 1200, 0), 1200},
		{"a 272-line DVD rip of 1.6 Mb/s", pictureProbe(272, 0, 1643), 1000},
		{"a 1080p file cut short, said to be 144 kb/s", pictureProbe(1080, 0, 144), 8000},
	} {
		if got := decide.EncodeKbps(tc.probe); got != tc.kbps {
			t.Errorf("%s is encoded at %d kb/s, want %d", tc.what, got, tc.kbps)
		}
	}
}
