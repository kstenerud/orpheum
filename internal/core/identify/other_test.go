// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// recordedVideoRecord is videoRecord with the time its container says it was recorded at.
func recordedVideoRecord(path string, recordedAt time.Time) nodeproto.ScanRecord {
	record := videoRecord(path)
	record.Observed.Probe.Format.RecordedAtMs = recordedAt.UnixMilli()
	return record
}

// otherVideoOf is what the pass derived of an other video: its date, where it came from, its file's name.
func (lib *testLibrary) otherVideoOf(titleID int64) sqlcgen.OtherVideo {
	lib.t.Helper()
	row, err := lib.database.Read().GetOtherVideo(lib.t.Context(), titleID)
	if err != nil {
		lib.t.Fatalf("title %d's other video: %v", titleID, err)
	}
	return row
}

// An other library files every video as a title of its own, named by its file and dated by its
// container, its name, its folder or its file's time, and never matches one (D349 §3, §4, §10).
func TestPass_AnOtherLibraryFilesEachVideoAsATitleOfItsOwnNeverMatched(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "other", "nfo")
	tagged := time.Date(2018, 3, 4, 5, 6, 7, 0, time.UTC)
	report := lib.scan(
		recordedVideoRecord("Wedding/ceremony.mp4", tagged),
		videoRecord("2019-07-14 Beach day.mp4"),
		videoRecord("2021-05 Birthday/clip.mp4"),
		videoRecord("Trip/clip.mp4"),
		videoRecord("Trip/Behind the scenes/trailer.mkv"),
		videoRecord("Phone/VID_20200102_101112.mp4"),
		videoRecord("Old/home.rm"),
	)
	files := lib.titlesByPath()
	want := map[string]string{
		"Wedding/ceremony.mp4":               "ceremony|tag|" + strconv.FormatInt(tagged.UnixMilli(), 10),
		"2019-07-14 Beach day.mp4":           "Beach day|name|" + msOf(2019, 7, 14, 0, 0, 0),
		"2021-05 Birthday/clip.mp4":          "clip|name|" + msOf(2021, 5, 1, 0, 0, 0),
		"Trip/clip.mp4":                      "clip|file|1",
		"Trip/Behind the scenes/trailer.mkv": "trailer|file|1",
		"Phone/VID_20200102_101112.mp4":      "VID 20200102 101112|name|" + msOf(2020, 1, 2, 10, 11, 12),
		"Old/home.rm":                        "home|file|1",
	}
	titles := map[int64]bool{}
	for path, wanted := range want {
		file := files[path]
		other := lib.otherVideoOf(file.WorkID)
		got := fmt.Sprintf("%s|%s|%d", titleName(file), other.RecordedFrom, other.RecordedAt)
		isOther := file.Kind == identify.KindOther && file.Filing == identify.FilingListed &&
			ptr.ValueOrZero(file.IdentificationState) == identify.IdentificationIdentified
		if got != wanted || !isOther {
			t.Errorf("%s: %s, a %s %s %s; want %s, an identified other video, listed", path, got, file.Kind,
				ptr.ValueOrZero(file.IdentificationState), file.Filing, wanted)
		}
		titles[file.WorkID] = true
	}
	if len(titles) != len(want) {
		t.Errorf("%d titles for %d videos; want each its own", len(titles), len(want))
	}
	if len(report.AwaitingMatch) != 0 || len(lib.provider.Calls) != 0 {
		t.Errorf("awaiting a match %v, provider asked %v; want none: an other video is never matched online",
			report.AwaitingMatch, lib.provider.Calls)
	}
}

func msOf(year int, month time.Month, day, hour, minute, second int) string {
	return strconv.FormatInt(time.Date(year, month, day, hour, minute, second, 0, time.UTC).UnixMilli(), 10)
}

// An other video renamed or moved stays the title it was, with its date and name again from its new
// path, and a pass over it again writes nothing (D349 §3; D303, DL9).
func TestPass_AnOtherVideoMovedStaysItsTitleAndAPassAgainWritesNothing(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "other", "nfo")
	lib.scan(videoRecord("Holiday/2019-07-14 Beach.mp4"))
	before := lib.titlesByPath()["Holiday/2019-07-14 Beach.mp4"]
	lib.scan(videoMoved("Holiday/2019-07-14 Beach.mp4", "2019/2019-08-01 Beach at dusk.mp4"))
	after := lib.titlesByPath()["2019/2019-08-01 Beach at dusk.mp4"]
	if after.WorkID != before.WorkID || titleName(after) != "Beach at dusk" {
		t.Errorf("moved: title %d %q; want title %d named again by its new name", after.WorkID, titleName(after),
			before.WorkID)
	}
	if other := lib.otherVideoOf(after.WorkID); other.FileName != "2019-08-01 Beach at dusk.mp4" ||
		strconv.FormatInt(other.RecordedAt, 10) != msOf(2019, 8, 1, 0, 0, 0) {
		t.Errorf("moved: %+v; want its new file's name and date", other)
	}
	rowsBefore, err := lib.database.RowsChanged(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lib.pass.IdentifyLibrary(t.Context(), lib.libraryID); err != nil {
		t.Fatal(err)
	}
	if rowsAfter, err := lib.database.RowsChanged(t.Context()); err != nil || rowsAfter != rowsBefore {
		t.Errorf("a pass over an unchanged other library changed %d rows (%v); want none", rowsAfter-rowsBefore, err)
	}
}

