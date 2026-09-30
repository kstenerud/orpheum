// SPDX-License-Identifier: GPL-3.0-or-later

package preferences

import (
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
		got := matchingTrack(tracks, tc.chosen)
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
