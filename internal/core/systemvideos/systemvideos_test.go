// SPDX-License-Identifier: AGPL-3.0-or-later

package systemvideos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	corenodes "github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// root is the films library's one root, as its node names it.
const root = "/media/films"

// systemFolder is node local's system folder in these tests.
const systemFolder = "/var/orpheum/system-videos"

// fakeSystemFolders are the connected nodes' system folders, as a test sets them.
type fakeSystemFolders struct {
	folders []corenodes.SystemFolder
}

func (folders *fakeSystemFolders) ConnectedSystemFolders(context.Context) []corenodes.SystemFolder {
	return folders.folders
}

// fakeMakeJobs keeps the jobs queued.
type fakeMakeJobs struct {
	queued []jobs.Request
}

func (queue *fakeMakeJobs) Enqueue(_ context.Context, job jobs.Request) (bool, error) {
	queue.queued = append(queue.queued, job)
	return true, nil
}

// fakeAssigner keeps the work given to each node.
type fakeAssigner struct {
	nodeIDs  []int64
	commands []nodeproto.Command
}

func (assigner *fakeAssigner) Assign(_ context.Context, nodeID int64, _ jobs.Job, command nodeproto.Command) error {
	assigner.nodeIDs = append(assigner.nodeIDs, nodeID)
	assigner.commands = append(assigner.commands, command)
	return nil
}

// fakeNodes is the node channel with one connected node, "local", that named its roots, and any
// others: the nodes and the scan requests the library's admin asks.
type fakeNodes struct {
	id    int64
	roots []string
	// others are other connected nodes by name, each with roots.
	others map[string]*fakeNodes
	// scanned are the roots a person asked to scan.
	scanned []string
}

func (nodes *fakeNodes) NodeConnection(context.Context, int64) (corenodes.Connection, time.Time) {
	return corenodes.Connected, time.Time{}
}

func (nodes *fakeNodes) ConnectedNodeRoots(_ context.Context, name string) (int64, []string, bool) {
	if other, isOther := nodes.others[name]; isOther {
		return other.id, other.roots, true
	}
	if name != "local" {
		return 0, nil, false
	}
	return nodes.id, nodes.roots, true
}

func (nodes *fakeNodes) RemoveNode(context.Context, int64) error { return nil }

func (nodes *fakeNodes) RootWatches(int64) map[string]nodeproto.RootWatch { return nil }

func (nodes *fakeNodes) WatchedFoldersChanged() {}

func (nodes *fakeNodes) InProgress(context.Context) (map[library.ScanRoot]library.ScanProgress, error) {
	return map[library.ScanRoot]library.ScanProgress{}, nil
}

func (nodes *fakeNodes) Request(ctx context.Context, nodeID int64, root string, priority int64) error {
	for _, other := range nodes.others {
		if other.id == nodeID {
			return other.Request(ctx, nodeID, root, priority)
		}
	}
	if nodeID == nodes.id && priority == jobs.PriorityPersonsRequest {
		nodes.scanned = append(nodes.scanned, root)
	}
	return nil
}

func (nodes *fakeNodes) RequestUnder(context.Context, int64, string, string, int64) error { return nil }

func (nodes *fakeNodes) RequestReadingAgain(
	context.Context, int64, string, string, library.ReadAgain, int64,
) error {
	return nil
}

func (nodes *fakeNodes) RequestDiscarding(
	context.Context, int64, string, string, library.ReadAgain, int64,
) error {
	return nil
}

// fakeWork is the job queue, keeping nothing a retire asks it to cancel.
type fakeWork struct{}

func (fakeWork) CancelRetired(_ context.Context, retired jobs.RetiredWork) (int, error) {
	return len(retired.FileIDs), nil
}

// readableFolders lists a node's folders as its byte socket would: readable, and empty.
type readableFolders struct{}

func (readableFolders) ListFolders(context.Context, int64, string) (nodeproto.FolderListing, error) {
	return nodeproto.FolderListing{State: nodeproto.ProbeReadable, Folders: []string{}}, nil
}

