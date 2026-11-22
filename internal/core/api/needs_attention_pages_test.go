// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// attentionPages is what GET /needs-attention says needs the admin by page.
func attentionPages(t *testing.T, harness testHarness, token string) *api.AttentionPagesBody {
	t.Helper()
	got := harness.do(t, http.MethodGet, "/needs-attention", token, nil)
	var body api.NeedsAttentionBody
	if err := json.Unmarshal([]byte(got.body), &body); err != nil || got.status != http.StatusOK {
		t.Fatalf("GET /needs-attention = %d %s", got.status, got.body)
	}
	if body.Pages == nil {
		t.Fatalf("GET /needs-attention = %s, want the pages", got.body)
	}
	return body.Pages
}

// D538 §12: Library setup's items are each folder and disk offline, with its library, machine and
// path, the anchor its line links to; nothing is on the page while nothing is offline. The counts
// beside them say what they said before (TestNeedsAttention_CountsTheFoldersAndDisksOffline).
func TestNeedsAttention_PagesListEachPlaceOfflineWithItsMachineAndPath(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	if before := attentionPages(t, harness, admin); before.LibrarySetup != nil {
		t.Errorf("with nothing offline, Library setup lists %+v; want nothing", before.LibrarySetup)
	}
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at)
		 VALUES (1, '/media', 'unreadable', 9000)`,
		`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (1, '/media', '/media', 5000, 'unread')`,
		`INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
		 VALUES (1, '/media', '/media/disk1', 21, 0)`,
		`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (1, '/media/disk1', '/media', 3000, 'missing')`)
	page := attentionPages(t, harness, admin).LibrarySetup
	if page == nil || !page.IsProblem || page.ItemCount != 2 || len(page.Items) != 2 {
		t.Fatalf("Library setup = %+v; want two problems", page)
	}
	byKind := map[string]api.AttentionItemBody{}
	for _, item := range page.Items {
		byKind[item.Kind] = item
	}
	folder := byKind["folderOffline"]
	if folder.NodeName == "" {
		t.Fatalf("the folder offline = %+v; want its machine named", folder)
	}
	wantFolder := api.AttentionItemBody{
		Kind: "folderOffline", IsProblem: true, Since: 5000, LibraryID: 1, LibraryName: folder.LibraryName,
		NodeName: folder.NodeName, RootPath: "/media",
	}
	if folder != wantFolder {
		t.Errorf("the folder offline = %+v; want library 1's /media on its machine since 5000", folder)
	}
	wantDisk := wantFolder
	wantDisk.Kind, wantDisk.Since, wantDisk.MountPath = "diskOffline", 3000, "/media/disk1"
	if disk := byKind["diskOffline"]; disk != wantDisk {
		t.Errorf("the disk offline = %+v; want /media/disk1 inside library 1's /media since 3000", disk)
	}
}

// D538 §12: System's items put its problems before its checks: the jobs failed within the job
// health window (the Jobs page's count) are a problem; the doctor's warnings after them are not.
// An admin's shell read carries the same pages, so the menu marks System in the one read.
func TestNeedsAttention_PagesPutSystemsWorkFailingBeforeItsChecks(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	failTwoJobs(t, harness)
	page := attentionPages(t, harness, admin).System
	if page == nil || !page.IsProblem || len(page.Items) == 0 || page.ItemCount != len(page.Items) {
		t.Fatalf("System = %+v; want its problems", page)
	}
	failing := api.AttentionItemBody{Kind: "workFailing", IsProblem: true, Count: 2, WindowHours: 24}
	if first := page.Items[0]; first != failing {
		t.Errorf("System's first item = %+v; want two jobs failed in 24 hours, a problem", first)
	}
	for index, item := range page.Items[1:] {
		if item.IsProblem || item.Kind != "finding" || item.Finding == nil ||
			item.Finding.Verdict != api.VerdictWarning {
			t.Errorf("System's item %d = %+v; want only the doctor's warnings after the problem", index+1, item)
		}
	}
	if system := shellSystemPage(t, harness, admin); system == nil || system.Items[0].Kind != "workFailing" {
		t.Errorf("GET /me?shell=true's System page = %+v; want work failing first", system)
	}
}

// shellSystemPage is the System page of what needs the admin as an admin's shell read says it.
func shellSystemPage(t *testing.T, harness testHarness, token string) *api.AttentionPageBody {
	t.Helper()
	var shell struct {
		Shell struct {
			NeedsAttention *api.NeedsAttentionBody `json:"needsAttention"`
		} `json:"shell"`
	}
	got := harness.do(t, http.MethodGet, "/me?shell=true", token, nil)
	if err := json.Unmarshal([]byte(got.body), &shell); err != nil || shell.Shell.NeedsAttention == nil ||
		shell.Shell.NeedsAttention.Pages == nil {
		t.Fatalf("GET /me?shell=true = %d %s; want its needsAttention's pages", got.status, got.body)
	}
	return shell.Shell.NeedsAttention.Pages.System
}

// failTwoJobs fails two read-through jobs for good, two hours before now.
func failTwoJobs(t *testing.T, harness testHarness) {
	t.Helper()
	for fileID := range int64(2) {
		if _, err := harness.queue.Enqueue(t.Context(), jobs.Request{
			Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: fileID + 1}, Reason: "test",
		}); err != nil {
			t.Fatal(err)
		}
	}
	failing := harness.queue.NewRunner(jobs.KindReadThrough, func(context.Context, jobs.Job) error {
		return jobs.MarkPermanent(errors.New("the file is unreadable"))
	})
	for range 2 {
		if err := failing.RunDue(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	harness.clock.Advance(2 * time.Hour)
}

// D538 §12: Titles to check's items are notices, counted as the tray counts them: a film with no
// sound still plays, without it.
func TestNeedsAttention_PagesCountTitlesToCheckAsNotices(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	if before := attentionPages(t, harness, admin); before.TitlesToCheck != nil {
		t.Errorf("with every title well, Titles to check lists %+v; want nothing", before.TitlesToCheck)
	}
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 11, Library: 1, Year: 1950, AddedAt: 3})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 3, TitleID: 11, CreatedAt: 3})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 3, LibraryID: 1, NodeID: 1, Path: "/m/Mister 517 (1950).mp4", SizeBytes: 1000, PayloadHash: []byte{0x03},
		Filing: "listed", VersionID: 3,
	})
	librarytest.Stream(t, harness.database,
		librarytest.StreamSpec{BytesFile: 3, StreamIndex: 0, Kind: "video", Codec: "h264"})
	page := attentionPages(t, harness, admin).TitlesToCheck
	want := api.AttentionItemBody{Kind: "filmsWithoutSound", Count: 1}
	if page == nil || page.IsProblem || page.ItemCount != 1 || len(page.Items) != 1 || page.Items[0] != want {
		t.Errorf("Titles to check = %+v; want one film without sound, a notice", page)
	}
}