// An admin's correction of an other video's date holds over its file's and says it is theirs, and no
// person can match one online (D349 §4, §10).
func TestChooser_AnOtherVideosDateIsCorrectedAndItIsNeverMatched(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "other", "nfo")
	lib.scan(videoRecord("2019-07-14 Beach.mp4"))
	titleID := lib.titlesByPath()["2019-07-14 Beach.mp4"].WorkID
	person := storetest.InsertUser(t, lib.database, "alice", "admin")
	corrected := msOf(2019, 7, 15, 9, 0, 0)
	if err := lib.chooser.SetOverride(t.Context(), titleID, "recorded_at", corrected, person); err != nil {
		t.Fatal(err)
	}
	if other := lib.otherVideoOf(titleID); other.RecordedFrom != "user" ||
		strconv.FormatInt(other.RecordedAt, 10) != corrected {
		t.Errorf("corrected: %+v; want the person's date, said to be theirs", other)
	}
	err := lib.chooser.SetOverride(t.Context(), titleID, "year", "2019", person)
	if !errors.Is(err, identify.ErrUnknownField) {
		t.Errorf("an other video's year: %v; want ErrUnknownField", err)
	}
	if err := lib.chooser.Choose(t.Context(), titleID, "tmdb", "9019"); !errors.Is(err, identify.ErrNeverMatched) {
		t.Errorf("an other video matched by a person: %v; want ErrNeverMatched", err)
	}
	if err := lib.chooser.ClearOverride(t.Context(), titleID, "recorded_at"); err != nil {
		t.Fatal(err)
	}
	if other := lib.otherVideoOf(titleID); other.RecordedFrom != "name" {
		t.Errorf("the correction removed: %+v; want the date its name gives", other)
	}
}

func TestOtherName_IsTheFilesNameWithoutItsDate(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"2019-07-14 Beach day":       "Beach day|2019-07-14",
		"Beach_day_2019.07.14":       "Beach day|2019-07-14",
		"20190714 - Mia's 5th":       "Mia's 5th|2019-07-14",
		"VID_20190714_103102":        "VID 20190714 103102|2019-07-14",
		"2019-07 Holiday in Crete":   "Holiday in Crete|2019-07-01",
		"Concert 1999":               "Concert 1999|",
		"lecture.part.2":             "lecture part 2|",
		"2019-02-31 Not a day":       "2019-02-31 Not a day|",
		"Mitschnitt 2003-05-06 Köln": "Mitschnitt Köln|2003-05-06",
	}
	for stem, want := range cases {
		name, date, hasDate := identify.OtherName(stem)
		day := ""
		if hasDate {
			day = date.Format(time.DateOnly)
		}
		if got := name + "|" + day; got != want {
			t.Errorf("%q: %s, want %s", stem, got, want)
		}
	}
}

// A change of locale keys every other video's file name again, with no scan, as it does every name
// (D47; D349 §4).
func TestRefreshSortAndSearchKeys_KeysAnOtherVideosFileNameInTheNewLocale(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "other", "nfo")
	lib.scan(videoRecord("Ödet.mp4"), videoRecord("Destiny.mp4"))
	settingstest.Set(t, lib.settings, settings.SortLocale, `"sv"`)
	if written, err := lib.pass.RefreshSortAndSearchKeys(t.Context()); err != nil || written == 0 {
		t.Fatalf("RefreshSortAndSearchKeys = %d, %v; want what Swedish keys otherwise keyed again", written, err)
	}
	swedish, err := textindex.NewSorter("sv")
	if err != nil {
		t.Fatal(err)
	}
	for path, file := range lib.titlesByPath() {
		if other := lib.otherVideoOf(file.WorkID); !bytes.Equal(other.FileNameKey, swedish.Key(path)) {
			t.Errorf("%s: its file name is not keyed in Swedish", path)
		}
	}
}
