// SPDX-License-Identifier: AGPL-3.0-or-later

package problemreport

import (
	"fmt"
	"strings"
	"testing"
)

// A word as common as "the" leads free text to the titles of its next word, not to every title it
// starts: a household's thousands of titles beginning "The" are not each compared at every "the" of
// a day's log, and the title said is still found.
func TestScrub_LooksUpATitleByItsFirstTwoWords(t *testing.T) {
	t.Parallel()
	scrubber := newScrubber(newNames([]byte("salt"), nil))
	for index := range 1000 {
		scrubber.know(KindItem, fmt.Sprintf("The quiet title %d", index), false)
	}
	scrubber.know(KindItem, "The Long Night", false)
	scrubber.know(KindItem, "Night", false)

	scrubbed := scrubber.scrub("the node closed the stream during the long night, then Night again")
	read := scrubbed.read(Options{Form: FormShareable})
	if strings.Contains(read, "long night") || strings.Contains(read, "Night again") {
		t.Errorf("scrubbed = %q; want the two titles tagged", read)
	}
	if scrubber.compared > 3 {
		t.Errorf("compared %d candidates; want the two titles found and none of the thousand beginning "+
			"\"The quiet\"", scrubber.compared)
	}
}
