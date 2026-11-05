// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
)

const tagLibraries = "Libraries"

// RootBody is one of a library's roots, and whether its files are available (D208).
type RootBody struct {
	NodeName string `json:"nodeName"`
	RootPath string `json:"rootPath"`
	// LastScanOutcome is the last scan's: complete, unreadable or withheld; empty before the first.
	LastScanOutcome string `enum:"complete,unreadable,withheld," json:"lastScanOutcome"`
	LastScanAt      int64  `json:"lastScanAt,omitempty"`
	WithheldReason  string `doc:"Why the last scan's removals wait for an admin" json:"withheldReason,omitempty"`
	// MissingMountPaths are the mount points inside the root whose disk a scan found missing
	// (D208): each is the path POST /libraries/{id}/mounts/gone takes.
	MissingMountPaths []string `doc:"Mount points inside the root whose disk a scan found missing; each is what POST /libraries/{id}/mounts/gone takes (D208)" json:"missingMountPaths"` //nolint:lll // one struct tag
	// Scan is the root's scan in progress, absent when none is (D236 §2).
	Scan *RootScanBody `json:"scan,omitempty"`
	// Watch is whether the root's node watches it for changes, absent until the node has said
	// (D272 §9).
	Watch *RootWatchBody `json:"watch,omitempty"`
}

// RootWatchBody is whether a node watches a root's folders for changes, and when it does not, why.
type RootWatchBody struct {
	IsWatching bool `json:"isWatching"`
	// Cause is why the root is not watched, empty while it is.
	Cause  string `enum:"network-filesystem,watch-limit,unavailable,unreadable," json:"cause"`
	Reason string `doc:"The cause in the node's own words" json:"reason,omitempty"`
}

// RootScanBody is how far a root's scan in progress is (D236 §2): listing the root, then reading
// the files it found new or changed; and whether one more runs after it, asked for meanwhile
// (D236 §3).
type RootScanBody struct {
	IsListed            bool  `doc:"Whether the root's listing is in; before it, the scan is listing" json:"isListed"`
	ListedCount         int64 `doc:"The files the listing found" json:"listedCount"`
	ToExamineCount      int64 `doc:"Of them, those new or changed, to read" json:"toExamineCount"`
	ExaminedCount       int64 `doc:"Of those, read" json:"examinedCount"`
	IsAnotherScanQueued bool  `doc:"Whether one more scan runs after this one" json:"isAnotherScanQueued"`
}

// LibraryBody is a library: its three answers (D184), and for an admin its roots and its files
// counted by filing.
type LibraryBody struct {
	ID                 int64            `json:"id"`
	Name               string           `json:"name"`
	Kind               string           `enum:"movie,series,other" json:"kind"`
	MetadataSource     string           `enum:"nfo_then_tmdb,tmdb,nfo" json:"metadataSource"`
	ChangeDetection    string           `enum:"watch,interval,manual" json:"changeDetection"`
	CheckEveryHours    *int64           `json:"checkEveryHours,omitempty"`
	IsWritable         bool             `json:"isWritable"`
	CreatedAt          int64            `json:"createdAt"`
	Roots              []RootBody       `json:"roots,omitempty"`
	FileCountsByFiling map[string]int64 `doc:"Live files by filing (D163)" json:"fileCountsByFiling,omitempty"`
	// Rescan is, for an admin, how far its rescans in progress are; absent when none is (D357 §5).
	Rescan *LibraryRescanBody `json:"rescan,omitempty"`
}

// LibrariesOutput lists libraries.
type LibrariesOutput struct {
	Body struct {
		Libraries []LibraryBody `json:"libraries"`
	}
}

// CreateLibraryInput is a new library: the three questions answered (D184), and the roots of one
// node, which the node configured (D107).
type CreateLibraryInput struct {
	AuthorizationHeader
	Body struct {
		Name string `json:"name"            minLength:"1"`
		// Kind: films, series, both, or other videos, neither films nor series, never identified
		// online (D349).
		Kind string `enum:"movie,series,other" json:"kind"`
		// MetadataSource: NFO files where present, TMDB for the rest; TMDB only; NFO files only. Asked
		// of every kind but other, which takes NFO files only and is not asked (D184, D349).
		MetadataSource  string   `enum:"nfo_then_tmdb,tmdb,nfo" json:"metadataSource,omitempty" required:"false"`
		ChangeDetection string   `enum:"watch,interval,manual" json:"changeDetection"`
		CheckEveryHours *int64   `doc:"Hours between checks; interval only" json:"checkEveryHours,omitempty" minimum:"1"` //nolint:lll // one struct tag
		IsWritable      bool     `doc:"May the server write beside the media" json:"isWritable"`
		NodeName        string   `doc:"The node's configured name" json:"nodeName" minLength:"1"`
		RootPaths       []string `doc:"The node's configured roots, or folders inside them (D359)" json:"rootPaths" minItems:"1"` //nolint:lll // one struct tag
	}
}

