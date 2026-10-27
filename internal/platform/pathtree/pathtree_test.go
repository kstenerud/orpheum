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

func TestDeepestHolding_IsTheDeepestDirectoryHoldingACleanPathAndNoneForAnother(t *testing.T) {
	t.Parallel()
	roots := []string{"/tank/media", "/tank/media/video", "/home/synctest"}
	for _, each := range []struct {
		path, want string
		isHeld     bool
	}{
		{"/tank/media", "/tank/media", true},
		{"/tank/media/video/movies", "/tank/media/video", true},
		{"/tank/media/music", "/tank/media", true},
		{"/home/synctest/movies", "/home/synctest", true},
		{"/tank/mediaX", "", false},
		{"/etc", "", false},
		{"/tank/media/../../etc", "", false},
		{"/tank/media/video/..", "", false},
		{"/tank/media/./video", "", false},
		{"/tank/media//video", "", false},
		{"/tank/media/video/", "", false},
		{"tank/media", "", false},
		{"", "", false},
	} {
		got, isHeld := pathtree.DeepestHolding(each.path, roots)
		if got != each.want || isHeld != each.isHeld {
			t.Errorf("DeepestHolding(%q) = %q, %v; want %q, %v", each.path, got, isHeld, each.want, each.isHeld)
		}
	}
}
