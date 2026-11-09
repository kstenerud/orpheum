// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/node/scan"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The core lists a root, then asks for the files it needs read: the node answers each as a result
// of the job (D240 §2, §4). The listing's start and end are logged with how long each took.
func TestAgent_ListsARootAndExaminesTheFilesTheCoreAsksFor(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	logged := &testutil.Log{}
	root, _ := startAgentWithWorkers(t, socket, Workers{
		Scanner: fakeScanner{}, Artworker: fakeArtworker{}, Subtitler: fakeSubtitler{},
		ThroughReader: fakeThroughReader{}, ChunkMaker: fakeChunkMaker{},
	}, func(node *Agent) { node.logger = slog.New(slog.NewTextHandler(logged, nil)) })
	if err := os.WriteFile(filepath.Join(root.Path(), "Arkon.mkv"), []byte("arkon"), 0o600); err != nil {
		t.Fatal(err)
	}
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 1, Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: root.Path(), Path: "."},
	}}
	listed := testutil.Receive(t, core.scans, "the listing")
	if listed.JobID != 1 || listed.Listing == nil || !slices.ContainsFunc(listed.Listing.Records,
		func(record nodeproto.ScanRecord) bool {
			return record.Listed != nil && record.Listed.Path.Text == "Arkon.mkv"
		}) {
		t.Errorf("job 1's answer = %+v, want a listing naming Arkon.mkv", listed)
	}
	logged.AwaitEvent(t, "listingStarted", "jobId=1", "waitedMs=")
	logged.AwaitEvent(t, "listingEnded", "jobId=1", "recordCount=", "tookMs=")
	core.commands <- nodeproto.Command{CommandSeq: 2, JobID: 2, Work: nodeproto.Work{
		Type: nodeproto.CommandExamine, Examine: &nodeproto.Examine{
			RootPath: root.Path(),
			Files:    []nodeproto.ExamineFile{{Path: nodeproto.Path{Text: "Arkon.mkv"}, Kind: nodeproto.ExamineVideo}},
		},
	}}
	examined := testutil.Receive(t, core.scans, "the examining")
	if examined.JobID != 2 || examined.Examined == nil || len(examined.Examined.Records) != 1 ||
		examined.Examined.Records[0].Observed == nil {
		t.Errorf("job 2's answer = %+v, want Arkon.mkv observed", examined)
	}
}

