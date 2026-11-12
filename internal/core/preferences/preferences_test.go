// SPDX-License-Identifier: AGPL-3.0-or-later

package preferences

import (
	"fmt"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

func TestMatchingTrack_PrefersTheSameKindThenTheSameTitle(t *testing.T) {
	t.Parallel()
	track := func(id int64, language string, kind Kind, title string) Track {
		return Track{TrackRef{StreamID: id}, Description{Language: language, Kind: kind, ContainerTitle: title}}
	}
	tracks := []Track{
		track(1, "fr", KindHearingImpaired, "Full"),
		track(2, "en", KindForced, "Full"),
		track(3, "en", KindPlain, "Signs"),
		track(4, "en", KindHearingImpaired, ""),
		track(5, "en", KindHearingImpaired, "Full"),
	}
	for _, tc := range []struct {
		name   string
		chosen Description
		want   int64
	}{
		{"the same language, kind and title", Description{"en", KindHearingImpaired, "Full"}, 5},
		{"the same kind before the same title", Description{"en", KindPlain, "Full"}, 3},
		{"the same title when no track is of the kind", Description{"en", KindCommentary, "Full"}, 2},
		{"no track in the language", Description{"de", KindPlain, ""}, 0},
	} {
		got := matchingTrack(tracks, tc.chosen, isSameTag)
		if (got == nil && tc.want != 0) || (got != nil && got.StreamID != tc.want) {
			t.Errorf("%s: matched %+v, want stream %d", tc.name, got, tc.want)
		}
	}
}

func TestWorkTrack_MatchesTheLanguageExactlyFirstThenByItsPrimarySubtag(t *testing.T) {
	t.Parallel()
	track := func(id int64, language string) Track {
		return Track{TrackRef{StreamID: id}, Description{Language: language, Kind: KindPlain}}
	}
	for _, tc := range []struct {
		name   string
		tracks []Track
		chosen string
		want   int64
	}{
		{"en-GB finds an en track", []Track{track(1, "fr"), track(2, "en")}, "en-GB", 2},
		{"en finds an en-GB track", []Track{track(1, "fr"), track(2, "en-GB")}, "en", 2},
		{"an exact match first", []Track{track(1, "en"), track(2, "en-GB"), track(3, "en-US")}, "en-GB", 2},
		{"an exact match first, the other way", []Track{track(1, "en-GB"), track(2, "en")}, "en", 2},
		{"another region of the language", []Track{track(1, "pt-PT"), track(2, "de")}, "pt-BR", 1},
		{"no track of the language", []Track{track(1, "fr"), track(2, "de")}, "en-GB", 0},
	} {
		got := workTrack(tc.tracks, Description{Language: tc.chosen, Kind: KindPlain})
		if (got == nil && tc.want != 0) || (got != nil && got.StreamID != tc.want) {
			t.Errorf("%s: matched %+v, want stream %d", tc.name, got, tc.want)
		}
	}
}

func TestKeepUsed_LeavesAChoiceMadeInTheFile(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	service := NewService(database, clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)))
	userID := storetest.InsertUser(t, database, "alice", "admin")
	const fileID = 6
	french := Track{TrackRef{StreamID: 9}, Description{Language: "fr", Kind: KindPlain}}
	english := Track{TrackRef{StreamID: 8}, Description{Language: "en", Kind: KindPlain}}
	tracks := Tracks{Subtitles: []Track{english, french}}
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		if err := service.ChooseSubtitle(t.Context(), queries, userID, fileID, Used{TrackRef: french.TrackRef},
			tracks.Subtitles); err != nil {
			return err
		}
		return service.KeepUsed(t.Context(), queries, userID, fileID, usedTrack(english), nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	prefs, err := service.ForFile(t.Context(), database.Read(), userID, fileID, tracks)
	if err != nil {
		t.Fatal(err)
	}
	if prefs.Subtitle == nil || *prefs.Subtitle != french.TrackRef {
		t.Errorf("the file starts with %+v, want the French chosen in it, not what a playback used before",
			prefs.Subtitle)
	}
}

// The sort a person lists a library in is kept for that library and that person, and goes with
// their preferences when they are removed (X5, the owner, 2026-11-12; D173 §4.6).
func TestLibrarySorts_AreKeptPerLibraryAndPersonAndForgottenWithThem(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	service := NewService(database, clocktest.NewFake(time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)))
	alice := storetest.InsertUser(t, database, "alice", "admin")
	bob := storetest.InsertUser(t, database, "bob", "admin")
	sortsOf := func(userID int64) map[int64]LibrarySort {
		t.Helper()
		sorts, err := service.LibrarySorts(t.Context(), userID)
		if err != nil {
			t.Fatal(err)
		}
		return sorts
	}
	for _, chosen := range []struct {
		userID, libraryID int64
		sort              LibrarySort
	}{
		{alice, 1, LibrarySort{Sort: "released", Order: "ascending"}},
		{alice, 2, LibrarySort{Sort: "recorded", Order: "descending"}},
		{alice, 1, LibrarySort{Sort: "added", Order: "descending"}},
		{bob, 1, LibrarySort{Sort: "name", Order: "descending"}},
	} {
		if err := service.ChooseLibrarySort(t.Context(), chosen.userID, chosen.libraryID, chosen.sort); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := fmt.Sprint(sortsOf(alice)), "map[1:{added descending} 2:{recorded descending}]"; got != want {
		t.Errorf("alice's sorts = %s; want %s, the last chosen in each library", got, want)
	}
	if got, want := fmt.Sprint(sortsOf(bob)), "map[1:{name descending}]"; got != want {
		t.Errorf("bob's sorts = %s; want %s, his own", got, want)
	}
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return ForgetPreferencesOf(t.Context(), queries, alice)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := sortsOf(alice); len(got) != 0 {
		t.Errorf("alice removed: her sorts %v; want none", got)
	}
	if got := sortsOf(bob); len(got) != 1 {
		t.Errorf("alice removed: bob's sorts %v; want his one kept", got)
	}
}

// A person's Show debug tools is kept as their own switch, off until they turn it on, and read back
// as they left it (the owner, 2026-10-29).
func TestSwitches_ShowDebugToolsIsKeptAsThePersonLeftIt(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	service := NewService(database, clocktest.NewFake(time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)))
	userID := storetest.InsertUser(t, database, "alice", "admin")
	shows := func() bool {
		t.Helper()
		switches, err := service.Switches(t.Context(), userID)
		if err != nil {
			t.Fatal(err)
		}
		return switches.ShowsDebugTools
	}
	if shows() {
		t.Error("before any choice: debug tools shown; want hidden")
	}
	for _, isShown := range []bool{true, false} {
		if err := service.SetDebugTools(t.Context(), userID, isShown); err != nil {
			t.Fatal(err)
		}
		if got := shows(); got != isShown {
			t.Errorf("set to %v: read back %v", isShown, got)
		}
	}
}