// CreatedOutput names what was created.
type CreatedOutput struct {
	Body struct {
		ID int64 `json:"id"`
	}
}

// ChangeDetectionInput chooses again how a library notices changes (D184).
type ChangeDetectionInput struct {
	AuthorizationHeader
	ID   int64 `path:"id"`
	Body struct {
		ChangeDetection string `enum:"watch,interval,manual" json:"changeDetection"`
		CheckEveryHours *int64 `doc:"Hours between checks; interval only" json:"checkEveryHours,omitempty" minimum:"1"`
	}
}

// LibraryInput names one library.
type LibraryInput struct {
	AuthorizationHeader
	ID int64 `path:"id"`
}

// ScanOutput says how many roots were asked to scan.
type ScanOutput struct {
	Body struct {
		// AskedRootCount is how many roots' nodes were asked; a root whose node is away scans on
		// its return.
		AskedRootCount int `json:"askedRootCount"`
	}
}

// RootActionInput names a root, or a mount point inside one, of a library, by node and path.
type RootActionInput struct {
	AuthorizationHeader
	ID   int64 `path:"id"`
	Body struct {
		NodeName string `doc:"The node's configured name" json:"nodeName" minLength:"1"`
		Path     string `doc:"For removals, the root's path; for a disk gone, the mount point's, one of its root's missingMountPaths" json:"path" minLength:"1"` //nolint:lll // one struct tag
	}
}

// RescanOutput says whether the root was asked to scan now.
type RescanOutput struct {
	Body struct {
		// IsScanning is false when the root's node is away; the approval waits for its next scan.
		IsScanning bool `json:"isScanning"`
	}
}

// RetireNodeInput names a node an admin retires, by its name in the path (D416 §7).
type RetireNodeInput struct {
	AuthorizationHeader
	Name string `doc:"The node's configured name" minLength:"1" path:"name"`
}

// RetiredOutput says what a retire took out of the library (D231 §7).
type RetiredOutput struct {
	Body struct {
		// TombstonedFileCount is how many live files became tombstones, their history kept.
		TombstonedFileCount int `json:"tombstonedFileCount"`
		// UncoveredFileCount is how many live files no library covers any more: kept with what was
		// learned and watched, for a library over their folder to find again (D363 §10).
		UncoveredFileCount int `json:"uncoveredFileCount"`
		// CancelledJobCount is how many queued, waiting or running jobs of theirs were cancelled.
		CancelledJobCount int `json:"cancelledJobCount"`
	}
}

func retiredOutput(retired library.Retired) *RetiredOutput {
	output := &RetiredOutput{}
	output.Body.TombstonedFileCount = retired.TombstonedFileCount
	output.Body.UncoveredFileCount = retired.UncoveredFileCount
	output.Body.CancelledJobCount = retired.CancelledJobCount
	return output
}

// FilesInput pages through what the scanner saw.
type FilesInput struct {
	AuthorizationHeader
	Filing  string `enum:"listed,extra,attached,skipped,errored,unidentified," query:"filing"`
	Library int64  `doc:"One library's files alone; absent for every library's" minimum:"0" query:"library"`
	Cursor  string `doc:"From the previous page's nextCursor" query:"cursor"`
	Limit   int64  `maximum:"500" minimum:"1" query:"limit" required:"true"`
}

// ErrorDetailBody is why the node could not read a file, as the platform said it (D163).
type ErrorDetailBody struct {
	Text            string `doc:"The platform's words"                        json:"text"`
	OwnerUID        int    `json:"ownerUid"`
	GroupGID        int    `json:"groupGid"`
	ModeText        string `doc:"The file's mode, as ls shows it: -rw-------" json:"modeText"`
	OccurrenceCount int    `doc:"How often the node met it"                   json:"occurrenceCount"`
}

