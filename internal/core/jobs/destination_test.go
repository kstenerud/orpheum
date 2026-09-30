// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"strconv"
	"testing"
)

func TestDestinations_ANameSaysWhatItIsAndReadsAsAPersonSaysIt(t *testing.T) {
	t.Parallel()
	nodeName := func(nodeID int64) string { return "node-" + strconv.FormatInt(nodeID, 10) }
	for _, check := range []struct {
		destination Destination
		parts       DestinationParts
		words       string
	}{
		{DestinationTMDBAPI, DestinationParts{}, "TMDB"},
		{DestinationTMDBImages, DestinationParts{}, "TMDB's image server"},
		{DestinationOpenSubtitles, DestinationParts{}, "OpenSubtitles"},
		{DestinationCoreStorage, DestinationParts{}, "the server's own disk"},
		{DestinationAnyNode, DestinationParts{}, "a node"},
		{NodeDestination(3), DestinationParts{NodeID: 3, IsNode: true}, "node node-3"},
		{
			RootDestination(3, "/media/films:new"),
			DestinationParts{NodeID: 3, Path: "/media/films:new", IsRoot: true},
			"/media/films:new on node node-3",
		},
		{
			PreparedDirectoryDestination(4),
			DestinationParts{NodeID: 4, IsPreparedDirectory: true},
			"the prepared directory on node node-4",
		},
	} {
		if parts := PartsOf(check.destination); parts != check.parts {
			t.Errorf("%q reads as %+v, want %+v", check.destination, parts, check.parts)
		}
		if words := DescribeDestination(check.destination, nodeName); words != check.words {
			t.Errorf("%q is described as %q, want %q", check.destination, words, check.words)
		}
	}
}
