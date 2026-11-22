// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
)

// D537: an admin alone sets a file's sound offset for every person; a person's own overrides it for
// them alone, and once they forget it, every person's holds for them again. The playback's start
// says both, and the one the file plays with.
func TestFileSoundOffset_EveryonesIsAnAdminsAndAPersonsOwnOverridesItUntilForgotten(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	partner := signInPartner(t, harness)
	harness.shelveFilms(t)
	expectOffset := func(token string, want api.FileSoundOffsetBody, why string) {
		t.Helper()
		if got := harness.play(t, token, chrome(2)).FileSoundOffset; !sameSoundOffset(got, want) {
			t.Errorf("%s: %s, want %s", why, soundOffsetText(got), soundOffsetText(want))
		}
	}
	expectStatus := func(got, want int, why string) {
		t.Helper()
		if got != want {
			t.Fatalf("%s: %d, want %d", why, got, want)
		}
	}
	everyones := func(token string, offset int) int {
		return harness.do(t, http.MethodPut, "/files/2/sound-offset", token,
			map[string]int{"soundOffsetMs": offset}).status
	}
	own := func(body map[string]any) int {
		return harness.do(t, http.MethodPut, "/files/2/preferences", partner, body).status
	}
	ownMs := int64(-900)
	everyonesOnly := api.FileSoundOffsetBody{SoundOffsetMs: -1200, EveryonesMs: -1200}

	expectOffset(partner, api.FileSoundOffsetBody{}, "a file no one set")
	expectStatus(everyones(partner, -1200), http.StatusForbidden, "a person who is not an admin sets every person's")
	expectStatus(everyones(admin, -1200), http.StatusNoContent, "the admin sets every person's")
	if status := everyones(admin, 2500); status < 400 || status >= 500 {
		t.Errorf("every person's past two seconds: %d, want refused", status)
	}
	expectOffset(partner, everyonesOnly, "the partner, with none of their own")
	expectStatus(own(map[string]any{"soundOffsetMs": ownMs}), http.StatusNoContent, "the partner sets their own")
	expectOffset(partner, api.FileSoundOffsetBody{SoundOffsetMs: ownMs, EveryonesMs: -1200, OwnMs: &ownMs},
		"the partner, their own set over every person's")
	expectOffset(admin, everyonesOnly, "the admin, after the partner's own")
	expectStatus(own(map[string]any{"soundOffsetMs": 100, "isSoundOffsetEveryones": true}),
		http.StatusBadRequest, "their own and every person's at once")
	expectStatus(own(map[string]any{"isSoundOffsetEveryones": true}), http.StatusNoContent,
		"the partner forgets their own")
	expectOffset(partner, everyonesOnly, "the partner, their own forgotten")
	expectStatus(everyones(admin, 0), http.StatusNoContent, "the admin puts the file back in step")
	expectOffset(partner, api.FileSoundOffsetBody{}, "after every person's 0")
	expectStatus(harness.do(t, http.MethodPut, "/files/3/sound-offset", admin,
		map[string]int{"soundOffsetMs": 300}).status, http.StatusNotFound, "a file that is not played")
}

// sameSoundOffset reports whether two playbacks' file sound offsets say the same, their own by value.
func sameSoundOffset(got, want api.FileSoundOffsetBody) bool {
	if got.SoundOffsetMs != want.SoundOffsetMs || got.EveryonesMs != want.EveryonesMs {
		return false
	}
	if got.OwnMs == nil || want.OwnMs == nil {
		return got.OwnMs == nil && want.OwnMs == nil
	}
	return *got.OwnMs == *want.OwnMs
}

// soundOffsetText says a file sound offset, its own by value, for a failure's message.
func soundOffsetText(offset api.FileSoundOffsetBody) string {
	own := "none"
	if offset.OwnMs != nil {
		own = strconv.FormatInt(*offset.OwnMs, 10)
	}
	return fmt.Sprintf("plays %d, every person's %d, own %s", offset.SoundOffsetMs, offset.EveryonesMs, own)
}

// D537: a report may say a file needs a sound offset and suggest one; the admin's list says the
// file and the suggestion, which the admin applies with PUT /files/{id}/sound-offset.
func TestProblemReports_AReportSuggestsAFilesSoundOffsetAndTheListSaysIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	person := signInPartner(t, harness)
	harness.shelveFilms(t)
	report := func(fileID int64) int {
		return harness.do(t, http.MethodPost, "/problem-reports", person, map[string]any{
			"words": problemReportWords, "page": "/watch/2", "device": []map[string]any{},
			"soundOffset": map[string]any{"fileId": fileID, "soundOffsetMs": -900},
		}).status
	}
	if status := report(999); status != http.StatusNotFound {
		t.Errorf("a suggestion for a file the person cannot reach: %d, want 404", status)
	}
	if status := report(2); status != http.StatusCreated {
		t.Fatalf("a report suggesting file 2's offset: %d, want 201", status)
	}
	makeProblemReport(t, harness, person)
	got := harness.do(t, http.MethodGet, "/problem-reports", admin, nil)
	var listed api.ProblemReportsOutput
	if err := json.Unmarshal([]byte(got.body), &listed.Body); err != nil || len(listed.Body.Reports) != 2 {
		t.Fatalf("GET /problem-reports = %d %s; want the two reports", got.status, got.body)
	}
	var suggested []api.ProblemReportSoundOffsetBody
	for _, summary := range listed.Body.Reports {
		if summary.SoundOffset != nil {
			suggested = append(suggested, *summary.SoundOffset)
		}
	}
	if len(suggested) != 1 || suggested[0].FileID != 2 || suggested[0].SoundOffsetMs != -900 {
		t.Errorf("the list's suggestions: %+v, want file 2's -900 alone", suggested)
	}
}