// FileBody is one file as the scanner saw it (D163).
type FileBody struct {
	ID           int64            `json:"id"`
	LibraryID    int64            `json:"libraryId"`
	Path         string           `json:"path"`
	Filing       string           `json:"filing"`
	FilingReason *string          `json:"filingReason,omitempty"`
	ErrorDetail  *ErrorDetailBody `json:"errorDetail,omitempty"`
	SizeBytes    *int64           `json:"sizeBytes,omitempty"`
	FirstSeenAt  int64            `json:"firstSeenAt"`
	LastSeenAt   int64            `json:"lastSeenAt"`
}

// FilesOutput is one page of files.
type FilesOutput struct {
	Body struct {
		Files      []FileBody `json:"files"`
		NextCursor string     `doc:"The next page's cursor; absent on the last" json:"nextCursor,omitempty"`
	}
}

func (server *Server) registerLibraries() {
	tags := []string{tagLibraries}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "createLibrary", Method: http.MethodPost, Path: "/libraries", DefaultStatus: http.StatusCreated,
		Summary: "Create a library over a node's roots, and scan it (admin)", Tags: tags,
	}, server.createLibrary)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listNodes", Method: http.MethodGet, Path: "/nodes",
		Summary: "The nodes, and the roots each connected one configured, for a new library (admin)", Tags: tags,
	}, server.listNodes)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listLibraries", Method: http.MethodGet, Path: "/libraries",
		Summary: "The libraries; an admin also sees their roots and files by filing", Tags: tags,
	}, server.listLibraries)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setLibraryChangeDetection", Method: http.MethodPut, Path: "/libraries/{id}/change-detection",
		Summary: "Choose again how a library notices changes: watching, checking every so many hours, or when asked",
		Tags:    tags, DefaultStatus: http.StatusNoContent,
	}, server.setLibraryChangeDetection)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "scanLibrary", Method: http.MethodPost, Path: "/libraries/{id}/scan",
		DefaultStatus: http.StatusAccepted, Summary: "Ask a library's nodes to scan it (admin)", Tags: tags,
	}, server.scanLibrary)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "approveLibraryRemovals", Method: http.MethodPost, Path: "/libraries/{id}/roots/removals",
		DefaultStatus: http.StatusAccepted, Tags: tags,
		Summary: "Approve a root's withheld removals; its next scan removes what it does not see (admin; D208)",
	}, server.approveLibraryRemovals)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "markLibraryMountGone", Method: http.MethodPost, Path: "/libraries/{id}/roots/mount-gone",
		DefaultStatus: http.StatusAccepted, Tags: tags,
		Summary: "Say a disk missing from a mount point in a root is gone; its files are removed (admin; D208)",
	}, server.markLibraryMountGone)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "retireLibraryRoot", Method: http.MethodPost, Path: "/libraries/{id}/roots/retire", Tags: tags,
		Summary: "Retire a root for good: its files become tombstones and their work is cancelled (admin; D231 §7)",
	}, server.retireLibraryRoot)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "repointLibraryRoot", Method: http.MethodPost, Path: "/libraries/{id}/roots/repoint",
		Tags:    tags,
		Summary: "Say a root moved: it and its files take the new path with their history, and a scan confirms each (admin; D241 §6)", //nolint:lll // one summary
	}, server.repointLibraryRoot)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "retireFile", Method: http.MethodPost, Path: "/files/{id}/retire", Tags: tags,
		Summary: "Retire one file that cannot be read now: it becomes a tombstone, its history kept (admin; D241 §5)",
	}, server.retireFile)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "removeLibrary", Method: http.MethodDelete, Path: "/libraries/{id}", Tags: tags,
		Summary: "Delete a library: its titles are hidden with their history, its work cancelled (admin; D231 §7)",
	}, server.removeLibrary)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "retireNode", Method: http.MethodPost, Path: "/nodes/{name}/retire", Tags: tags,
		Summary: "Retire a node for good: its roots are retired and its work is cancelled (admin; D231 §7)",
	}, server.retireNode)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listFiles", Method: http.MethodGet, Path: "/files",
		Summary: "What the scanner saw: every file with its filing and reason (admin; D163)", Tags: tags,
	}, server.listFiles)
	server.registerRescans()
	server.registerLibraryFolders()
}

