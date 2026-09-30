// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
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
	Enqueue(ctx context.Context, job jobs.Request) (bool, error)
	SetProgress(ctx context.Context, id int64, fraction float64) error
}

// Parts are what the playback service is built from.
type Parts struct {
	Database *store.Database
	Cache    *store.Cache
	Queue    Queue
	Nodes    Nodes
	Settings Settings
	Clock    clock.Clock
	// Random makes media tokens and the run ids of renditions.
	Random io.Reader
	Logger *slog.Logger
	// MaxSilence is how long a running prepare or extraction may go without a word from its node
	// before it is given up and retried.
	MaxSilence time.Duration
	// Subtitler fetches subtitles on request (D224); nil when the server has none.
	Subtitler Subtitler
}

// Service starts playbacks, says where their bytes are, and has renditions prepared.
type Service struct {
	database            *store.Database
	cache               *store.Cache
	queue               Queue
	nodes               Nodes
	settings            Settings
	clock               clock.Clock
	random              io.Reader
	maxSilence          time.Duration
	tokens              *tokens
	prepares            *prepares
	subtitleTextAnswers *nodes.AnswerRouter[nodeproto.SubtitleText]
	loudnessAnswers     *nodes.AnswerRouter[nodeproto.LoudnessResult]
	subtitler           Subtitler
}

// NewService returns the playback service.
func NewService(parts Parts) *Service {
	return &Service{
		database: parts.Database, cache: parts.Cache, queue: parts.Queue, nodes: parts.Nodes, settings: parts.Settings,
		clock: parts.Clock, random: parts.Random, maxSilence: parts.MaxSilence,
		tokens: newTokens(), prepares: newPrepares(parts),
		subtitleTextAnswers: nodes.NewAnswerRouter[nodeproto.SubtitleText](parts.Logger, "a subtitle"),
		subtitler:           parts.Subtitler,
		loudnessAnswers:     nodes.NewAnswerRouter[nodeproto.LoudnessResult](parts.Logger, "a loudness measurement"),
	}
}

// Request is what a client asks to play, and what it can play (D21, D217 §1).
type Request struct {
	FileID int64
	// DeviceID is the device asking, whose sound decides whether we fold it down (D222); 0 for
	// a session with no device.
	DeviceID          int64
	PlayableFormats   decide.PlayableFormats
	Network           decide.Network
	PreferredDelivery decide.PreferredDelivery
}

// Started is a playback: its media token and the decision.
type Started struct {
	Token    string
	Decision decide.Decision
	// IsPreparing is whether a prepared playback's rendition is still being made.
	IsPreparing bool
	// AudioRenditions are a prepared playback's audio renditions, in the master playlist's order.
	AudioRenditions []AudioRendition
	// Subtitles are the file's text subtitle tracks, each extracted or on its way (D217 §6).
	Subtitles []SubtitleTrack
	// Preferences are the person's choices for the file, which the player starts with.
	Preferences FilePreferences
	// ResumePositionMs is where the player starts: where the person left the file, or 0.
	ResumePositionMs int64
	// AudioOutput is how the device asking makes sound, or empty until it is asked (D222).
	AudioOutput string
	// Levelling is what brings the main sound to the level: the one played as it is, or, when
	// prepared, its stereo mix (D226). Each audio rendition carries its own.
	Levelling Levelling
}

// Start decides how a file plays for this client and issues the playback's media token, bound
// to the person and the session that asked (D217 §2).
func (service *Service) Start(ctx context.Context, userID int64, sessionKey string, request Request) (Started, error) {
	if err := request.PlayableFormats.Validate(); err != nil {
		return Started{}, err
	}
	file, err := service.playableFile(ctx, service.database.Read(), request.FileID)
	if err != nil {
		return Started{}, err
	}
	probe, err := decodeProbe(file.ProbeJSON)
	if err != nil {
		return Started{}, err
	}
	remoteCeilingKbps, err := service.settings.Int(ctx, settings.RemoteBitRateCeilingKbps)
	if err != nil {
		return Started{}, fmt.Errorf("read the remote bit-rate ceiling: %w", err)
	}
	output, err := service.audioOutput(ctx, userID, request.DeviceID)
	if err != nil {
		return Started{}, err
	}
	decision := decide.Decide(decide.Input{
		Probe: probe, PlayableFormats: request.PlayableFormats, Network: request.Network,
		PreferredDelivery: request.PreferredDelivery,
		RemoteCeilingKbps: remoteCeilingKbps,
		IsStereoOutput:    isStereoOutput(output),
	})
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return Started{}, err
	}
	started := Started{Decision: decision, AudioOutput: output}
	if started.Subtitles, err = service.wantSubtitles(ctx, file.ID); err != nil {
		return Started{}, err
	}
	if started.Preferences, err = service.FilePreferences(ctx, userID, file.ID); err != nil {
		return Started{}, err
	}
	started.ResumePositionMs, err = service.resumePositionMs(ctx, userID, file.ID, probe.Format.DurationMs)
	if err != nil {
		return Started{}, err
	}
	levels, err := service.levellings(ctx, file.PayloadHash)
	if err != nil {
		return Started{}, err
	}
	started.Levelling = directLevelling(levels, probe)
	playback := tokenPlayback{userID: userID, sessionKey: sessionKey, fileID: file.ID}
	if decision.Delivery == decide.HLS {
		playback.profile = nodeproto.PrepareProfile(decision.Profile)
		rendition, wantErr := service.prepares.want(ctx, file, playback.profile)
		if wantErr != nil {
			return Started{}, wantErr
		}
		started.IsPreparing = rendition.State != renditionComplete
		started.AudioRenditions = audioRenditions(
			prepareCommand(nodeproto.BytesFile{}, file.ID, playback.profile, "", probe))
		levelAudio(started.AudioRenditions, levels, probe, rendition.CarriedGainsDBJSON)
		if main, found := decide.MainAudio(probe); found {
			started.Levelling = levels[soundKey{int64(main.StreamIndex), mixStereo}]
		}
	}
	started.Token, err = service.tokens.issue(service.random, playback, service.clock.Now(), idle)
	if err != nil {
		return Started{}, err
	}
	return started, nil
}

