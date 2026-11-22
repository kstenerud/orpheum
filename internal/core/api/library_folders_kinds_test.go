// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// videosOf is a folder's videos as a test compares them: each one's file, name, title, length and
// resume point.
func videosOf(folder api.FolderOutput) string {
	var said string
	for _, video := range folder.Body.Videos {
		title := "none"
		if video.Title != nil {
			year := "-"
			if video.Title.Year != nil {
				year = fmt.Sprint(*video.Title.Year)
			}
			title = fmt.Sprintf("%d %s %q %s %s", video.Title.ID, video.Title.Kind, video.Title.Name, year,
				video.Title.IdentificationState)
		}
		said += fmt.Sprintf("[%d %q (%s) %d %d]", video.FileID, video.Name, title, video.DurationMs,
			video.ResumePositionMs)
	}
	return said
}

// Every kind of library's folder lists its video files, matched or not, each with its file, its name,
// the title it is filed as, its length and where the person resumes it; an other library's lists its
// titles too, each one's file at the same index (D539 §7).
func TestBrowse_EveryKindOfLibrarysFolderListsItsVideoFiles(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveOtherVideos(t)
	librarytest.LibraryOn(t, harness.database, 1,
		librarytest.Library{Name: "Shows", Kind: "series", MetadataSource: "nfo", Root: "/s"})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 3, LibraryID: 1, NodeID: 1, Path: "/m/a.mkv", PayloadHash: []byte{0x03}, Filing: "unidentified",
	})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 50, LibraryID: 3, NodeID: 1, Path: "/s/x/a.mkv", PayloadHash: []byte{0x50}, Filing: "unidentified",
	})
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: 1, DurationMs: 600000})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO play_position (user_id, file_id, position_ms, report_seq, updated_at, position_read_at)
		 SELECT id, 1, 120000, 1, 1, 1 FROM user`)
	for _, check := range []struct{ query, subfolders, titles, videos string }{
		{ // a film library's top: its unmatched file by its name, before the film by its title's
			"/libraries/1/folders?sort=name", "[{BB BB 1}]", "",
			`[3 "a.mkv" (none) 0 0][1 "Metropolis (1927).mkv" (10 film "Metropolis" 1995 identified) 600000 120000]`,
		},
		{
			"/libraries/1/folders?sort=name&order=descending", "[{BB BB 1}]", "",
			`[1 "Metropolis (1927).mkv" (10 film "Metropolis" 1995 identified) 600000 120000][3 "a.mkv" (none) 0 0]`,
		},
		{
			"/libraries/1/folders?sort=fileName&folder=BB", "[]", "",
			`[2 "S01E01.mkv" (21 episode "Pilot" 2008 identified) 0 0]`,
		},
		{"/libraries/3/folders?sort=added", "[{x x 1}]", "", ""},
		{"/libraries/3/folders?sort=recorded&folder=x", "[]", "", `[50 "a.mkv" (none) 0 0]`},
		{
			"/libraries/2/folders?sort=name&folder=Trip", "[]", "clip",
			`[42 "clip.mp4" (42 other "clip" - identified) 0 0]`,
		},
	} {
		var folder api.FolderOutput
		harness.readJSON(t, check.query, admin, &folder.Body)
		var titles string
		for _, title := range folder.Body.Titles {
			titles += title.Name
		}
		if got := fmt.Sprint(folder.Body.Subfolders); got != check.subfolders || titles != check.titles ||
			videosOf(folder) != check.videos {
			t.Errorf("%s = folders %s, titles %q, videos %s; want %s, %q, %s", check.query, got, titles,
				videosOf(folder), check.subfolders, check.titles, check.videos)
		}
	}
}