// A full rescan's examining checks the stores of chunks of each video it names to check against
// their records after reading it, and says what it found in a chunksChecked record: the files checked
// and each dropped; a video not named to check has its stores left alone (D392 §4).
func TestAgent_AnExaminingChecksTheChunksOfTheVideosAFullRescanNames(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	shelf, otherChunk := shelfWithADamagedChunkOfFiles7And8(t)
	root, _ := startAgentWithShelf(t, socket, shelf)
	for _, name := range []string{"Arkon.mkv", "Brisk.mkv"} {
		if err := os.WriteFile(filepath.Join(root.Path(), name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files := []nodeproto.ExamineFile{
		{Path: nodeproto.Path{Text: "Arkon.mkv"}, Kind: nodeproto.ExamineVideo, ChecksChunksOf: 7},
		{Path: nodeproto.Path{Text: "Brisk.mkv"}, Kind: nodeproto.ExamineVideo},
	}
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 1, Work: nodeproto.Work{
		Type: nodeproto.CommandExamine, Examine: &nodeproto.Examine{RootPath: root.Path(), Files: files},
	}}
	examined := testutil.Receive(t, core.scans, "the examining")
	if examined.Examined == nil || len(examined.Examined.Records) != 3 {
		t.Fatalf("the examining = %+v, want both videos observed and file 7's chunks checked", examined.Examined)
	}
	checked := examined.Examined.Records[2]
	if checked.Type != nodeproto.RecordChunksChecked || checked.ChunksChecked == nil ||
		checked.ChunksChecked.FileID != 7 || checked.ChunksChecked.CheckedCount != 2 ||
		len(checked.ChunksChecked.Dropped) != 1 || checked.ChunksChecked.Dropped[0].Name != "00000.m4s" {
		t.Errorf("the chunks checked = %+v (%+v), want file 7's two files checked and chunk 0 dropped", checked,
			checked.ChunksChecked)
	}
	if _, err := os.Stat(otherChunk); err != nil {
		t.Errorf("a video not named to check had its chunk dropped: %v", err)
	}
}

// shelfWithADamagedChunkOfFiles7And8 is a prepared shelf with a store of file 7 and one of file 8,
// each its init and chunk 0, the chunk then cut short; and the path of file 8's chunk.
func shelfWithADamagedChunkOfFiles7And8(t *testing.T) (*prepared.Shelf, string) {
	t.Helper()
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	var chunk string
	for _, storeDir := range []string{"7/chunks-picture/0123456789abcdef", "8/chunks-picture/0123456789abcdef"} {
		if _, err := shelf.OpenStore(t.Context(), nodeproto.StoreManifest{StoreDir: storeDir}); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"init.mp4", "00000.m4s"} {
			if err := shelf.PutFile(storeDir, name, []byte("the bytes of "+name)); err != nil {
				t.Fatal(err)
			}
		}
		chunk = filepath.Join(directory, filepath.FromSlash(storeDir), "00000.m4s")
		if err := os.WriteFile(chunk, []byte("the"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return shelf, chunk
}

// A library folder inside a root is listed and examined as a root of its own: its scan begins at its
// path, which the core compares, and names its files relative to it (D359 §1, §4).
func TestAgent_ListsAndExaminesALibraryFolderInsideARootReachedWithoutALink(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgentWithWorkers(t, socket, Workers{
		Scanner: fakeScanner{}, Artworker: fakeArtworker{}, Subtitler: fakeSubtitler{},
		ThroughReader: fakeThroughReader{}, ChunkMaker: fakeChunkMaker{},
	})
	movies := libraryFolderTree(t, root.Path())
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 1, Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: movies, Path: "."},
	}}
	listed := testutil.Receive(t, core.scans, "the folder's listing")
	beganAt, paths := beganAtAndListed(listed.Listing)
	if listed.Listing.Refusal != "" || beganAt != movies || !slices.Equal(paths, []string{"Arkon", "Arkon/Arkon.mkv"}) {
		t.Errorf("the folder's listing began at %q with %v (refusal %q); want it begun at %s with Arkon's film "+
			"alone, relative to it", beganAt, paths, listed.Listing.Refusal, movies)
	}
	core.commands <- nodeproto.Command{CommandSeq: 2, JobID: 2, Work: nodeproto.Work{
		Type: nodeproto.CommandExamine, Examine: &nodeproto.Examine{RootPath: movies, Files: []nodeproto.ExamineFile{
			{Path: nodeproto.Path{Text: "Arkon/Arkon.mkv"}, Kind: nodeproto.ExamineVideo},
		}},
	}}
	if examined := testutil.Receive(t, core.scans, "the folder's examining"); examined.Examined == nil ||
		len(examined.Examined.Records) != 1 || examined.Examined.Records[0].Observed == nil {
		t.Errorf("the folder's examining = %+v, want Arkon.mkv observed", examined.Examined)
	}
}

// libraryFolderTree makes video/movies/Arkon/Arkon.mkv, video/Dark.mkv and a link, films, to
// video/movies, under root, and returns video/movies' path.
func libraryFolderTree(t *testing.T, root string) string {
	t.Helper()
	movies := filepath.Join(root, "video", "movies")
	if err := os.MkdirAll(filepath.Join(movies, "Arkon"), 0o700); err != nil {
		t.Fatal(err)
	}
	for path, contents := range map[string]string{
		filepath.Join(movies, "Arkon", "Arkon.mkv"): "arkon", filepath.Join(root, "video", "Dark.mkv"): "dark",
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(movies, filepath.Join(root, "films")); err != nil {
		t.Fatal(err)
	}
	return movies
}

// beganAtAndListed are where a listing's scan began and the paths it listed.
func beganAtAndListed(listing *nodeproto.Listing) (string, []string) {
	if listing == nil {
		return "", nil
	}
	beganAt, paths := "", []string{}
	for _, record := range listing.Records {
		if record.Listed != nil {
			paths = append(paths, record.Listed.Path.Text)
		}
		if record.ScanBegin != nil {
			beganAt = record.ScanBegin.RootPath
		}
	}
	return beganAt, paths
}

// A library folder reached through a link, or a path climbing out of the root, is refused: a library
// folder is named by its real path, and never widens a root (D107, D359 §1).
func TestAgent_RefusesALibraryFolderReachedThroughALinkOrClimbingOutOfItsRoot(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	movies := libraryFolderTree(t, root.Path())
	for seq, refused := range []string{filepath.Join(root.Path(), "films"), movies + "/../../../etc"} {
		core.commands <- nodeproto.Command{CommandSeq: int64(3 + seq), JobID: int64(3 + seq), Work: nodeproto.Work{
			Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: refused, Path: "."},
		}}
		if answer := testutil.Receive(t, core.scans, "a refused listing"); answer.Listing == nil ||
			answer.Listing.Refusal == "" || len(answer.Listing.Records) != 0 {
			t.Errorf("listing %s = %+v, want it refused", refused, answer.Listing)
		}
	}
}

// A library folder inside a root that is gone is listed as a root that could not be read, not refused:
// its listing begins at the folder, says it was not found, and completes, so the core keeps its files
// and marks it unavailable until a listing reads it (D40, D241 §2). Refused, the folder's scan was
// given up and it stayed complete (staging, 2026-10-27, VQ81).
func TestAgent_ListsALibraryFolderThatIsGoneAsARootThatCouldNotBeRead(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	libraryFolderTree(t, root.Path())
	movies := filepath.Join(root.Path(), "video", "gone") // a folder renamed away since it was added
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 1, Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: movies, Path: "."},
	}}
	listing := testutil.Receive(t, core.scans, "the gone folder's listing").Listing
	if listing == nil || listing.Refusal != "" {
		t.Fatalf("the gone folder's listing = %+v; want it listed, not refused", listing)
	}
	beganAt, paths := beganAtAndListed(listing)
	var rootErrors []nodeproto.ScanError
	var completed *nodeproto.ScanComplete
	for _, record := range listing.Records {
		if record.Error != nil && record.Error.Scope == nodeproto.ScopeRoot {
			rootErrors = append(rootErrors, *record.Error)
		}
		if record.ScanComplete != nil {
			completed = record.ScanComplete
		}
	}
	if beganAt != movies || len(paths) != 0 || len(rootErrors) != 1 || rootErrors[0].Code != nodeproto.CodeNotFound ||
		completed == nil || completed.RootPath != movies {
		t.Errorf("the gone folder's listing began at %q, listed %v, with root errors %+v, completed %+v; want "+
			"it begun and completed at %s with nothing listed and one root error, not found", beganAt, paths,
			rootErrors, completed, movies)
	}
}