// systemVideosWorld is a database with node local and a films library over root, its node connected
// with its system folder among its roots, the ingest, and the system videos over it.
type systemVideosWorld struct {
	t        *testing.T
	database *store.Database
	clock    *clocktest.Fake
	nodeID   int64
	ingester *library.Ingester
	nodes    *fakeNodes
	folders  *fakeSystemFolders
	queue    *fakeMakeJobs
	assigner *fakeAssigner
	videos   *Service
}

func newSystemVideosWorld(t *testing.T) *systemVideosWorld {
	t.Helper()
	database := storetest.OpenMigrated(t)
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	nodeID, _ := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: root})
	world := &systemVideosWorld{
		t: t, database: database, clock: fakeClock, nodeID: nodeID,
		ingester: library.NewIngester(
			database, fakeClock, settingstest.Seeded(t, database), slog.New(slog.DiscardHandler),
		),
		folders: &fakeSystemFolders{}, queue: &fakeMakeJobs{}, assigner: &fakeAssigner{},
	}
	world.nodes = &fakeNodes{id: world.nodeID, roots: []string{"/media", systemFolder}}
	world.folders.folders = []corenodes.SystemFolder{{NodeID: world.nodeID, NodeName: "local", Path: systemFolder}}
	retirement := library.Retirement{Work: fakeWork{}}
	admin := library.NewAdmin(world.database, world.clock, world.nodes, world.nodes, retirement)
	admin.ListFoldersThrough(readableFolders{})
	world.videos = New(Parts{
		Database: world.database, Clock: world.clock, Admin: admin, Folders: world.folders, Jobs: world.queue,
		Assigner: world.assigner, Scans: world.nodes,
	})
	return world
}

// hashOf is a stand-in payload hash for a file's content.
func hashOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// observedRecord is a video observed at path with content, read whole.
func observedRecord(path, content string) nodeproto.ScanRecord {
	return nodeproto.ScanRecord{Type: nodeproto.RecordObserved, Observed: &nodeproto.Observed{
		Path: nodeproto.Path{Text: path}, SizeBytes: int64(len(content)), ModifiedAt: 1,
		Identity: nodeproto.Identity{
			Scheme: "payload-v1", Anchor: "whole-file", PayloadHash: hashOf(content), PayloadBytes: 1,
		},
		Probe: nodeproto.Probe{Streams: []nodeproto.ProbeStream{
			{StreamIndex: 0, Kind: "video", Codec: "h264"},
			{
				StreamIndex: 1, Kind: "audio", Codec: "aac", ContainerLanguage: "ger", Language: "de",
				ContainerTitle: "Deutsch",
				Disposition:    []string{"default", "forced"}, Audio: nodeproto.ProbeAudio{ChannelCount: 6},
			},
			{StreamIndex: 2, Kind: "data", Codec: "bin_data"},
		}},
	}}
}

// scan sends one complete scan of the films library's root.
func (world *systemVideosWorld) scan(records []nodeproto.ScanRecord) {
	world.t.Helper()
	world.clock.Advance(time.Minute)
	facts := nodeproto.ScanBegin{RootPath: root, FilesystemDevice: 7, IsMountPoint: true}
	if err := world.ingester.IngestScan(world.t.Context(), world.nodeID, root,
		librarytest.Scan(facts, records, true)); err != nil {
		world.t.Fatal(err)
	}
}

// liveFilesByPath is the live file rows of node local by path.
func (world *systemVideosWorld) liveFilesByPath() map[string]sqlcgen.ListLiveFilesByNodeRow {
	world.t.Helper()
	rows, err := world.database.Read().ListLiveFilesByNode(world.t.Context(), world.nodeID)
	if err != nil {
		world.t.Fatal(err)
	}
	byPath := map[string]sqlcgen.ListLiveFilesByNodeRow{}
	for _, row := range rows {
		byPath[row.Path] = row
	}
	return byPath
}

func itoa(value int64) string { return strconv.FormatInt(value, 10) }

