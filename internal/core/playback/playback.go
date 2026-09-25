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
	"time"

	"github.com/kstenerud/orpheum/internal/core/decide"
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

// Service starts playbacks and says where their bytes are.
type Service struct {
	database *store.Database
	settings Settings
	clock    clock.Clock
	random   io.Reader
	tokens   *tokens
}

// NewService returns the playback service, which makes media tokens from random.
func NewService(database *store.Database, config Settings, now clock.Clock, random io.Reader) *Service {
	return &Service{database: database, settings: config, clock: now, random: random, tokens: newTokens()}
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
	token, err := service.tokens.issue(service.random, playing{userID: userID, session: session, fileID: file.ID},
		service.clock.Now(), idle)
	if err != nil {
		return Started{}, err
	}
	return Started{Token: token, Decision: decision}, nil
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
	if !found {
		return Source{}, ErrNoToken
	}
	file, err := service.file(ctx, what.fileID)
	if err != nil {
		return Source{}, err
	}
	roots, err := service.database.Read().ListLibraryRootsOnNode(ctx, sqlcgen.ListLibraryRootsOnNodeParams{
		LibraryID: file.LibraryID, NodeID: file.NodeID,
	})
	if err != nil {
		return Source{}, fmt.Errorf("read the library's roots: %w", err)
	}
	probe, err := decodeProbe(file.Probe)
	if err != nil {
		return Source{}, err
	}
	for _, root := range roots {
		if relative, under := bytes.CutPrefix(file.PathKey, []byte(root+"/")); under {
			named := nodeproto.BytesFile{Root: root, Path: relative, Hash: hex.EncodeToString(file.PayloadHash)}
			return Source{NodeID: file.NodeID, Target: named.Target(), ContentType: contentType(probe.Format)}, nil
		}
	}
	return Source{}, fmt.Errorf("%w: file %d is under none of its library's roots", ErrNoFile, file.ID)
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
