// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/jobs"
)

// walkPages follows a list's nextCursor from its first page to its last, path asking limit at a
// time, and answers each page's entries by field, a page's entries in order. It fails at a page
// fuller than limit, and at one with no nextCursor but the last.
func (harness testHarness) walkPages(t *testing.T, token, path, list, field string, limit int) [][]string {
	t.Helper()
	var pages [][]string
	cursor := ""
	for range 20 {
		separator := "?"
		if strings.Contains(path, "?") {
			separator = "&"
		}
		asked := fmt.Sprintf("%s%slimit=%d", path, separator, limit)
		if cursor != "" {
			asked += "&cursor=" + url.QueryEscape(cursor)
		}
		answer := harness.do(t, http.MethodGet, asked, token, nil)
		var body map[string]json.RawMessage
		if err := json.Unmarshal([]byte(answer.body), &body); err != nil || answer.status != http.StatusOK {
			t.Fatalf("GET %s = %d %s", asked, answer.status, answer.body)
		}
		var entries []map[string]any
		if err := json.Unmarshal(body[list], &entries); err != nil {
			t.Fatalf("GET %s: %s is not a list: %s", asked, list, answer.body)
		}
		if len(entries) > limit {
			t.Fatalf("GET %s: %d entries, more than the limit", asked, len(entries))
		}
		page := []string{}
		for _, entry := range entries {
			page = append(page, fmt.Sprint(entry[field]))
		}
		pages = append(pages, page)
		next, hasNext := body["nextCursor"]
		if !hasNext {
			return pages
		}
		if err := json.Unmarshal(next, &cursor); err != nil || cursor == "" {
			t.Fatalf("GET %s: nextCursor %s; want a cursor or none at all (D416 §6)", asked, next)
		}
	}
	t.Fatalf("GET %s: still a nextCursor after 20 pages", path)
	return nil
}

// Everyone, the invitations, the genres, the job view and the filing reports are each paged by
// the cursor GET /items gives: a first page of limit, the next reached by its nextCursor, and the
// last page with no nextCursor at all (D416 §6).
func TestPages_EachListThatCanGrowIsPagedByItsCursor(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelvePeople(t)
	for _, name := range []string{"carol", "bob"} {
		created := harness.do(t, http.MethodPost, "/users", admin,
			map[string]string{"name": name, "password": name + "-password", "role": "user"})
		if created.status != http.StatusCreated {
			t.Fatalf("POST /users %s = %d %s", name, created.status, created.body)
		}
	}
	var invited []string
	for _, note := range []string{"first", "second", "third"} {
		invited = append(invited, fmt.Sprint(harness.invite(t, admin, "user", note).Invitation.ID))
	}
	slices.Reverse(invited)
	for fileID := range int64(3) {
		request := jobs.Request{Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 100 + fileID}}
		if _, err := harness.queue.Enqueue(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	reports := harness.writeFilingReports(t, admin, 3)

	for _, list := range []struct {
		path, list, field string
		want              [][]string
	}{
		{"/users", "users", "name", [][]string{{"bob", "carol"}, {"owner"}}},
		{"/invitations", "invitations", "id", [][]string{invited[:2], invited[2:]}},
		{"/genres?kind=film,series,other", "genres", "name", [][]string{{"Crime", "Drama"}, {}}},
		{"/filing-reports", "reports", "name", [][]string{reports[:2], reports[2:]}},
	} {
		if pages := harness.walkPages(t, admin, list.path, list.list, list.field, 2); !equalPages(pages, list.want) {
			t.Errorf("%s by 2: %q, want %q", list.path, pages, list.want)
		}
	}
	jobPages := harness.walkPages(t, admin, "/jobs", "jobs", "id", 2)
	every := harness.walkPages(t, admin, "/jobs", "jobs", "id", 500)
	if len(every) != 1 || len(every[0]) < 3 || !equalPages([][]string{slices.Concat(jobPages...)}, every) ||
		len(jobPages[0]) != 2 {
		t.Errorf("GET /jobs by 2: %q; want the one page of 500, %q, two at a time", jobPages, every)
	}
}

// The job view's pages end at the newest job_view_max_jobs, however small its pages: the setting
// says how far back it reaches, the cursor only how it is cut.
func TestPages_TheJobViewsPagesEndAtItsMaxJobs(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	for fileID := range int64(4) {
		request := jobs.Request{Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 100 + fileID}}
		if _, err := harness.queue.Enqueue(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	harness.setSetting(t, "job_view_max_jobs", "3")
	pages := harness.walkPages(t, admin, "/jobs", "jobs", "id", 2)
	if len(pages) != 2 || len(pages[0]) != 2 || len(pages[1]) != 1 {
		t.Errorf("GET /jobs by 2 at job_view_max_jobs 3: %q; want two pages, of 2 and of 1, and no cursor after", pages)
	}
}

// A cursor this server did not give is refused at query.cursor, on each list.
func TestPages_ACursorTheServerDidNotGiveIsRefused(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	for _, path := range []string{
		"/users?", "/invitations?", "/genres?kind=film,series,other&", "/jobs?", "/filing-reports?",
	} {
		wantError(t, harness.do(t, http.MethodGet, path+"limit=2&cursor=%21%21", admin, nil),
			http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	}
}

// writeFilingReports has the admin write a filing report, and beside it on the disk count-1 more,
// each written a minute before the one before; it answers their names newest first.
func (harness testHarness) writeFilingReports(t *testing.T, admin string, count int) []string {
	t.Helper()
	made := harness.do(t, http.MethodPost, "/items/20/filing-report", admin, nil)
	var report api.FilingReportBody
	if err := json.Unmarshal([]byte(made.body), &report); err != nil || made.status != http.StatusCreated {
		t.Fatalf("POST /items/20/filing-report = %d %s", made.status, made.body)
	}
	written := time.Date(2020, 1, 9, 12, 0, 0, 0, time.UTC)
	names := []string{report.Name}
	for index := range count {
		path := report.Path
		if index > 0 {
			names = append(names, fmt.Sprintf("brass-arena-20-2020010%dT120000Z.md", 9-index))
			path = filepath.Join(filepath.Dir(report.Path), names[index])
			if err := os.WriteFile(path, []byte(report.Markdown), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		at := written.Add(-time.Duration(index) * time.Minute)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	return names
}

func equalPages(got, want [][]string) bool {
	return slices.EqualFunc(got, want, slices.Equal)
}
