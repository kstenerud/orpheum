// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/platform/strictjson"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

var (
	// ErrSystemVideosUnavailable is no connected node with a system folder: nothing can make or hold the
	// server's own videos now (D435 §5).
	ErrSystemVideosUnavailable = errors.New("no connected node can hold the server's own videos")
	// ErrNoSuchSystemVideo is a purpose and codec no system video has.
	ErrNoSuchSystemVideo = errors.New("no system video is that purpose in that codec")
)

// systemLibraryName is the system library's name, which no listing shows (D435 §1).
const systemLibraryName = "System videos"

// SystemVideoList are the server's own videos, each named by what it is and the file the core names
// for it in the system folder: the one list (D435 §6). A new video adds its recipe to the node's
// systemvideo package too.
func SystemVideoList() []nodeproto.SystemVideo {
	videos := []nodeproto.SystemVideo{{Purpose: nodeproto.SystemVideoSoundTiming, Codec: nodeproto.SystemVideoH264AAC}}
	for index := range videos {
		videos[index].FileName = videos[index].Purpose + "." + videos[index].Codec + ".mp4"
	}
	return videos
}

// SystemFolders are the connected nodes' system folders: the node service's ConnectedSystemFolders.
type SystemFolders interface {
	ConnectedSystemFolders(ctx context.Context) []nodes.SystemFolder
}

// SystemVideosParts are what the system videos are kept with.
type SystemVideosParts struct {
	Database *store.Database
	Clock    clock.Clock
	// Admin adds the library's folder and repoints it (D429), the one way a library folder moves.
	Admin    *Admin
	Folders  SystemFolders
	Jobs     ScanJobs
	Assigner Assigner
	Scans    ScanRequests
	Logger   *slog.Logger
}

// SystemVideos keeps the server's own videos in the hidden system library (D435): made on a node on
// their first use, found by what they are, moved to another node when theirs is gone.
type SystemVideos struct {
	parts SystemVideosParts
}

// NewSystemVideos returns the system videos.
func NewSystemVideos(parts SystemVideosParts) *SystemVideos {
	return &SystemVideos{parts: parts}
}

// SystemVideoState is one system video: what it is, and its file once a scan found it, zero while it
// is being made.
type SystemVideoState struct {
	Video     nodeproto.SystemVideo
	FileID    int64
	SizeBytes int64
}

// SystemVideosView is the system library as the debug view shows it (D435 §7): its node and folder,
// and each video.
type SystemVideosView struct {
	NodeName string
	Folder   string
	Videos   []SystemVideoState
}

// systemLibrary is the system library's row: its folder on its node, both zero when it has none live.
type systemLibrary struct {
	id       int64
	nodeID   int64
	nodeName string
	folder   string
}

// makeArgs are a make_system_videos job's arguments.
type makeArgs struct {
	IsRebuild bool `json:"isRebuild,omitempty"`
}

// Find is the video of that purpose in that codec, its file once made; while it is being made, its
// FileID is zero and a make is queued, which makes the library on its first use and moves it to
// another connected node when its own is gone (D435 §5, §6). Find itself only reads and queues: a
// play's GET writes nothing else (D303).
func (videos *SystemVideos) Find(ctx context.Context, purpose, codec string) (SystemVideoState, error) {
	index := slices.IndexFunc(SystemVideoList(), func(video nodeproto.SystemVideo) bool {
		return video.Purpose == purpose && video.Codec == codec
	})
	if index < 0 {
		return SystemVideoState{}, fmt.Errorf("%s/%s: %w", purpose, codec, ErrNoSuchSystemVideo)
	}
	view, isPlaced, err := videos.current(ctx)
	if err != nil {
		return SystemVideoState{}, err
	}
	state := view.Videos[index]
	if !isPlaced {
		state.FileID, state.SizeBytes = 0, 0
	}
	if state.FileID == 0 {
		err = videos.queueMake(ctx, false)
	}
	return state, err
}

// View is the system library and its videos as they are: no node and no files before the first use.
func (videos *SystemVideos) View(ctx context.Context) (SystemVideosView, error) {
	library, err := videos.read(ctx)
	if err != nil {
		return SystemVideosView{}, err
	}
	return videos.view(ctx, library)
}

// Rebuild makes every system video again, over the files there (D435 §7).
func (videos *SystemVideos) Rebuild(ctx context.Context) error {
	if _, _, err := videos.current(ctx); err != nil {
		return err
	}
	return videos.queueMake(ctx, true)
}