func (server *Server) createLibrary(ctx context.Context, input *CreateLibraryInput) (*CreatedOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	body := input.Body
	if (body.ChangeDetection == "interval") != (body.CheckEveryHours != nil) {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "body.checkEveryHours",
			"checkEveryHours is given with changeDetection interval, and only then")
	}
	metadataSource, err := metadataSourceOf(body.Kind, body.MetadataSource)
	if err != nil {
		return nil, err
	}
	id, err := dependencies.Libraries.Create(ctx, library.CreateRequest{
		NodeName: body.NodeName, NewLibrary: library.NewLibrary{
			Name: body.Name, Kind: body.Kind, MetadataSource: metadataSource,
			ChangeDetection: body.ChangeDetection, CheckEveryHours: body.CheckEveryHours, IsWritable: body.IsWritable,
			Roots: body.RootPaths,
		},
	})
	if err != nil {
		return nil, server.libraryError(ctx, "create a library", err)
	}
	output := &CreatedOutput{}
	output.Body.ID = id
	return output, nil
}

// metadataSourceOf is a new library's metadata source: the one asked of it (D184), or for an other
// library NFO files, as it is never identified online and is not asked (D349).
func metadataSourceOf(kind, asked string) (string, error) {
	switch {
	case kind == library.LibraryKindOther && asked != "" && asked != library.MetadataSourceNFO:
		return "", invalidRequestAt(http.StatusUnprocessableEntity, "body.metadataSource",
			"an other library is never identified online: it takes NFO files only")
	case kind == library.LibraryKindOther:
		return library.MetadataSourceNFO, nil
	case asked == "":
		return "", invalidRequestAt(http.StatusUnprocessableEntity, "body.metadataSource",
			"metadataSource is asked of every library but an other one (D184)")
	}
	return asked, nil
}

// NodeBody is one node, and the roots it configured when it is connected (D107). What else it
// says about itself is the System page's (D225).
type NodeBody struct {
	Name        string   `json:"name"`
	IsConnected bool     `doc:"Whether it is connected now: polling for its commands, or heard within node_silent_after_seconds" json:"isConnected"` //nolint:lll // one struct tag
	RootPaths   []string `doc:"Its configured roots; empty while it is not connected" json:"rootPaths"`
}

// NodesOutput is every enrolled node.
type NodesOutput struct {
	Body struct {
		Nodes []NodeBody `json:"nodes"`
	}
}

func (server *Server) listNodes(ctx context.Context, input *AuthorizationHeader) (*NodesOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	nodes, err := dependencies.Libraries.NodeRoots(ctx)
	if err != nil {
		return nil, server.libraryError(ctx, "list nodes", err)
	}
	output := &NodesOutput{}
	output.Body.Nodes = make([]NodeBody, 0, len(nodes))
	for _, node := range nodes {
		output.Body.Nodes = append(output.Body.Nodes, NodeBody{
			Name: node.Name, IsConnected: node.IsConnected, RootPaths: node.Roots,
		})
	}
	return output, nil
}

func (server *Server) listLibraries(ctx context.Context, input *AuthorizationHeader) (*LibrariesOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	summaries, err := dependencies.Libraries.List(ctx, principal.Reach)
	if err != nil {
		return nil, server.libraryError(ctx, "list libraries", err)
	}
	var rescans map[int64]identify.RescanProgress
	if administersHere(ctx, principal) {
		if rescans, err = dependencies.Rescans.Progress(ctx); err != nil {
			return nil, server.logInternalError(ctx, "list the rescans", err)
		}
	}
	output := &LibrariesOutput{}
	output.Body.Libraries = make([]LibraryBody, 0, len(summaries))
	for _, summary := range summaries {
		body := LibraryBody{
			ID: summary.ID, Name: summary.Name, Kind: summary.Kind, MetadataSource: summary.MetadataSource,
			ChangeDetection: summary.ChangeDetection, CheckEveryHours: summary.CheckEveryHours,
			IsWritable: summary.IsWritable == 1,
			CreatedAt:  summary.CreatedAt,
		}
		// Roots are shown where the admin administers, never from away (the Plex-chain review's T2).
		if administersHere(ctx, principal) {
			body.FileCountsByFiling = summary.FileCountByFiling
			body.Rescan = rescanBodyOf(rescans, summary.ID)
			for _, root := range summary.Roots {
				body.Roots = append(body.Roots, RootBody{
					NodeName: root.NodeName, RootPath: root.RootPath,
					LastScanOutcome: ptr.ValueOrZero(root.LastScanOutcome),
					LastScanAt:      ptr.ValueOrZero(root.LastScanAt),
					WithheldReason:  ptr.ValueOrZero(root.WithheldReason), MissingMountPaths: root.MissingMountPaths,
					Scan: scanBodyOf(root.Scan), Watch: watchBodyOf(root.Watch),
				})
			}
		}
		output.Body.Libraries = append(output.Body.Libraries, body)
	}
	return output, nil
}

