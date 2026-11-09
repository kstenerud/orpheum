// SPDX-License-Identifier: AGPL-3.0-or-later

package jobs

import "testing"

// A destination's name is camelCase, as every value the API says is (the owner, 2026-11-09): the
// outside hosts and the core's own, and the word before a node's destination's number.
func TestNames_TheDestinationsTheAPISaysAreCamelCase(t *testing.T) {
	t.Parallel()
	for _, each := range []struct {
		destination Destination
		want        string
	}{
		{DestinationTMDBAPI, "tmdbApi"},
		{DestinationTMDBImages, "tmdbImages"},
		{DestinationOpenSubtitles, "opensubtitles"},
		{DestinationCoreStorage, "coreStorage"},
		{DestinationAnyNode, "anyNode"},
		{NodeDestination(3), "node:3"},
		{RootDestination(3, "/media"), "root:3:/media"},
		{QuickPassDestination(3, "sda"), "quickPass:3:sda"},
		{LastReaderDestination(3, "sda"), "lastReader:3:sda"},
	} {
		if string(each.destination) != each.want {
			t.Errorf("the destination is named %q; want %q", each.destination, each.want)
		}
	}
}
