// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
)

// signInStranger makes a person whose role reaches no library and signs them in.
func (harness testHarness) signInStranger(t *testing.T, admin string) string {
	t.Helper()
	made := harness.do(t, http.MethodPost, "/roles", admin, map[string]any{
		"name": "no films", "capabilities": []string{}, "libraries": []any{},
	})
	if made.status != http.StatusCreated {
		t.Fatalf("POST /roles = %d %s", made.status, made.body)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "stranger", Password: "stranger-password", Role: "no films"}); err != nil {
		t.Fatal(err)
	}
	return harness.signIn(t, "stranger", "stranger-password")
}

// trickplaySeen is GET /files/{id}/trickplay's answer.
type trickplaySeen struct {
	IntervalMs    int64 `json:"intervalMs"`
	PictureWidth  int   `json:"pictureWidth"`
	PictureHeight int   `json:"pictureHeight"`
	Columns       int   `json:"columns"`
	Rows          int   `json:"rows"`
	PictureCount  int   `json:"pictureCount"`
	Sheets        []struct {
		Index int    `json:"index"`
		URL   string `json:"url"`
	} `json:"sheets"`
}

// trickplay.md §5: a file's scrub-bar pictures are unknown (404) until its first sheet is kept; then
// they are the version's sizes, the whole film's count and each sheet kept, in order, whose address
// serves its WebP by the media cookie, kept for good as an image is. A sheet not the file's, and a
// person not reaching the file, are answered as what does not exist; no one signed in is refused.
func TestTrickplay_AFilesPicturesAreItsSheetsKeptServedByContentToThoseReachingIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilmsForTrickplay(t)
	wantError(t, harness.do(t, http.MethodGet, "/files/2/trickplay", admin, nil), http.StatusNotFound,
		api.CodeNotFound)
	command, _ := harness.startTrickplay(t, 97, false)
	for _, index := range []int{1, 0} {
		if err := harness.postSheet(t, command, 97, index, "RIFF sheet "+strconv.Itoa(index)); err != nil {
			t.Fatal(err)
		}
	}
	var seen trickplaySeen
	harness.readJSON(t, "/files/2/trickplay", admin, &seen)
	wantSheetsZeroAndOne(t, seen)
	got := harness.getBytePath(t, seen.Sheets[0].URL, admin, true, "")
	if got.status != http.StatusOK || got.body != "RIFF sheet 0" {
		t.Fatalf("GET sheet 0 by the media cookie = %d %q; want its WebP", got.status, got.body)
	}
	if contentType, cache := got.header.Get("Content-Type"), got.header.Get("Cache-Control"); contentType !=
		"image/webp" || cache != "private, max-age=31536000, immutable" {
		t.Errorf("Content-Type %q, Cache-Control %q; want image/webp kept for good", contentType, cache)
	}
	wantTrickplayRefused(t, harness, admin, seen.Sheets[0].URL)
}

// wantSheetsZeroAndOne fails t unless seen is film 2's pictures with its sheets 0 and 1, in order.
func wantSheetsZeroAndOne(t *testing.T, seen trickplaySeen) {
	t.Helper()
	if seen.IntervalMs != 10_000 || seen.PictureWidth != 320 || seen.PictureHeight != 180 ||
		seen.Columns != 10 || seen.Rows != 10 || seen.PictureCount != 120 {
		t.Errorf("the pictures are %+v; want 120 of 320×180 every 10 s, 10×10 to a sheet", seen)
	}
	var indexes []int
	for _, sheet := range seen.Sheets {
		indexes = append(indexes, sheet.Index)
	}
	if !slices.Equal(indexes, []int{0, 1}) {
		t.Fatalf("the sheets are %+v; want 0 and 1, in order", seen.Sheets)
	}
	want := api.Prefix + "/bytes/trickplay/2/" + shaOf("RIFF sheet 0") + ".webp"
	if seen.Sheets[0].URL != want {
		t.Errorf("sheet 0 is at %q; want %q", seen.Sheets[0].URL, want)
	}
}

// wantTrickplayRefused fails t unless film 2's sheet at sheetURL is refused when asked as film 1's,
// and film 2's pictures and the sheet to a person reaching no library and to no one signed in.
func wantTrickplayRefused(t *testing.T, harness testHarness, admin, sheetURL string) {
	t.Helper()
	notTheFiles := api.Prefix + "/bytes/trickplay/1/" + shaOf("RIFF sheet 0") + ".webp"
	if refused := harness.getBytePath(t, notTheFiles, admin, true, ""); refused.status != http.StatusNotFound {
		t.Errorf("film 2's sheet asked as film 1's = %d; want 404", refused.status)
	}
	stranger := harness.signInStranger(t, admin)
	wantError(t, harness.do(t, http.MethodGet, "/files/2/trickplay", stranger, nil), http.StatusNotFound,
		api.CodeNotFound)
	if refused := harness.getBytePath(t, sheetURL, stranger, true, ""); refused.status !=
		http.StatusNotFound {
		t.Errorf("a person reaching no library = %d for a sheet; want 404", refused.status)
	}
	wantError(t, harness.do(t, http.MethodGet, "/files/2/trickplay", "", nil), http.StatusUnauthorized,
		api.CodeAuthRequired)
	if refused := harness.getBytePath(t, sheetURL, "", true, ""); refused.status !=
		http.StatusUnauthorized {
		t.Errorf("no one signed in = %d for a sheet; want 401", refused.status)
	}
}
