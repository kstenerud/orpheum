// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"reflect"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A series is told when three or more of its episodes have no number outside a specials or extras
// folder: a series whose unnumbered files are its DVD extras, or which has one or two, is not
// (D306 §6; staging, 2026-10-16: Star Wren Far Harbour Nine's 148 DVD extras).
func TestUnplacedOf_TellsASeriesWhoseEpisodesAmongItsSeasonsHaveNoNumber(t *testing.T) {
	t.Parallel()
	var rows []sqlcgen.ListUnnumberedEpisodeFilesRow
	add := func(seriesID int64, name string, paths ...string) {
		for _, path := range paths {
			rows = append(rows, sqlcgen.ListUnnumberedEpisodeFilesRow{SeriesID: seriesID, SeriesName: name, Path: path})
		}
	}
	add(1, "Quillwick", "/m/Quillwick/S01/Quillwick - 1x1.avi", "/m/Quillwick/S01/Quillwick - 1x2.avi",
		"/m/Quillwick/S01/Quillwick - 1x3.avi", "/m/Quillwick/Extras/Making of.avi")
	add(2, "Far Harbour Nine", "/m/DS9/Season 1/DS9 - Season 1 - DVD Extras/A.mkv",
		"/m/DS9/Season 1/DS9 - Season 1 - DVD Extras/B.mkv", "/m/DS9/Season 1/DS9 - Season 1 - DVD Extras/C.mkv")
	add(3, "Pilots", "/m/Pilots/Unaired Pilot.mkv", "/m/Pilots/Season 1/Second Pilot.mkv")
	got := unplacedOf(rows)
	want := []UnplacedEpisodes{{SeriesID: 1, SeriesName: "Quillwick", FileCount: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unplacedOf = %+v, want %+v", got, want)
	}
}