// makeVideos runs the make job last queued: given to its node, answered as made into folder.
func (world *systemVideosWorld) makeVideos(nodeID int64, folder string) {
	world.t.Helper()
	request := world.queue.queued[len(world.queue.queued)-1]
	job := jobs.Job{ID: 1, Kind: request.Kind, ArgsJSON: request.ArgsJSON}
	if err := world.videos.HandleMakeSystemVideos(world.t.Context(), job); err != nil {
		world.t.Fatal(err)
	}
	made := nodeproto.Result{SystemVideos: &nodeproto.SystemVideosMade{
		Folder: folder, FileNames: []string{"soundTiming.h264-aac.mp4"},
	}}
	err := world.videos.Assignee(nil).Answer(world.t.Context(), job, corenodes.Attempt{NodeID: nodeID}, made)
	if err != nil {
		world.t.Fatal(err)
	}
}

// scanSystemFolder is the scan of the system folder on node nodeID finding the clip.
func (world *systemVideosWorld) scanSystemFolder(nodeID int64) {
	world.t.Helper()
	facts := nodeproto.ScanBegin{RootPath: systemFolder, FilesystemDevice: 8}
	clip := observedRecord("soundTiming.h264-aac.mp4", "clip")
	records := librarytest.Scan(facts, []nodeproto.ScanRecord{clip}, true)
	if err := world.ingester.IngestScan(world.t.Context(), nodeID, systemFolder, records); err != nil {
		world.t.Fatal(err)
	}
}

// requireMadeOnLocal fails the test unless the make made the system library over local's system
// folder, gave local every system video to make, and asked for that folder's scan once made.
func (world *systemVideosWorld) requireMadeOnLocal() {
	t := world.t
	t.Helper()
	systemLibrary := storetest.QueryRaw(t, world.database.Path(), "SELECT library.kind, library_root.root_path "+
		"FROM system_library JOIN library ON library.id = system_library.library_id "+
		"JOIN library_root ON library_root.library_id = library.id")
	if !slices.Equal(systemLibrary, []string{"other|" + systemFolder}) {
		t.Errorf("the system library = %v; want an other library over the node's system folder", systemLibrary)
	}
	command := world.assigner.commands[0].MakeSystemVideos
	if world.assigner.nodeIDs[0] != world.nodeID || command == nil ||
		!slices.Equal(command.Videos, List()) || command.IsRebuild {
		t.Errorf("gave %+v to node %v; want every system video made on local", command, world.assigner.nodeIDs)
	}
	if !slices.Equal(world.nodes.scanned, []string{systemFolder}) {
		t.Errorf("scanned %v once made; want the system folder", world.nodes.scanned)
	}
}

// The first use of a system video makes the system library over a connected node's system folder and
// asks that node to make the videos; what it made is scanned, and the next use finds the file (D435
// §4, §6).
func TestSystemVideos_TheFirstUseMakesTheLibraryAndTheNodeMakesTheVideos(t *testing.T) {
	t.Parallel()
	world := newSystemVideosWorld(t)
	ctx := t.Context()
	state, err := world.videos.Find(ctx, nodeproto.SystemVideoSoundTiming, nodeproto.SystemVideoH264AAC)
	if err != nil || state.FileID != 0 || len(world.queue.queued) != 1 {
		t.Fatalf("the first use = %+v, %v, queued %+v; want it being made", state, err, world.queue.queued)
	}
	path := world.database.Path()
	if got := storetest.QueryRaw(t, path, "SELECT count(*) FROM system_library"); got[0] != "0" {
		t.Errorf("system libraries = %v after a GET's use; want it made by the job, a GET only reads", got)
	}
	world.makeVideos(world.nodeID, systemFolder)
	world.requireMadeOnLocal()
	world.scanSystemFolder(world.nodeID)
	state, err = world.videos.Find(ctx, nodeproto.SystemVideoSoundTiming, nodeproto.SystemVideoH264AAC)
	if err != nil || state.FileID == 0 || state.Video.FileName != "soundTiming.h264-aac.mp4" {
		t.Errorf("the use after the scan = %+v, %v; want the clip's file", state, err)
	}
	if len(world.queue.queued) != 1 {
		t.Errorf("queued %d makes; want none once the file is there", len(world.queue.queued))
	}
	if err := world.videos.Rebuild(ctx); err != nil || world.queue.queued[1].ArgsJSON != `{"isRebuild":true}` {
		t.Errorf("a rebuild = %v, queued %+v; want a make that makes them again", err, world.queue.queued)
	}
}

