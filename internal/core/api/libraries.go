// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
)

const tagLibraries = "Libraries"

// RootBody is one of a library's roots, and whether its files are available (D208).
type RootBody struct {
	Node string `json:"node"`
	Path string `json:"path"`
	// Outcome is the last scan's: complete, unreadable or withheld; empty before the first.
	Outcome        string `enum:"complete,unreadable,withheld," json:"outcome"`
	OutcomeAt      int64  `json:"outcomeAt,omitempty"`
	WithheldReason string `doc:"Why the last scan's removals wait for an admin" json:"withheldReason,omitempty"`
}

// LibraryBody is a library: its three answers (D184), and for an admin its roots and its files
// counted by filing.
type LibraryBody struct {
	ID              int64            `json:"id"`
	Name            string           `json:"name"`
	Kind            string           `enum:"movies,shows,mixed" json:"kind"`
	MetadataSource  string           `enum:"nfo_then_tmdb,tmdb,nfo" json:"metadataSource"`
	ChangeDetection string           `enum:"watch,interval,manual" json:"changeDetection"`
	CheckEveryH     *int64           `json:"checkEveryH,omitempty"`
	IsWritable      bool             `json:"isWritable"`
	CreatedAt       int64            `json:"createdAt"`
	Roots           []RootBody       `json:"roots,omitempty"`
	Filings         map[string]int64 `doc:"Live files by filing (D163)" json:"filings,omitempty"`
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
		Kind string `enum:"movies,shows,mixed" json:"kind"`
		// MetadataSource: NFO files where present, TMDB for the rest; TMDB only; NFO files only.
		MetadataSource  string   `enum:"nfo_then_tmdb,tmdb,nfo" json:"metadataSource"`
		ChangeDetection string   `enum:"watch,interval,manual" json:"changeDetection"`
		CheckEveryH     *int64   `doc:"Hours between checks; interval only" json:"checkEveryH,omitempty" minimum:"1"`
		IsWritable      bool     `doc:"May the server write beside the media" json:"isWritable"`
		Node            string   `doc:"The node's configured name" json:"node" minLength:"1"`
		Roots           []string `doc:"Roots the node configured" json:"roots" minItems:"1"`
	}
}

// CreatedOutput names what was created.
type CreatedOutput struct {
	Body struct {
		ID int64 `json:"id"`
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
		// Roots is how many roots' nodes were asked; a root whose node is away scans on its return.
		RootsAsked int `json:"rootsAsked"`
	}
}

// RootActionInput names a root, or a mount point inside one, of a library, by node and path.
type RootActionInput struct {
	AuthorizationHeader
	ID   int64 `path:"id"`
	Body struct {
		Node string `doc:"The node's configured name" json:"node" minLength:"1"`
		Path string `json:"path" minLength:"1"`
	}
}

// RescanOutput says whether the root was asked to scan now.
type RescanOutput struct {
	Body struct {
		// Scanning is false when the root's node is away; the approval waits for its next scan.
		IsScanning bool `json:"isScanning"`
	}
}

// FilesInput pages through what the scanner saw.
type FilesInput struct {
	AuthorizationHeader
	Filing  string `enum:"listed,extra,attached,skipped,errored,unidentified," query:"filing"`
	Library int64  `query:"library"`
	Cursor  string `doc:"From the previous page's nextCursor" query:"cursor"`
	Limit   int64  `maximum:"1000" minimum:"1" query:"limit" required:"true"`
}

// FileBody is one file as the scanner saw it (D163).
type FileBody struct {
	ID           int64   `json:"id"`
	LibraryID    int64   `json:"libraryId"`
	Path         string  `json:"path"`
	Filing       string  `json:"filing"`
	FilingReason *string `json:"filingReason,omitempty"`
	ErrorDetail  *string `doc:"Owner, group, mode; the platform's words" json:"errorDetail,omitempty"`
	SizeBytes    *int64  `json:"sizeBytes,omitempty"`
	FirstSeenAt  int64   `json:"firstSeenAt"`
	LastSeenAt   int64   `json:"lastSeenAt"`
}

// FilesOutput is one page of files.
type FilesOutput struct {
	Body struct {
		Files      []FileBody `json:"files"`
		NextCursor string     `doc:"Empty on the last page" json:"nextCursor,omitempty"`
	}
}

