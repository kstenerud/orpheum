// SPDX-License-Identifier: AGPL-3.0-or-later

package problemreport

import (
	"strconv"
	"testing"
)

func TestNames_NoTwoValuesShareATag(t *testing.T) {
	t.Parallel()
	// 20,000 titles at six hex digits collide about a dozen times: each must be lengthened.
	names := newNames([]byte("salt"), nil)
	byTag := map[string]string{}
	lengthened := 0
	for at := range 20000 {
		value := "title " + strconv.Itoa(at)
		tag := names.name(KindItem, value, false).tag
		if earlier, ok := byTag[tag]; ok {
			t.Fatalf("%q and %q share the tag %s", earlier, value, tag)
		}
		byTag[tag] = value
		if len(tag) > len("item:")+tagHexStart {
			lengthened++
		}
		if again := names.name(KindItem, value, false).tag; again != tag {
			t.Fatalf("%q has two tags, %s and %s", value, tag, again)
		}
	}
	if lengthened == 0 {
		t.Error("no tag was lengthened: the test no longer makes a collision")
	}
}