// watchBodyOf is a root's watch as the API says it, or nil before its node has said.
func watchBodyOf(watch *nodeproto.RootWatch) *RootWatchBody {
	if watch == nil {
		return nil
	}
	return &RootWatchBody{IsWatching: watch.IsWatching, Cause: string(watch.Cause), Reason: watch.Reason}
}

// scanBodyOf is a root's scan in progress as the API says it, or nil for none.
func scanBodyOf(progress *library.ScanProgress) *RootScanBody {
	if progress == nil {
		return nil
	}
	return &RootScanBody{
		IsListed: progress.IsListed, ListedCount: progress.ListedCount, ToExamineCount: progress.ToExamineCount,
		ExaminedCount: progress.ExaminedCount, IsAnotherScanQueued: progress.IsAnotherScanQueued,
	}
}

func (server *Server) scanLibrary(ctx context.Context, input *LibraryInput) (*ScanOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	asked, err := dependencies.Libraries.Scan(ctx, input.ID)
	if err != nil {
		return nil, server.libraryError(ctx, "scan a library", err)
	}
	output := &ScanOutput{}
	output.Body.AskedRootCount = asked
	return output, nil
}

func (server *Server) approveLibraryRemovals(ctx context.Context, input *RootActionInput) (*RescanOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	isScanning, err := dependencies.Libraries.ApproveRemovals(ctx, input.ID, input.Body.NodeName, input.Body.Path)
	if err != nil {
		return nil, server.libraryError(ctx, "approve removals", err)
	}
	output := &RescanOutput{}
	output.Body.IsScanning = isScanning
	return output, nil
}

func (server *Server) markLibraryMountGone(ctx context.Context, input *RootActionInput) (*RescanOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	isScanning, err := dependencies.Libraries.ForgetMount(ctx, input.ID, input.Body.NodeName, input.Body.Path)
	if err != nil {
		return nil, server.libraryError(ctx, "forget a mount", err)
	}
	output := &RescanOutput{}
	output.Body.IsScanning = isScanning
	return output, nil
}

func (server *Server) retireLibraryRoot(ctx context.Context, input *RootActionInput) (*RetiredOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	retired, err := dependencies.Libraries.RetireRoot(ctx, input.ID, input.Body.NodeName, input.Body.Path)
	if err != nil {
		return nil, server.libraryError(ctx, "retire a root", err)
	}
	return retiredOutput(retired), nil
}

// RepointInput names a root of a library, by node and path, and where it is now.
type RepointInput struct {
	AuthorizationHeader
	ID   int64 `path:"id"`
	Body struct {
		NodeName    string `doc:"The root's node" json:"nodeName" minLength:"1"`
		Path        string `doc:"The root's path" json:"path" minLength:"1"`
		NewNodeName string `doc:"The node it is on now: the same, or another" json:"newNodeName" minLength:"1"`
		NewPath     string `doc:"Where it is now: one of that node's configured roots, or a folder inside one" json:"newPath" minLength:"1"` //nolint:lll // one struct tag
	}
}

// RepointedOutput says how many files a repoint moved.
type RepointedOutput struct {
	Body struct {
		// MovedFileCount is how many live files took the new path; the next scan confirms each.
		MovedFileCount int `json:"movedFileCount"`
	}
}

func (server *Server) repointLibraryRoot(ctx context.Context, input *RepointInput) (*RepointedOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	repointed, err := dependencies.Libraries.RepointRoot(ctx, library.Repoint{
		LibraryID: input.ID, NodeName: input.Body.NodeName, Path: input.Body.Path,
		NewNodeName: input.Body.NewNodeName, NewPath: input.Body.NewPath,
	})
	if errors.Is(err, library.ErrNotANodeRoot) || errors.Is(err, library.ErrRootCovered) {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "body.newPath", err.Error())
	}
	if err != nil {
		return nil, server.libraryError(ctx, "repoint a root", err)
	}
	output := &RepointedOutput{}
	output.Body.MovedFileCount = repointed.MovedFileCount
	return output, nil
}

func (server *Server) retireFile(ctx context.Context, input *ItemInput) (*RetiredOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	retired, err := dependencies.Libraries.RetireFile(ctx, input.ID)
	if err != nil {
		return nil, server.libraryError(ctx, "retire a file", err)
	}
	return retiredOutput(retired), nil
}

