// SPDX-License-Identifier: AGPL-3.0-or-later

package pathtree_test

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/pathtree"
)

func TestIsAtOrUnder_CutsOnlyAtASeparator(t *testing.T) {
	t.Parallel()
	for _, each := range []struct {
		path, directory string
		want            bool
	}{
		{"/media/films", "/media/films", true},
		{"/media/films/Wing.mkv", "/media/films", true},
		{"/media/films/Wing.mkv", "/media/films/", true},
		{"/media/films/Wing.mkv", "/media/film", false},
		{"/media", "/media/films", false},
		{"/anything", "/", true},
		{"/", "/", true},
	} {
		if got := pathtree.IsAtOrUnder(each.path, each.directory); got != each.want {
			t.Errorf("IsAtOrUnder(%q, %q) = %v, want %v", each.path, each.directory, got, each.want)
		}
	}
}