// queueMake queues the make of the system videos.
func (videos *SystemVideos) queueMake(ctx context.Context, isRebuild bool) error {
	args, err := json.Marshal(makeArgs{IsRebuild: isRebuild})
	if err != nil {
		return fmt.Errorf("encode a system video make's arguments: %w", err)
	}
	_, err = videos.parts.Jobs.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindMakeSystemVideos, ArgsJSON: string(args), Priority: jobs.PriorityPersonsRequest,
		Reason: "makes the server's own videos, which someone asked for (D435)",
	})
	if err != nil {
		return fmt.Errorf("queue the system videos' make: %w", err)
	}
	return nil
}

// current is the system library's videos, and whether the library is on a connected node's system
// folder; with no such folder anywhere, the videos are unavailable (D435 §5).
func (videos *SystemVideos) current(ctx context.Context) (SystemVideosView, bool, error) {
	library, err := videos.read(ctx)
	if err != nil {
		return SystemVideosView{}, false, err
	}
	folders := videos.parts.Folders.ConnectedSystemFolders(ctx)
	isPlaced := slices.ContainsFunc(folders, func(folder nodes.SystemFolder) bool {
		return folder.NodeID == library.nodeID && folder.Path == library.folder
	})
	if !isPlaced && len(folders) == 0 {
		return SystemVideosView{}, false, ErrSystemVideosUnavailable
	}
	view, err := videos.view(ctx, library)
	return view, isPlaced, err
}

// ensure is the system library on a connected node's system folder: made when there is none, and
// repointed to another node's when its node is gone or its folder moved (D435 §5).
func (videos *SystemVideos) ensure(ctx context.Context) (systemLibrary, error) {
	library, err := videos.read(ctx)
	if err != nil {
		return systemLibrary{}, err
	}
	folders := videos.parts.Folders.ConnectedSystemFolders(ctx)
	if slices.ContainsFunc(folders, func(folder nodes.SystemFolder) bool {
		return folder.NodeID == library.nodeID && folder.Path == library.folder
	}) {
		return library, nil
	}
	if len(folders) == 0 {
		return systemLibrary{}, ErrSystemVideosUnavailable
	}
	if err := videos.place(ctx, library, folders[0]); err != nil {
		return systemLibrary{}, err
	}
	return videos.read(ctx)
}

// place puts the system library on folder: made there when there is none, its folder repointed there
// when it has one live, and added there when it has none (D429; D435 §5).
func (videos *SystemVideos) place(ctx context.Context, library systemLibrary, folder nodes.SystemFolder) error {
	logger := videos.logger()
	switch {
	case library.id == 0:
		logger.InfoContext(ctx, "made the system library", "nodeName", folder.NodeName, "folder", folder.Path)
		return videos.create(ctx, folder)
	case library.nodeID == 0:
		logger.InfoContext(ctx, "gave the system library a folder", "nodeName", folder.NodeName, "folder", folder.Path)
		return videos.parts.Admin.AddRoot(ctx, library.id, folder.NodeName, folder.Path)
	}
	logger.InfoContext(ctx, "moved the system library to another node's system folder",
		"nodeName", library.nodeName, "folder", library.folder,
		"newNodeName", folder.NodeName, "newFolder", folder.Path)
	_, err := videos.parts.Admin.Repoint(ctx, FolderMove{
		NodeName: library.nodeName, Path: library.folder, NewNodeName: folder.NodeName, NewPath: folder.Path,
	}, 0)
	return err
}

// create makes the system library over folder, unless another use made one first.
func (videos *SystemVideos) create(ctx context.Context, folder nodes.SystemFolder) error {
	err := videos.parts.Database.Write(ctx, func(queries *sqlcgen.Queries) error {
		_, err := store.OrNotFound(queries.GetSystemLibrary(ctx))
		if err == nil {
			return nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("read the system library: %w", err)
		}
		id, err := queries.InsertLibrary(ctx, sqlcgen.InsertLibraryParams{
			Name: systemLibraryName, Kind: LibraryKindOther, MetadataSource: "nfo", ChangeDetection: "manual",
			CreatedAt: videos.parts.Clock.Now().UnixMilli(),
		})
		if err != nil {
			return fmt.Errorf("create the system library: %w", err)
		}
		err = queries.InsertLibraryRoot(ctx, sqlcgen.InsertLibraryRootParams{
			LibraryID: id, NodeID: folder.NodeID, Path: folder.Path,
		})
		if err != nil {
			return fmt.Errorf("add the system library's folder: %w", err)
		}
		return queries.InsertSystemLibrary(ctx, id)
	})
	if err != nil {
		return fmt.Errorf("make the system library: %w", err)
	}
	return nil
}