// A listing or an examining under a root the node was not given is refused, whatever the core asks
// (D107).
func TestAgent_RefusesToListOrExamineARootItWasNotGiven(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	startAgent(t, socket)
	core.commands <- nodeproto.Command{CommandSeq: 3, JobID: 3, Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: "/etc", Path: "."},
	}}
	if refused := testutil.Receive(t, core.scans, "the refused listing"); refused.Listing == nil ||
		refused.Listing.Refusal == "" || len(refused.Listing.Records) != 0 {
		t.Errorf("listing /etc = %+v, want it refused: the node was not given it", refused.Listing)
	}
	core.commands <- nodeproto.Command{CommandSeq: 4, JobID: 4, Work: nodeproto.Work{
		Type: nodeproto.CommandExamine, Examine: &nodeproto.Examine{RootPath: "/etc", Files: []nodeproto.ExamineFile{
			{Path: nodeproto.Path{Text: "passwd"}, Kind: nodeproto.ExamineNFO},
		}},
	}}
	if refused := testutil.Receive(t, core.scans, "the refused examining"); refused.Examined == nil ||
		refused.Examined.Refusal == "" || len(refused.Examined.Records) != 0 {
		t.Errorf("examining under /etc = %+v, want it refused", refused.Examined)
	}
}

// List lists as the node's scanner does.
func (fakeScanner) List(ctx context.Context, root *confine.MediaRoot, command nodeproto.List) (
	[]nodeproto.ScanRecord, error,
) {
	return realScanner().List(ctx, root, command)
}

