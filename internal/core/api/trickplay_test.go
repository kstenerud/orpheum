// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/trickplay"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// shelveFilmsForTrickplay shelves films 1 and 2 with their pictures, film 2's cover besides.
func (harness testHarness) shelveFilmsForTrickplay(t *testing.T) {
	t.Helper()
	harness.shelveFilms(t)
	harness.shelvePictures(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 4)
}

// filesForTrickplay are the files the trickplay sweep finds.
func (harness testHarness) filesForTrickplay(t *testing.T) []int64 {
	t.Helper()
	ids, err := harness.trickplay.FilesToMake(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

// startTrickplay runs film 2's trickplay as job jobID and returns the command its node was sent and
// the job's end.
func (harness testHarness) startTrickplay(
	t *testing.T, jobID int64, stalledBefore bool,
) (nodeproto.Command, <-chan error) {
	t.Helper()
	job := jobs.Job{
		ID: jobID, Kind: jobs.KindTrickplay, Target: jobs.Target{FileID: 2}, PreviousAttemptToolFailed: stalledBefore,
	}
	finished := harness.handle(t, harness.trickplay.HandleTrickplay, job)
	command := harness.nextCommand(t)
	if command.MakeTrickplay == nil {
		t.Fatalf("film 2's trickplay sent %+v; want a trickplay", command)
	}
	return command, finished
}

// postSheet posts sheet index of command's run from node 1, as its node does, its pictures 180 px
// high.
func (harness testHarness) postSheet(
	t *testing.T, command nodeproto.Command, jobID int64, index int, webp string,
) error {
	t.Helper()
	//nolint:wrapcheck // the test reads the exchanges' own error
	return harness.exchanges.DeliverTrickplaySheet(t.Context(), 1, nodeproto.TrickplaySheet{
		JobID: jobID, AttemptToken: command.AttemptToken, Index: index, WebP: []byte(webp), PictureHeight: 180,
	})
}

// sheetsOf are the sheets kept of command's bytes.
func (harness testHarness) sheetsOf(t *testing.T, command nodeproto.Command) []cachegen.ListTrickplaySheetsRow {
	t.Helper()
	payloadHash, err := hex.DecodeString(command.MakeTrickplay.Video.PayloadHash)
	if err != nil {
		t.Fatal(err)
	}
	sheets, err := harness.cache.Read().ListTrickplaySheets(t.Context(), cachegen.ListTrickplaySheetsParams{
		PayloadHash: payloadHash, Version: trickplay.StoredVersion(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return sheets
}

func shaOf(webp string) string {
	sum := sha256.Sum256([]byte(webp))
	return hex.EncodeToString(sum[:])
}

// Each live video with a picture has its pictures made once per payload hash, a file of covers
// never: the node is told the version's recipe and the main picture, each sheet it posts is kept by
// its SHA-256 in the cache directory, a sheet posted again changes nothing, and once its run ends the
// sweep asks no more of the bytes (trickplay.md sections 1, 3, 4).
func TestTrickplay_EachVideoHasItsPicturesMadeOnceAndEachSheetKeptAsItArrives(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	harness.shelveFilmsForTrickplay(t)
	if ids := harness.filesForTrickplay(t); !slices.Equal(ids, []int64{1, 2}) {
		t.Fatalf("the sweep finds %v; want films 1 and 2", ids)
	}
	command, finished := harness.startTrickplay(t, 90, false)
	wantAskedInTheBackground(t, command)
	harness.postSheetsZeroTwiceAndOne(t, command)
	harness.deliver(t, 90, nodeproto.Result{Trickplay: &nodeproto.TrickplayResult{}})
	if err := testutil.Receive(t, finished, "the trickplay to end"); err != nil {
		t.Errorf("a made trickplay ended %v; want done", err)
	}
	if ids := harness.filesForTrickplay(t); !slices.Equal(ids, []int64{1}) {
		t.Errorf("after film 2's are made, the sweep finds %v; want film 1 alone", ids)
	}
	if err := harness.trickplay.HandleTrickplay(t.Context(), jobs.Job{
		ID: 91, Kind: jobs.KindTrickplay, Target: jobs.Target{FileID: 2},
	}); err != nil || len(harness.nodes.commanded) != 0 {
		t.Errorf("film 2's trickplay asked again: %v, %d commands; want nothing asked", err,
			len(harness.nodes.commanded))
	}
}

// wantAskedInTheBackground checks that film 2's trickplay asked the version's recipe of its main
// picture, from the first sheet, in the background.
func wantAskedInTheBackground(t *testing.T, command nodeproto.Command) {
	t.Helper()
	want := nodeproto.MakeTrickplay{
		Video: command.MakeTrickplay.Video, VideoStreamIndex: 0, IntervalMs: 10_000, PictureWidth: 320,
		SheetColumns: 10, SheetRows: 10, WebPQuality: 60,
	}
	got := *command.MakeTrickplay
	isRecipe := got.IntervalMs == want.IntervalMs && got.PictureWidth == want.PictureWidth &&
		got.SheetColumns == 10 && got.SheetRows == 10 && got.WebPQuality == 60
	if !slices.Equal(got.SheetsMade, nil) || got.VideoStreamIndex != want.VideoStreamIndex || !isRecipe ||
		command.IsPlayDriven {
		t.Errorf("film 2's trickplay asked %+v; want %+v in the background", got, want)
	}
}

// postSheetsZeroTwiceAndOne posts sheet 0 twice, the second as a node whose answer was lost sends
// it, then sheet 1, and checks that each is kept once by its SHA-256 with its image.
func (harness testHarness) postSheetsZeroTwiceAndOne(t *testing.T, command nodeproto.Command) {
	t.Helper()
	for range 2 {
		if err := harness.postSheet(t, command, 90, 0, "RIFF sheet 0"); err != nil {
			t.Fatal(err)
		}
	}
	if err := harness.postSheet(t, command, 90, 1, "RIFF sheet 1"); err != nil {
		t.Fatal(err)
	}
	sheets := harness.sheetsOf(t, command)
	if len(sheets) != 2 || sheets[0].SheetSha256Hex != shaOf("RIFF sheet 0") || sheets[1].SizeBytes != 12 {
		t.Errorf("sheets kept %+v; want sheets 0 and 1 by their SHA-256", sheets)
	}
	if image, err := harness.directories.ReadTrickplaySheet(shaOf("RIFF sheet 1")); err != nil ||
		string(image) != "RIFF sheet 1" {
		t.Errorf("sheet 1's image = %q, %v", image, err)
	}
}

// A run its node refused, or whose node went away, keeps the sheets it posted, and the next run
// starts after them (trickplay.md section 3, SheetsMade); a sheet of a run no longer held is refused,
// which stops its node.
func TestTrickplay_ARunStoppedPartWayResumesAfterTheSheetsKept(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	harness.shelveFilmsForTrickplay(t)
	command, finished := harness.startTrickplay(t, 92, false)
	for index := range 2 {
		if err := harness.postSheet(t, command, 92, index, "RIFF "+string(rune('a'+index))); err != nil {
			t.Fatal(err)
		}
	}
	harness.deliver(t, 92, nodeproto.Result{Trickplay: &nodeproto.TrickplayResult{
		Refusal: "too busy",
	}})
	if err := testutil.Receive(t, finished, "the trickplay to end"); err == nil || jobs.IsMarkedPermanent(err) {
		t.Errorf("a refused trickplay ended %v; want it tried again", err)
	}
	if err := harness.postSheet(t, command, 92, 2, "RIFF c"); !errors.Is(err, nodes.ErrAttemptNotHeld) {
		t.Errorf("a sheet after its run ended: %v; want ErrAttemptNotHeld", err)
	}
	if ids := harness.filesForTrickplay(t); !slices.Equal(ids, []int64{1, 2}) {
		t.Errorf("after film 2's run was refused, the sweep finds %v; want film 2 asked again", ids)
	}
	again, _ := harness.startTrickplay(t, 93, false)
	if !slices.Equal(again.MakeTrickplay.SheetsMade, []int{0, 1}) {
		t.Errorf("the next run was told sheets %v were made; want 0 and 1", again.MakeTrickplay.SheetsMade)
	}
}

// FFmpeg's word that it cannot make a file's pictures, or two stalls in a row, is the bytes' failure,
// kept so they are not asked again; one stall is not (D244 §1).
func TestTrickplay_FFmpegsFailureAndTwoStallsInARowAreKeptAndNotAskedAgain(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		answers  []nodeproto.TrickplayResult
		wantText string
	}{
		"FFmpeg's failure": {[]nodeproto.TrickplayResult{{Failure: "invalid data"}}, "invalid data"},
		"two stalls in a row": {
			[]nodeproto.TrickplayResult{{Stall: testStall}, {Stall: testStall}}, "FFmpeg stalled twice",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			harness := newHarness(t)
			harness.createAdminAndSignIn(t)
			harness.shelveFilmsForTrickplay(t)
			var err error
			for attempt, answer := range test.answers {
				if attempt > 0 {
					if ids := harness.filesForTrickplay(t); !slices.Equal(ids, []int64{1, 2}) {
						t.Errorf("after one stall the sweep finds %v; want film 2 asked again", ids)
					}
				}
				jobID := int64(94 + attempt)
				_, finished := harness.startTrickplay(t, jobID, attempt > 0)
				harness.deliver(t, jobID, nodeproto.Result{Trickplay: &answer})
				err = testutil.Receive(t, finished, "the trickplay to end")
			}
			if err == nil || !strings.Contains(err.Error(), test.wantText) {
				t.Errorf("the trickplay ended %v; want %q", err, test.wantText)
			}
			if ids := harness.filesForTrickplay(t); !slices.Equal(ids, []int64{1}) {
				t.Errorf("after %s the sweep finds %v; want film 1 alone", name, ids)
			}
		})
	}
}

// A file under a root that is unavailable is not swept (D231 §5).
func TestTrickplay_AFileUnderAMissingRootIsNotSwept(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	harness.shelveFilmsForTrickplay(t)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at)
		 VALUES (1, '/media', 'unreadable', 5000)`,
		`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (1, '/media', '/media', 5000, 'unread')`)
	if ids := harness.filesForTrickplay(t); len(ids) != 0 {
		t.Errorf("with its root unreadable, the sweep finds %v; want none", ids)
	}
}

// A probe whose picture changed forgets the bytes' pictures and their images (D392 §3); the removed
// files' sweep forgets those of bytes no file names, and every image no row names, as a cache.db
// thrown out leaves (D231 §3). An image another row names stays.
func TestTrickplay_PicturesOfChangedOrRemovedBytesAreForgottenWithTheirImages(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	harness.shelveFilmsForTrickplay(t)
	command, _ := harness.startTrickplay(t, 96, false)
	for _, index := range []int{0, 1} {
		if err := harness.postSheet(t, command, 96, index, "RIFF shared"); err != nil {
			t.Fatal(err)
		}
	}
	if err := harness.postSheet(t, command, 96, 2, "RIFF own"); err != nil {
		t.Fatal(err)
	}
	stray := harness.keepSharedAndStrayImages(t)
	payloadHash, err := hex.DecodeString(command.MakeTrickplay.Video.PayloadHash)
	if err != nil {
		t.Fatal(err)
	}
	of := library.DerivedOf{FileID: 2, PayloadHash: payloadHash}
	if forgot, forgetErr := harness.trickplay.Forget(t.Context(), of); forgetErr != nil || !forgot {
		t.Fatalf("Forget = %v, %v; want film 2's forgotten", forgot, forgetErr)
	}
	if sheets := harness.sheetsOf(t, command); len(sheets) != 0 {
		t.Errorf("film 2's sheets after Forget: %+v; want none", sheets)
	}
	isThere := func(image string) bool { return harness.hasTrickplayImage(t, image) }
	if isThere(shaOf("RIFF own")) || !isThere(shaOf("RIFF shared")) {
		t.Errorf("after Forget, its own image there %v, the shared one %v; want only the shared one kept",
			isThere(shaOf("RIFF own")), isThere(shaOf("RIFF shared")))
	}
	if _, err := harness.trickplay.ForgetTheRemoved(t.Context()); err != nil {
		t.Fatal(err)
	}
	if isThere(shaOf("RIFF shared")) || isThere(stray) {
		t.Error("the removed files' sweep left an image of bytes no file names, or one no row names")
	}
}

// keepSharedAndStrayImages keeps a row of bytes no file names that shares film 2's image "RIFF
// shared", and an image no row names, and returns the stray image's name.
func (harness testHarness) keepSharedAndStrayImages(t *testing.T) string {
	t.Helper()
	gone := []byte{0xde, 0xad}
	err := harness.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		return queries.UpsertTrickplaySheet(t.Context(), cachegen.UpsertTrickplaySheetParams{
			PayloadHash: gone, Version: trickplay.StoredVersion(), SheetIndex: 0, SheetSha256Hex: shaOf("RIFF shared"),
			SizeBytes: 11, PictureHeight: 180, MadeAt: 1,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	stray := strings.Repeat("ab", 32)
	if err := harness.directories.WriteTrickplaySheet(stray, []byte("RIFF stray")); err != nil {
		t.Fatal(err)
	}
	return stray
}

// hasTrickplayImage says whether the cache directory holds image.
func (harness testHarness) hasTrickplayImage(t *testing.T, image string) bool {
	t.Helper()
	_, err := harness.directories.ReadTrickplaySheet(image)
	if err != nil && !errors.Is(err, instancedir.ErrNotFound) {
		t.Fatal(err)
	}
	return err == nil
}