// read is the system library's row, zero when there is none.
func (videos *SystemVideos) read(ctx context.Context) (systemLibrary, error) {
	row, err := store.OrNotFound(videos.parts.Database.Read().GetSystemLibrary(ctx))
	if errors.Is(err, store.ErrNotFound) {
		return systemLibrary{}, nil
	}
	if err != nil {
		return systemLibrary{}, fmt.Errorf("read the system library: %w", err)
	}
	library := systemLibrary{id: row.ID}
	if row.NodeID != nil && row.RootPath != nil && row.NodeName != nil {
		library.nodeID, library.folder, library.nodeName = *row.NodeID, *row.RootPath, *row.NodeName
	}
	return library, nil
}

// view is the library's videos, each with its file where a scan found it in the library's folder.
func (videos *SystemVideos) view(ctx context.Context, library systemLibrary) (SystemVideosView, error) {
	found, err := videos.parts.Database.Read().ListSystemVideos(ctx)
	if err != nil {
		return SystemVideosView{}, fmt.Errorf("list the system videos: %w", err)
	}
	view := SystemVideosView{NodeName: library.nodeName, Folder: library.folder}
	for _, video := range SystemVideoList() {
		state := SystemVideoState{Video: video}
		for _, file := range found {
			if file.NodeID == library.nodeID && file.Path == path.Join(library.folder, video.FileName) &&
				file.FileID != nil {
				state.FileID, state.SizeBytes = *file.FileID, ptr.ValueOrZero(file.SizeBytes)
			}
		}
		view.Videos = append(view.Videos, state)
	}
	return view, nil
}

// HandleMakeSystemVideos puts the system library on a connected node's system folder, making or moving
// it as it must, and gives the make of the system videos (jobs.KindMakeSystemVideos) to that node,
// which makes them there. With no node connected to hold them, the job ends: the next use asks again
// (D435 §5).
func (videos *SystemVideos) HandleMakeSystemVideos(ctx context.Context, job jobs.Job) error {
	var args makeArgs
	if err := strictjson.Unmarshal([]byte(job.ArgsJSON), &args); err != nil {
		return jobs.MarkPermanent(fmt.Errorf("a system video make's arguments %q: %w", job.ArgsJSON, err))
	}
	library, err := videos.ensure(ctx)
	if errors.Is(err, ErrSystemVideosUnavailable) {
		return jobs.MarkPermanent(err)
	}
	if err != nil {
		return err
	}
	return videos.parts.Assigner.Assign(ctx, library.nodeID, job, nodeproto.Command{Work: nodeproto.Work{
		Type: nodeproto.CommandMakeSystemVideos,
		MakeSystemVideos: &nodeproto.MakeSystemVideos{
			Videos: SystemVideoList(), IsRebuild: args.IsRebuild,
		},
	}})
}

// Assignee is how a make given to a node is resolved: what it made is scanned, as any folder of a
// library's, so the core finds each file there (D435 §4).
func (videos *SystemVideos) Assignee(runner nodes.Resolver) nodes.Assignee {
	return nodes.Assignee{
		Runner: runner,
		Answer: func(ctx context.Context, _ jobs.Job, attempt nodes.Attempt, result nodeproto.Result) error {
			made := result.SystemVideos
			if made == nil {
				return fmt.Errorf("node %d answered a system video make with something else", attempt.NodeID)
			}
			if made.Failure != "" {
				return jobs.MarkPermanent(fmt.Errorf("node %d could not make the system videos: %s",
					attempt.NodeID, made.Failure))
			}
			videos.logger().InfoContext(ctx, "made the system videos", "nodeId", attempt.NodeID,
				"folder", made.Folder, "fileNames", made.FileNames)
			return videos.parts.Scans.Request(ctx, attempt.NodeID, made.Folder, jobs.PriorityPersonsRequest)
		},
		Unanswered: func(_ context.Context, job jobs.Job, _ nodes.Attempt, err error) error {
			return nodes.UnansweredOutcome(job, err, nil)
		},
	}
}

// logger is where it logs.
func (videos *SystemVideos) logger() *slog.Logger {
	if videos.parts.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return videos.parts.Logger
}