// A system video removed from its folder after it was made is made again: the scan that finds it
// gone leaves no file for the next use, which queues the make, and the make's scan finds it there
// once more (D510 §1, D435 §4).
func TestSystemVideos_AVideoRemovedFromItsFolderIsMadeAgain(t *testing.T) {
	t.Parallel()
	world := newSystemVideosWorld(t)
	ctx := t.Context()
	if _, err := world.videos.Find(ctx, nodeproto.SystemVideoSoundTiming, nodeproto.SystemVideoH264AAC); err != nil {
		t.Fatal(err)
	}
	world.makeVideos(world.nodeID, systemFolder)
	world.scanSystemFolder(world.nodeID)
	world.clock.Advance(time.Minute) // a later scan, which finds the folder without it
	facts := nodeproto.ScanBegin{RootPath: systemFolder, FilesystemDevice: 8}
	if err := world.ingester.IngestScan(ctx, world.nodeID, systemFolder,
		librarytest.Scan(facts, nil, true)); err != nil {
		t.Fatal(err)
	}
	state, err := world.videos.Find(ctx, nodeproto.SystemVideoSoundTiming, nodeproto.SystemVideoH264AAC)
	if err != nil || state.FileID != 0 || len(world.queue.queued) != 2 {
		t.Fatalf("the use after the clip went = %+v, %v, queued %+v; want it being made again", state, err,
			world.queue.queued)
	}
	world.makeVideos(world.nodeID, systemFolder)
	world.scanSystemFolder(world.nodeID)
	state, err = world.videos.Find(ctx, nodeproto.SystemVideoSoundTiming, nodeproto.SystemVideoH264AAC)
	if err != nil || state.FileID == 0 || len(world.queue.queued) != 2 {
		t.Errorf("the use after it was made again = %+v, %v, queued %d makes; want the clip's file and no "+
			"further make", state, err, len(world.queue.queued))
	}
}

// With no node connected that has a system folder, the feature is unavailable, and no library is made
// (D435 §5).
func TestSystemVideos_NoNodeToHoldThemIsUnavailable(t *testing.T) {
	t.Parallel()
	world := newSystemVideosWorld(t)
	world.folders.folders = nil
	_, err := world.videos.Find(t.Context(), nodeproto.SystemVideoSoundTiming, nodeproto.SystemVideoH264AAC)
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("a use with no node = %v; want unavailable", err)
	}
	if got := storetest.QueryRaw(t, world.database.Path(), "SELECT count(*) FROM system_library"); got[0] != "0" {
		t.Errorf("system libraries = %v; want none made", got)
	}
	if _, err = world.videos.Find(t.Context(), "soundTiming", "vp9-opus"); !errors.Is(err, ErrNoSuchVideo) {
		t.Errorf("a video no recipe makes = %v; want no such video", err)
	}
}