// isStereoOutput is whether a device's sound is two channels the server folds down for it: its
// built-in speakers or headphones, not a soundbar or receiver, nor a device not yet asked (D222).
func isStereoOutput(audioOutput string) bool {
	return audioOutput == auth.AudioOutputBuiltInSpeakers || audioOutput == auth.AudioOutputHeadphones
}

// audioOutput is how a person's device makes sound, or empty when it is not yet asked or there is
// no device.
func (service *Service) audioOutput(ctx context.Context, userID, deviceID int64) (string, error) {
	if deviceID == 0 {
		return "", nil
	}
	device, err := store.OrNotFound(service.database.Read().GetLiveDeviceOfUser(ctx, sqlcgen.GetLiveDeviceOfUserParams{
		ID: deviceID, UserID: &userID,
	}))
	if errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read device %d: %w", deviceID, err)
	}
	return ptr.ValueOrZero(device.AudioOutput), nil
}

// Source is where a direct playback's bytes are: a node, the target on its byte socket, and
// the type to serve them as.
type Source struct {
	NodeID      int64
	Target      string
	ContentType string
}

// DirectSource is where the bytes of the file a media token refers to are.
func (service *Service) DirectSource(
	ctx context.Context, token string, userID int64, sessionKey string,
) (Source, error) {
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return Source{}, err
	}
	playback, found := service.tokens.use(token, userID, sessionKey, service.clock.Now(), idle)
	if !found || playback.profile != "" {
		return Source{}, ErrNoToken
	}
	file, err := service.playableFile(ctx, service.database.Read(), playback.fileID)
	if err != nil {
		return Source{}, err
	}
	bytesFile, err := service.bytesFileOf(ctx, locatedOf(file))
	if err != nil {
		return Source{}, err
	}
	probe, err := decodeProbe(file.ProbeJSON)
	if err != nil {
		return Source{}, err
	}
	return Source{NodeID: file.NodeID, Target: bytesFile.Target(), ContentType: contentType(probe.Format)}, nil
}

// located is where a file's bytes are: its library, its node, the path its node reported, and
// its payload hash. Its subject names what was located, "file 12" or "stream 40", for an error.
type located struct {
	subject           string
	libraryID         int64
	nodeID            int64
	reportedPathBytes []byte
	payloadHash       []byte
}

func locatedOf(file sqlcgen.GetPlayableFileRow) located {
	return located{
		subject: fmt.Sprintf("file %d", file.ID), libraryID: file.LibraryID, nodeID: file.NodeID,
		reportedPathBytes: file.ReportedPathBytes, payloadHash: file.PayloadHash,
	}
}

// bytesFileOf names a file on its node's byte socket: the library root it is under, its path
// relative to that root, and its payload hash.
func (service *Service) bytesFileOf(ctx context.Context, file located) (nodeproto.BytesFile, error) {
	roots, err := service.database.Read().ListLibraryRootsOnNode(ctx, sqlcgen.ListLibraryRootsOnNodeParams{
		LibraryID: file.libraryID, NodeID: file.nodeID,
	})
	if err != nil {
		return nodeproto.BytesFile{}, fmt.Errorf("read the library's roots: %w", err)
	}
	root, relative, found := library.RootHolding(file.reportedPathBytes, roots)
	if !found {
		return nodeproto.BytesFile{}, fmt.Errorf("%w: %s is under none of its library's roots", ErrNoFile, file.subject)
	}
	return nodeproto.BytesFile{RootPath: root, Path: relative, PayloadHash: hex.EncodeToString(file.payloadHash)}, nil
}

func (service *Service) playableFile(
	ctx context.Context, queries *sqlcgen.Queries, fileID int64,
) (sqlcgen.GetPlayableFileRow, error) {
	file, err := store.OrNotFound(queries.GetPlayableFile(ctx, fileID))
	if errors.Is(err, store.ErrNotFound) {
		return file, ErrNoFile
	}
	if err != nil {
		return file, fmt.Errorf("read file %d: %w", fileID, err)
	}
	if file.ProbeJSON == nil || file.PayloadHash == nil {
		return file, fmt.Errorf("%w: file %d was never probed and hashed", ErrNoFile, fileID)
	}
	return file, nil
}

func (service *Service) tokenIdleExpiry(ctx context.Context) (time.Duration, error) {
	hours, err := service.settings.Int(ctx, settings.MediaTokenIdleExpiryH)
	if err != nil {
		return 0, fmt.Errorf("read the media token's idle time: %w", err)
	}
	return time.Duration(hours) * time.Hour, nil
}

// decodeProbe is a file's stored probe; a file never probed is ErrNoFile.
func decodeProbe(stored *string) (nodeproto.Probe, error) {
	if stored == nil {
		return nodeproto.Probe{}, ErrNoFile
	}
	return library.DecodeProbe(*stored)
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
