// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/decide"
)

// Under a data cap a film plays from its ladder, a wish, but for its own file chosen where the file
// is within the cap; with no cap the person's preference stands (D488 §4, D491 §4, D493).
func TestDeliveryUnderCap_IsTheLadderButForAnOriginalChosenWithinTheCap(t *testing.T) {
	t.Parallel()
	for _, each := range []struct {
		preferred         decide.Delivery
		capKbps, fileKbps int64
		want              decide.Delivery
	}{
		{"", 0, 2400, ""},
		{decide.Direct, 0, 2400, decide.Direct},
		{"", 1000, 2400, decide.HLS},
		{"", 5000, 2400, decide.HLS},
		{decide.Direct, 1000, 2400, decide.HLS},
		{decide.Direct, 2400, 2400, decide.Direct},
		{decide.Direct, 5000, 0, decide.HLS},
	} {
		if got := deliveryUnderCap(each.preferred, each.capKbps, each.fileKbps); got != each.want {
			t.Errorf("preferring %q under a cap of %d kbit/s, a file of %d: %q; want %q",
				each.preferred, each.capKbps, each.fileKbps, got, each.want)
		}
	}
}

// A start on the ladder keeps under both the upload left and the device's cap, either alone where
// the other sets no bound (D286 §5, D493).
func TestWithinDataCap_IsTheLowerOfTheUploadLeftAndTheCap(t *testing.T) {
	t.Parallel()
	for _, each := range []struct{ atMost, capKbps, want int64 }{
		{0, 0, 0}, {3000, 0, 3000}, {0, 400, 400}, {3000, 400, 400}, {300, 400, 300},
	} {
		if got := withinDataCap(each.atMost, each.capKbps); got != each.want {
			t.Errorf("with %d kbit/s of the upload left under a cap of %d: %d; want %d",
				each.atMost, each.capKbps, got, each.want)
		}
	}
}
