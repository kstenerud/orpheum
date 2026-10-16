// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"reflect"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A series is told when five or more of its episodes have no number outside a specials or extras
// folder: a series whose unnumbered files are its DVD extras, by their folder or their own names,
// or which has a few, is not (D306 §6; staging, 2026-10-16: Far Harbour Nine's 148 DVD extras, The
// Pumbletons' special features and Brackenmere's three specials).
func TestUnplacedOf_TellsASeriesWhoseEpisodesAmongItsSeasonsHaveNoNumber(t *testing.T) {
	t.Parallel()
	var rows []sqlcgen.ListUnnumberedEpisodeFilesRow
	add := func(seriesID int64, name string, paths ...string) {
		for _, path := range paths {
			rows = append(rows, sqlcgen.ListUnnumberedEpisodeFilesRow{SeriesID: seriesID, SeriesName: name, Path: path})
		}
	}
	add(1, "Quillwick", "/m/Quillwick/S01/Quillwick - 1x1.avi", "/m/Quillwick/S01/Quillwick - 1x2.avi",
		"/m/Quillwick/S01/Quillwick - 1x3.avi", "/m/Quillwick/S01/Quillwick - 1x4.avi",
		"/m/Quillwick/S01/Quillwick - 1x5.avi", "/m/Quillwick/Extras/Making of.avi")
	add(2, "Far Harbour Nine", "/m/DS9/Season 1/DS9 - Season 1 - DVD Extras/A.mkv",
		"/m/DS9/Season 1/DS9 - Season 1 - DVD Extras/B.mkv", "/m/DS9/Season 1/DS9 - Season 1 - DVD Extras/C.mkv")
	add(3, "Pilots", "/m/Pilots/Unaired Pilot.mkv", "/m/Pilots/Season 1/Second Pilot.mkv",
		"/m/Pilots/Third/Third Pilot.mkv", "/m/Pilots/Fourth Pilot.mkv")
	add(4, "Springfield", "/m/Springfield/S06/Season 6 Special Features - Commercials.mkv",
		"/m/Springfield/S06/Season 6 Special Features - Deleted Scenes.mkv",
		"/m/Springfield/S02/Music Video - Do the Dance.mkv", "/m/Springfield/S02/Music Video - Deep Water.mkv",
		"/m/Springfield/S07/Season 7 Special Features - Deleted Scenes.mkv")
	got := unplacedOf(rows)
	want := []UnplacedEpisodes{{SeriesID: 1, SeriesName: "Quillwick", FileCount: 5}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unplacedOf = %+v, want %+v", got, want)
	}
}
