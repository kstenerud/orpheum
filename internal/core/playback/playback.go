// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The errors a playback's caller distinguishes.
var (
	// ErrNoFile is a file that is not there to play: never seen, gone, or not a video.
	ErrNoFile = errors.New("no such file to play")
	// ErrNoToken is a media token that refers to no playback of this person's session.
	ErrNoToken = errors.New("no such playback")
)

// Settings are the settings playback reads.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// Nodes is how playback reaches the nodes: nodes.Service.
type Nodes interface {
	Enqueue(nodeID int64, command nodeproto.Command)
}

// Queue is the job queue: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, job jobs.New) (bool, error)
}

// Parts are what the playback service is built from.
type Parts struct {
	Database *store.Database
	Cache    *store.Cache
	Queue    Queue
	Nodes    Nodes
	Settings Settings
	Clock    clock.Clock
	// Random makes media tokens.
	Random io.Reader
	Logger *slog.Logger
	// Silence is how long a running prepare may go without a word from its node before it is
	// given up and retried.
	Silence time.Duration
}

// Service starts playbacks, says where their bytes are, and has renditions prepared.
type Service struct {
	database *store.Database
	settings Settings
	clock    clock.Clock
	random   io.Reader
	tokens   *tokens
	prepares *prepares
}

// NewService returns the playback service.
func NewService(parts Parts) *Service {
	return &Service{
		database: parts.Database, settings: parts.Settings, clock: parts.Clock, random: parts.Random,
		tokens: newTokens(), prepares: newPrepares(parts),
	}
}

// Request is what a client asks to play, and what it can play (D21, D217 §1).
type Request struct {
	FileID int64
	Menu   decide.Menu
	Link   decide.Link
	Prefer decide.Prefer
}

// Started is a playback: its media token and the decision.
type Started struct {
	Token    string
	Decision decide.Decision
	// Preparing is whether a prepared playback's rendition is still being made.
	Preparing bool
	// Audio are a prepared playback's audio renditions, in the master playlist's order.
	Audio []AudioRendition
}

// Start decides how a file plays for this client and issues the playback's media token, bound
// to the person and the session that asked (D217 §2).
func (service *Service) Start(ctx context.Context, userID int64, session string, request Request) (Started, error) {
	if err := request.Menu.Validate(); err != nil {
		return Started{}, err
	}
	file, err := service.file(ctx, request.FileID)
	if err != nil {
		return Started{}, err
	}
	probe, err := decodeProbe(file.Probe)
	if err != nil {
		return Started{}, err
	}
	ceiling, err := service.settings.Int(ctx, settings.RemoteBitRateCeilingKbps)
	if err != nil {
		return Started{}, fmt.Errorf("read the remote bit-rate ceiling: %w", err)
	}
	decision := decide.Decide(decide.Input{
		Probe: probe, Menu: request.Menu, Link: request.Link, Prefer: request.Prefer, RemoteCeilingKbps: ceiling,
	})
	idle, err := service.idle(ctx)
	if err != nil {
		return Started{}, err
	}
	started := Started{Decision: decision}
	what := playing{userID: userID, session: session, fileID: file.ID}
	if decision.Kind == decide.HLS {
		what.profile = nodeproto.PrepareProfile(decision.Profile)
		rendition, wantErr := service.prepares.want(ctx, file, what.profile)
		if wantErr != nil {
			return Started{}, wantErr
		}
		started.Preparing = rendition.State != renditionComplete
		started.Audio = audioRenditions(prepareCommand(nodeproto.BytesFile{}, file.ID, what.profile, "", probe))
	}
	started.Token, err = service.tokens.issue(service.random, what, service.clock.Now(), idle)
	if err != nil {
		return Started{}, err
	}
	return started, nil
}

// Source is where a direct playback's bytes are: a node, the target on its byte socket, and
// the type to serve them as.
type Source struct {
	NodeID      int64
	Target      string
	ContentType string
}

// Direct is where the bytes of the file a media token refers to are.
func (service *Service) Direct(ctx context.Context, token string, userID int64, session string) (Source, error) {
	idle, err := service.idle(ctx)
	if err != nil {
		return Source{}, err
	}
	what, found := service.tokens.use(token, userID, session, service.clock.Now(), idle)
	if !found || what.profile != "" {
		return Source{}, ErrNoToken
	}
	file, err := service.file(ctx, what.fileID)
	if err != nil {
		return Source{}, err
	}
	named, err := service.source(ctx, file)
	if err != nil {
		return Source{}, err
	}
	probe, err := decodeProbe(file.Probe)
	if err != nil {
		return Source{}, err
	}
	return Source{NodeID: file.NodeID, Target: named.Target(), ContentType: contentType(probe.Format)}, nil
}

// source names a file on its node's byte socket: the library root it is under, its path
// relative to that root, and its payload hash.
func (service *Service) source(ctx context.Context, file sqlcgen.GetPlayableFileRow) (nodeproto.BytesFile, error) {
	roots, err := service.database.Read().ListLibraryRootsOnNode(ctx, sqlcgen.ListLibraryRootsOnNodeParams{
		LibraryID: file.LibraryID, NodeID: file.NodeID,
	})
	if err != nil {
		return nodeproto.BytesFile{}, fmt.Errorf("read the library's roots: %w", err)
	}
	for _, root := range roots {
		if relative, under := bytes.CutPrefix(file.PathKey, []byte(root+"/")); under {
			return nodeproto.BytesFile{Root: root, Path: relative, Hash: hex.EncodeToString(file.PayloadHash)}, nil
		}
	}
	return nodeproto.BytesFile{}, fmt.Errorf("%w: file %d is under none of its library's roots", ErrNoFile, file.ID)
}

func (service *Service) file(ctx context.Context, fileID int64) (sqlcgen.GetPlayableFileRow, error) {
	file, err := store.One(service.database.Read().GetPlayableFile(ctx, fileID))
	if errors.Is(err, store.ErrNotFound) {
		return file, ErrNoFile
	}
	if err != nil {
		return file, fmt.Errorf("read file %d: %w", fileID, err)
	}
	if file.Probe == nil || file.PayloadHash == nil {
		return file, fmt.Errorf("%w: file %d was never probed and hashed", ErrNoFile, fileID)
	}
	return file, nil
}

func (service *Service) idle(ctx context.Context) (time.Duration, error) {
	hours, err := service.settings.Int(ctx, settings.MediaTokenIdleH)
	if err != nil {
		return 0, fmt.Errorf("read the media token's idle time: %w", err)
	}
	return time.Duration(hours) * time.Hour, nil
}

func decodeProbe(stored *string) (nodeproto.Probe, error) {
	var probe nodeproto.Probe
	if stored == nil {
		return probe, ErrNoFile
	}
	if err := json.Unmarshal([]byte(*stored), &probe); err != nil {
		return probe, fmt.Errorf("read the stored probe: %w", err)
	}
	return probe, nil
}

// contentType is what a browser is told a file is, by its container.
func contentType(format nodeproto.ProbeFormat) string {
	container, found := decide.ContainerOf(format)
	if !found {
		return "application/octet-stream"
	}
	switch container {
	case decide.MP4:
		return "video/mp4"
	case decide.Matroska:
		return "video/x-matroska"
	case decide.MPEGTS:
		return "video/mp2t"
	}
	return "application/octet-stream"
}