// Examine examines as the node's scanner does, with a prober that finds every video a Matroska.
func (fakeScanner) Examine(
	ctx context.Context, root *confine.MediaRoot, files []nodeproto.ExamineFile, between func() error,
) ([]nodeproto.ScanRecord, error) {
	return realScanner().Examine(ctx, root, files, between)
}

// realScanner is the node's scanner over no other roots, probing nothing.
func realScanner() *scan.Scanner {
	return scan.NewScanner(nil, matroskaProber{}, scan.Options{})
}

// matroskaProber finds every file a Matroska.
type matroskaProber struct{}

func (matroskaProber) Probe(context.Context, execx.Film) (nodeproto.Probe, error) {
	return nodeproto.Probe{Format: nodeproto.ProbeFormat{FormatNames: []string{"matroska"}}}, nil
}

// hangingScanner lists nothing until released, as a listing of a disk that does not answer, which
// no cancel stops: it says when it starts, and when it returns. The test's end releases it too.
type hangingScanner struct {
	fakeScanner
	started, release, returned chan struct{}
	testCtx                    context.Context //nolint:containedctx // the test's, which bounds the hang
}

func (scanner hangingScanner) List(context.Context, *confine.MediaRoot, nodeproto.List) (
	[]nodeproto.ScanRecord, error,
) {
	close(scanner.started)
	select {
	case <-scanner.release:
	case <-scanner.testCtx.Done():
	}
	close(scanner.returned)
	return nil, nil
}

// A listing whose reading does not move for the node's read window is answered as stalled on its
// disk, and its reading, which the node cannot stop, is listed as stuck until it returns; what it
// returns then is dropped (D233 §10).
func TestAgent_AListingThatStopsMovingIsAnsweredStalledAndListedAsStuck(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	scanner := hangingScanner{
		started: make(chan struct{}), release: make(chan struct{}), returned: make(chan struct{}),
		testCtx: t.Context(),
	}
	var clockSource *clocktest.Fake
	root, tick := startAgentWithWorkers(t, socket, Workers{Scanner: scanner, ReadStallWindow: time.Minute},
		func(node *Agent) {
			fake, isFake := node.clock.(*clocktest.Fake)
			if !isFake {
				t.Fatal("the test agent's clock is not a fake")
			}
			clockSource = fake
		})
	tick <- time.Time{}
	testutil.Receive(t, core.statuses, "the first status")
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 1, Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: root.Path(), Path: "."},
	}}
	testutil.Receive(t, scanner.started, "the listing's start")
	clockSource.Advance(time.Minute)
	tick <- time.Time{}
	stalled := testutil.Receive(t, core.scans, "the stalled listing's answer")
	if stalled.JobID != 1 || stalled.Listing == nil || stalled.Listing.Stall == nil ||
		stalled.Listing.Stall.Condition != nodeproto.ConditionWaitingOnDisk || !stalled.Listing.Stall.IsLeftRunning {
		t.Fatalf("job 1's answer = %+v; want a listing stalled on its disk, left running", stalled)
	}
	stuck := statusUntil(t, core, tick, func(status nodeproto.Status) bool { return len(status.Stuck) > 0 }).Stuck
	if len(stuck) != 1 || stuck[0].Tool != stuckReadTool || stuck[0].Paths[0] != root.Path() {
		t.Errorf("the status's stuck = %+v; want the listing's reading of %s", stuck, root.Path())
	}
	close(scanner.release)
	testutil.Receive(t, scanner.returned, "the listing's return")
	statusUntil(t, core, tick, func(status nodeproto.Status) bool { return len(status.Stuck) == 0 })
	select {
	case late := <-core.scans:
		t.Errorf("the stuck listing answered again once it returned: %+v", late)
	default:
	}
}

// statusUntil ticks until the node posts a status that holds, and returns it.
func statusUntil(
	t *testing.T, core *fakeCore, tick chan time.Time, holds func(nodeproto.Status) bool,
) nodeproto.Status {
	t.Helper()
	for range 100 {
		tick <- time.Time{}
		if status := testutil.Receive(t, core.statuses, "a status"); holds(status) {
			return status
		}
	}
	t.Fatal("no status held")
	return nodeproto.Status{}
}