// When the system library's node is gone, the next use repoints the library to another connected
// node's system folder (D429) and makes the videos there (D435 §5).
func TestSystemVideos_ANodeGoneMovesTheLibraryToAnother(t *testing.T) {
	t.Parallel()
	world := newSystemVideosWorld(t)
	ctx := t.Context()
	if _, err := world.videos.Find(ctx, nodeproto.SystemVideoSoundTiming, nodeproto.SystemVideoH264AAC); err != nil {
		t.Fatal(err)
	}
	world.makeVideos(world.nodeID, systemFolder)
	world.scanSystemFolder(world.nodeID)
	const otherFolder = "/srv/orpheum/system-videos"
	otherID := librarytest.Node(t, world.database, "other")
	world.nodes.others = map[string]*fakeNodes{"other": {id: otherID, roots: []string{otherFolder}}}
	world.folders.folders = []corenodes.SystemFolder{{NodeID: otherID, NodeName: "other", Path: otherFolder}}
	state, err := world.videos.Find(ctx, nodeproto.SystemVideoSoundTiming, nodeproto.SystemVideoH264AAC)
	if err != nil || state.FileID != 0 {
		t.Fatalf("the use after local went = %+v, %v; want it made again on other", state, err)
	}
	world.makeVideos(otherID, otherFolder)
	roots := storetest.QueryRaw(t, world.database.Path(), "SELECT node_id, root_path FROM library_root "+
		"WHERE library_id = (SELECT library_id FROM system_library)")
	if !slices.Equal(roots, []string{itoa(otherID) + "|" + otherFolder}) {
		t.Errorf("the system library's folders = %v; want other's system folder alone", roots)
	}
	if world.assigner.nodeIDs[len(world.assigner.nodeIDs)-1] != otherID {
		t.Errorf("the make went to node %v; want other", world.assigner.nodeIDs)
	}
}

// Any signed-in person may play a system library's file, though no role reaches that library; an
// ordinary library a person does not reach is still refused (D435 §6, D243 §4).
func TestReachFile_ASystemVideoIsReachedByAnyoneAndNothingElseIs(t *testing.T) {
	t.Parallel()
	world := newSystemVideosWorld(t)
	ctx := t.Context()
	if _, err := world.videos.Find(ctx, nodeproto.SystemVideoSoundTiming, nodeproto.SystemVideoH264AAC); err != nil {
		t.Fatal(err)
	}
	world.makeVideos(world.nodeID, systemFolder)
	world.scanSystemFolder(world.nodeID)
	world.scan([]nodeproto.ScanRecord{observedRecord("Arkon (1979)/Arkon.mkv", "arkon")})
	clip := world.liveFilesByPath()[systemFolder+"/soundTiming.h264-aac.mp4"]
	film := world.liveFilesByPath()[root+"/Arkon (1979)/Arkon.mkv"]
	if clip.ID == 0 || film.ID == 0 {
		t.Fatalf("files %+v; want the clip and the film scanned", world.liveFilesByPath())
	}
	none := auth.LibrariesOnly()
	if err := library.ReachFile(ctx, world.database.Read(), none, clip.ID); err != nil {
		t.Errorf("the clip for a person who reaches no library = %v; want it reached", err)
	}
	if err := library.ReachFile(ctx, world.database.Read(), none, film.ID); !errors.Is(err, library.ErrNoFile) {
		t.Errorf("a film of a library the person does not reach = %v; want it refused as absent", err)
	}
}

// A system video is what a playback plays with no library named: the system library is no page's
// (D435 §1), so the Play page's trail goes nowhere for it, and starting it is no error. An admin
// reaches every library, the system one among them.
func TestWorkOfFile_ASystemVideoNamesNoLibrary(t *testing.T) {
	t.Parallel()
	world := newSystemVideosWorld(t)
	ctx := t.Context()
	if _, err := world.videos.Find(ctx, nodeproto.SystemVideoSoundTiming, nodeproto.SystemVideoH264AAC); err != nil {
		t.Fatal(err)
	}
	world.makeVideos(world.nodeID, systemFolder)
	world.scanSystemFolder(world.nodeID)
	clip := world.liveFilesByPath()[systemFolder+"/soundTiming.h264-aac.mp4"]
	if clip.ID == 0 {
		t.Fatalf("files %+v; want the clip scanned", world.liveFilesByPath())
	}
	config := settingstest.Seeded(t, world.database)
	for _, reach := range []auth.Reach{auth.EveryLibrary(), auth.LibrariesOnly()} {
		work, err := library.NewBrowser(world.database, config).WorkOfFile(ctx, reach, library.Opened{FileID: clip.ID})
		if err != nil || work.LibraryID != 0 || work.LibraryName != "" {
			t.Errorf("the clip's work = %+v, %v; want it with no library", work, err)
		}
	}
}