func (server *Server) setLibraryChangeDetection(ctx context.Context, input *ChangeDetectionInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	body := input.Body
	if (body.ChangeDetection == "interval") != (body.CheckEveryHours != nil) {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "body.checkEveryHours",
			"checkEveryHours is given with changeDetection interval, and only then")
	}
	err = dependencies.Libraries.SetChangeDetection(ctx, input.ID, body.ChangeDetection, body.CheckEveryHours)
	if err != nil {
		return nil, server.libraryError(ctx, "set a library's change detection", err)
	}
	return &struct{}{}, nil
}

func (server *Server) removeLibrary(ctx context.Context, input *LibraryInput) (*RetiredOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	retired, err := dependencies.Libraries.Delete(ctx, input.ID)
	if err != nil {
		return nil, server.libraryError(ctx, "delete a library", err)
	}
	return retiredOutput(retired), nil
}

func (server *Server) retireNode(ctx context.Context, input *RetireNodeInput) (*RetiredOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	retired, err := dependencies.Libraries.RetireNode(ctx, input.Name)
	if err != nil {
		return nil, server.libraryError(ctx, "retire a node", err)
	}
	return retiredOutput(retired), nil
}

func (server *Server) listFiles(ctx context.Context, input *FilesInput) (*FilesOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	query := library.FilesQuery{Cursor: input.Cursor, Limit: input.Limit}
	if input.Library != 0 {
		query.LibraryID = &input.Library
	}
	if input.Filing != "" {
		query.Filing = &input.Filing
	}
	page, err := dependencies.Libraries.Files(ctx, query)
	if err != nil {
		return nil, server.libraryError(ctx, "list files", err)
	}
	output := &FilesOutput{}
	output.Body.Files = make([]FileBody, 0, len(page.Files))
	output.Body.NextCursor = page.NextCursor
	for _, row := range page.Files {
		file := FileBody{
			ID: row.ID, LibraryID: row.LibraryID, Path: row.Path, Filing: row.Filing,
			FilingReason: row.FilingReason, SizeBytes: row.SizeBytes,
			FirstSeenAt: row.FirstSeenAt, LastSeenAt: row.LastSeenAt,
		}
		if row.ErrorDetailJSON != nil {
			detail, err := library.DecodeErrorDetail(*row.ErrorDetailJSON)
			if err != nil {
				return nil, server.libraryError(ctx, "list files", err)
			}
			file.ErrorDetail = &ErrorDetailBody{
				Text: detail.Text, OwnerUID: detail.OwnerUID, GroupGID: detail.GroupGID, ModeText: detail.ModeText,
				OccurrenceCount: detail.OccurrenceCount,
			}
		}
		output.Body.Files = append(output.Body.Files, file)
	}
	return output, nil
}

// libraryError maps the library package's refusals, an admin's and a browsing person's, to API
// errors. Every handler that calls library answers through it, so a refusal always leaves with
// one code and one message.
func (server *Server) libraryError(ctx context.Context, action string, err error) error {
	switch {
	case errors.Is(err, library.ErrNoItem):
		return newError(http.StatusNotFound, CodeNotFound, "no such title or series")
	case errors.Is(err, library.ErrBadCursor):
		return errBadCursor()
	case errors.Is(err, library.ErrNoLibrary):
		return newError(http.StatusNotFound, CodeNotFound, "no such library")
	case errors.Is(err, library.ErrNoNode):
		return newError(http.StatusNotFound, CodeNotFound, "no such node")
	case errors.Is(err, library.ErrNameTaken):
		return newError(http.StatusConflict, CodeConflict, "a library with that name already exists")
	case errors.Is(err, library.ErrNodeNotConnected):
		return newError(http.StatusServiceUnavailable, CodeNodeSilent,
			"the node has not connected since the server started, so its roots are not known")
	case errors.Is(err, library.ErrNotANodeRoot), errors.Is(err, library.ErrRootCovered): // a new library's
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.rootPaths", err.Error())
	case errors.Is(err, library.ErrNoSuchRoot): // a root's answer names it by path
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.path", err.Error())
	case errors.Is(err, library.ErrNothingWithheld), errors.Is(err, library.ErrMountNotMissing),
		errors.Is(err, library.ErrFileReadable):
		return newError(http.StatusConflict, CodeConflict, err.Error())
	case errors.Is(err, library.ErrNoFile):
		return newError(http.StatusNotFound, CodeNotFound, "no such file")
	case errors.Is(err, library.ErrNoSuchSort): // recorded and file_name are other videos' alone
		return invalidRequestAt(http.StatusUnprocessableEntity, "query.sort", err.Error())
	}
	return server.logInternalError(ctx, action, err)
}
