// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	corenodes "github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

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

// systemVideosWorld is the films harness, its node local connected with its system folder among its
// roots, and the system videos over it.
type systemVideosWorld struct {
	*harness
	nodes    *fakeNodes
	folders  *fakeSystemFolders
	queue    *fakeMakeJobs
	assigner *fakeAssigner
	videos   *SystemVideos
}

func newSystemVideosWorld(t *testing.T) *systemVideosWorld {
	t.Helper()
	world := &systemVideosWorld{
		harness: newHarness(t), folders: &fakeSystemFolders{}, queue: &fakeMakeJobs{}, assigner: &fakeAssigner{},
	}
	world.nodes = &fakeNodes{id: world.nodeID, roots: []string{"/media", systemFolder}}
	world.folders.folders = []corenodes.SystemFolder{{NodeID: world.nodeID, NodeName: "local", Path: systemFolder}}
	admin := NewAdmin(world.database, world.clock, world.nodes, world.nodes, retirementOf(&fakeWork{}))
	admin.ListFoldersThrough(readableFolders())
	world.videos = NewSystemVideos(SystemVideosParts{
		Database: world.database, Clock: world.clock, Admin: admin, Folders: world.folders, Jobs: world.queue,
		Assigner: world.assigner, Scans: world.nodes,
	})
	return world
}

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
	library := storetest.QueryRaw(t, world.database.Path(), "SELECT library.kind, library_root.root_path "+
		"FROM system_library JOIN library ON library.id = system_library.library_id "+
		"JOIN library_root ON library_root.library_id = library.id")
	if !slices.Equal(library, []string{"other|" + systemFolder}) {
		t.Errorf("the system library = %v; want an other library over the node's system folder", library)
	}
	command := world.assigner.commands[0].MakeSystemVideos
	if world.assigner.nodeIDs[0] != world.nodeID || command == nil ||
		!slices.Equal(command.Videos, SystemVideoList()) || command.IsRebuild {
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

// With no node connected that has a system folder, the feature is unavailable, and no library is made
// (D435 §5).
func TestSystemVideos_NoNodeToHoldThemIsUnavailable(t *testing.T) {
	t.Parallel()
	world := newSystemVideosWorld(t)
	world.folders.folders = nil
	_, err := world.videos.Find(t.Context(), nodeproto.SystemVideoSoundTiming, nodeproto.SystemVideoH264AAC)
	if !errors.Is(err, ErrSystemVideosUnavailable) {
		t.Errorf("a use with no node = %v; want unavailable", err)
	}
	if got := storetest.QueryRaw(t, world.database.Path(), "SELECT count(*) FROM system_library"); got[0] != "0" {
		t.Errorf("system libraries = %v; want none made", got)
	}
	if _, err = world.videos.Find(t.Context(), "soundTiming", "vp9-opus"); !errors.Is(err, ErrNoSuchSystemVideo) {
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
	world.scan([]nodeproto.ScanRecord{observedRecord("Arkon (1979)/Arkon.mkv", "arkon")}, true)
	clip := world.liveFilesByPath()[systemFolder+"/soundTiming.h264-aac.mp4"]
	film := world.liveFilesByPath()[root+"/Arkon (1979)/Arkon.mkv"]
	if clip.ID == 0 || film.ID == 0 {
		t.Fatalf("files %+v; want the clip and the film scanned", world.liveFilesByPath())
	}
	none := auth.LibrariesOnly()
	if err := ReachFile(ctx, world.database.Read(), none, clip.ID); err != nil {
		t.Errorf("the clip for a person who reaches no library = %v; want it reached", err)
	}
	if err := ReachFile(ctx, world.database.Read(), none, film.ID); !errors.Is(err, ErrNoFile) {
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
		work, err := NewBrowser(world.database, config).WorkOfFile(ctx, reach, clip.ID)
		if err != nil || work.LibraryID != 0 || work.LibraryName != "" {
			t.Errorf("the clip's work = %+v, %v; want it with no library", work, err)
		}
	}
}
