// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import "testing"

// A player waiting on the picture's keyframes makes the sweep's queued full read an index read, which
// the full read follows; a running read covers only a request asking what it asks (D231 §8, D269 §5).
func TestArgsMerge_APlayersIndexReadWinsOverAQueuedFullReadAndARunningOneCoversOnlyItsOwn(t *testing.T) {
	t.Parallel()
	const fullRead, indexRead = "{}", `{"isFromIndexOnly":true}`
	merge := ArgsMerge()
	for _, test := range []struct{ queued, requested, want string }{
		{fullRead, indexRead, indexRead},
		{indexRead, fullRead, indexRead},
		{fullRead, fullRead, fullRead},
		{"", fullRead, fullRead},
	} {
		merged, err := merge.Merge(test.queued, test.requested)
		if err != nil || merged != test.want {
			t.Errorf("Merge(%q, %q) = %q, %v; want %q", test.queued, test.requested, merged, err, test.want)
		}
	}
	for _, test := range []struct {
		running, requested string
		want               bool
	}{
		{fullRead, fullRead, true},
		{indexRead, indexRead, true},
		{fullRead, indexRead, false},
		{indexRead, fullRead, false},
	} {
		covers, err := merge.Covers(test.running, test.requested)
		if err != nil || covers != test.want {
			t.Errorf("Covers(%q, %q) = %t, %v; want %t", test.running, test.requested, covers, err, test.want)
		}
	}
}