func (server *Server) registerLibraries() {
	tags := []string{tagLibraries}
	huma.Register(server.api, huma.Operation{
		OperationID: "postLibrary", Method: http.MethodPost, Path: "/libraries", DefaultStatus: http.StatusCreated,
		Summary: "Create a library over a node's roots, and scan it (admin)", Tags: tags,
	}, server.createLibrary)
	huma.Register(server.api, huma.Operation{
		OperationID: "getNodes", Method: http.MethodGet, Path: "/nodes",
		Summary: "The nodes, and the roots each connected one configured, for a new library (admin)", Tags: tags,
	}, server.getNodes)
	huma.Register(server.api, huma.Operation{
		OperationID: "getLibraries", Method: http.MethodGet, Path: "/libraries",
		Summary: "The libraries; an admin also sees their roots and files by filing", Tags: tags,
	}, server.getLibraries)
	huma.Register(server.api, huma.Operation{
		OperationID: "postLibraryScan", Method: http.MethodPost, Path: "/libraries/{id}/scan",
		DefaultStatus: http.StatusAccepted, Summary: "Ask a library's nodes to scan it (admin)", Tags: tags,
	}, server.scanLibrary)
	huma.Register(server.api, huma.Operation{
		OperationID: "postLibraryRemovals", Method: http.MethodPost, Path: "/libraries/{id}/removals",
		DefaultStatus: http.StatusAccepted, Tags: tags,
		Summary: "Approve a root's withheld removals; its next scan removes what it does not see (admin; D208)",
	}, server.approveRemovals)
	huma.Register(server.api, huma.Operation{
		OperationID: "postLibraryMountGone", Method: http.MethodPost, Path: "/libraries/{id}/mounts/gone",
		DefaultStatus: http.StatusAccepted, Tags: tags,
		Summary: "Say a disk missing from a mount point in a root is gone; its files are removed (admin; D208)",
	}, server.forgetMount)
	huma.Register(server.api, huma.Operation{
		OperationID: "getFiles", Method: http.MethodGet, Path: "/files",
		Summary: "What the scanner saw: every file with its filing and reason (admin; D163)", Tags: tags,
	}, server.getFiles)
}

func (server *Server) createLibrary(ctx context.Context, input *CreateLibraryInput) (*CreatedOutput, error) {
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	body := input.Body
	if (body.ChangeDetection == "interval") != (body.CheckEveryH != nil) {
		return nil, newError(http.StatusUnprocessableEntity, CodeInvalidRequest,
			"checkEveryH is given with changeDetection interval, and only then")
	}
	id, err := dependencies.Libraries.Create(ctx, library.Request{Node: body.Node, NewLibrary: library.NewLibrary{
		Name: body.Name, Kind: body.Kind, MetadataSource: body.MetadataSource, ChangeDetection: body.ChangeDetection,
		CheckEveryH: body.CheckEveryH, Writable: body.IsWritable, Roots: body.Roots,
	}})
	if err != nil {
		return nil, server.libraryError(ctx, "create a library", err)
	}
	output := &CreatedOutput{}
	output.Body.ID = id
	return output, nil
}

// NodeBody is one node, and the roots it configured when it is connected (D107).
type NodeBody struct {
	Name        string   `json:"name"`
	IsConnected bool     `doc:"Whether it has said hello since the server started" json:"isConnected"`
	Roots       []string `doc:"Its configured roots; empty while it is not connected" json:"roots"`
	// Confinement is how its FFmpeg runs are confined, per mechanism, as it last said; empty while
	// it is not connected (D112).
	Confinement []ConfinementBody `json:"confinement"`
}

// ConfinementBody is one mechanism on a node: in force, or not, why, and what would put it there.
type ConfinementBody struct {
	Mechanism string `enum:"landlock,seccomp" json:"mechanism"`
	IsInForce bool   `json:"isInForce"`
	// Cause names why it is not in force, for a client to say in its viewer's words.
	Cause  string `enum:"disabled_at_boot,kernel_without,blocked_by_runtime,not_linux,architecture_unwritten,unexplained" json:"cause,omitempty"` //nolint:lll // one struct tag
	Reason string `json:"reason,omitempty"`
	Remedy string `json:"remedy,omitempty"`
}

// NodeReports is what each connected node said about itself: nodes.Service.
type NodeReports interface {
	Confinement(name string) ([]nodeproto.Confinement, bool)
}

// confinementOf is a node's confinement as the API shows it: empty when it is not connected.
func confinementOf(reports NodeReports, name string) []ConfinementBody {
	reported, _ := reports.Confinement(name)
	bodies := make([]ConfinementBody, 0, len(reported))
	for _, state := range reported {
		bodies = append(bodies, ConfinementBody{
			Mechanism: state.Mechanism, IsInForce: state.InForce, Cause: state.Cause, Reason: state.Reason,
			Remedy: state.Remedy,
		})
	}
	return bodies
}

// NodesOutput is every enrolled node.
type NodesOutput struct {
	Body struct {
		Nodes []NodeBody `json:"nodes"`
	}
}

func (server *Server) getNodes(ctx context.Context, input *AuthorizationHeader) (*NodesOutput, error) {
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	nodes, err := dependencies.Libraries.NodeRoots(ctx)
	if err != nil {
		return nil, server.internal(ctx, "list nodes", err)
	}
	output := &NodesOutput{}
	output.Body.Nodes = make([]NodeBody, 0, len(nodes))
	for _, node := range nodes {
		body := NodeBody{
			Name: node.Name, IsConnected: node.Connected, Roots: node.Roots,
			Confinement: confinementOf(dependencies.NodeReports, node.Name),
		}
		output.Body.Nodes = append(output.Body.Nodes, body)
	}
	return output, nil
}

func (server *Server) getLibraries(ctx context.Context, input *AuthorizationHeader) (*LibrariesOutput, error) {
	dependencies, principal, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	summaries, err := dependencies.Libraries.List(ctx)
	if err != nil {
		return nil, server.internal(ctx, "list libraries", err)
	}
	output := &LibrariesOutput{}
	output.Body.Libraries = make([]LibraryBody, 0, len(summaries))
	for _, summary := range summaries {
		body := LibraryBody{
			ID: summary.ID, Name: summary.Name, Kind: summary.Kind, MetadataSource: summary.MetadataSource,
			ChangeDetection: summary.ChangeDetection, CheckEveryH: summary.CheckEveryH,
			IsWritable: summary.Writable == 1,
			CreatedAt:  summary.CreatedAt,
		}
		if principal.IsAdmin() {
			body.Filings = summary.Filings
			for _, root := range summary.Roots {
				body.Roots = append(body.Roots, RootBody{
					Node: root.NodeName, Path: root.Path, Outcome: ptr.ValueOrZero(root.Outcome),
					OutcomeAt:      ptr.ValueOrZero(root.OutcomeAt),
					WithheldReason: ptr.ValueOrZero(root.WithheldReason),
				})
			}
		}
		output.Body.Libraries = append(output.Body.Libraries, body)
	}
	return output, nil
}

func (server *Server) scanLibrary(ctx context.Context, input *LibraryInput) (*ScanOutput, error) {
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	asked, err := dependencies.Libraries.Scan(ctx, input.ID)
	if err != nil {
		return nil, server.libraryError(ctx, "scan a library", err)
	}
	output := &ScanOutput{}
	output.Body.RootsAsked = asked
	return output, nil
}

func (server *Server) approveRemovals(ctx context.Context, input *RootActionInput) (*RescanOutput, error) {
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	scanning, err := dependencies.Libraries.ApproveRemovals(ctx, input.ID, input.Body.Node, input.Body.Path)
	if err != nil {
		return nil, server.libraryError(ctx, "approve removals", err)
	}
	output := &RescanOutput{}
	output.Body.IsScanning = scanning
	return output, nil
}

func (server *Server) forgetMount(ctx context.Context, input *RootActionInput) (*RescanOutput, error) {
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	scanning, err := dependencies.Libraries.ForgetMount(ctx, input.ID, input.Body.Node, input.Body.Path)
	if err != nil {
		return nil, server.libraryError(ctx, "forget a mount", err)
	}
	output := &RescanOutput{}
	output.Body.IsScanning = scanning
	return output, nil
}

func (server *Server) getFiles(ctx context.Context, input *FilesInput) (*FilesOutput, error) {
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	query := library.FilesQuery{Limit: input.Limit}
	if input.Cursor != "" {
		if query.After, err = strconv.ParseInt(input.Cursor, 10, 64); err != nil || query.After < 0 {
			return nil, newError(http.StatusUnprocessableEntity, CodeInvalidRequest,
				"the cursor is not one this server gave")
		}
	}
	if input.Library != 0 {
		query.LibraryID = &input.Library
	}
	if input.Filing != "" {
		query.Filing = &input.Filing
	}
	rows, err := dependencies.Libraries.Files(ctx, query)
	if err != nil {
		return nil, server.internal(ctx, "list files", err)
	}
	output := &FilesOutput{}
	output.Body.Files = make([]FileBody, 0, len(rows))
	for _, row := range rows {
		output.Body.Files = append(output.Body.Files, FileBody{
			ID: row.ID, LibraryID: row.LibraryID, Path: row.Path, Filing: row.Filing,
			FilingReason: row.FilingReason, ErrorDetail: row.ErrorDetail, SizeBytes: row.SizeBytes,
			FirstSeenAt: row.FirstSeenAt, LastSeenAt: row.LastSeenAt,
		})
	}
	if int64(len(rows)) == input.Limit {
		output.Body.NextCursor = strconv.FormatInt(rows[len(rows)-1].ID, 10)
	}
	return output, nil
}

// libraryError maps the library package's refusals to API errors.
func (server *Server) libraryError(ctx context.Context, doing string, err error) error {
	switch {
	case errors.Is(err, library.ErrNoLibrary):
		return newError(http.StatusNotFound, CodeNotFound, "no such library")
	case errors.Is(err, library.ErrExists):
		return newError(http.StatusConflict, CodeConflict, "a library with that name already exists")
	case errors.Is(err, library.ErrNodeNotConnected):
		return newError(http.StatusServiceUnavailable, CodeNodeSilent,
			"the node has not connected since the server started, so its roots are not known")
	case errors.Is(err, library.ErrNotANodeRoot), errors.Is(err, library.ErrNoSuchRoot):
		return newError(http.StatusUnprocessableEntity, CodeInvalidRequest, err.Error())
	case errors.Is(err, library.ErrNothingWithheld), errors.Is(err, library.ErrMountNotMissing):
		return newError(http.StatusConflict, CodeConflict, err.Error())
	}
	return server.internal(ctx, doing, err)
}
