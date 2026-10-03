// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
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
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The errors a playback's caller distinguishes.
var (
	// ErrNoToken is a media token that refers to no playback of this person's session.
	ErrNoToken = errors.New("no such playback")
)

// Settings are the settings playback reads.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
}

// Nodes is how playback reaches the nodes: nodes.Service. Send parks a job on a node that is not
// connected, and Unanswered a job whose node went away while it waited (D231 §11); a cancel is
// queued as it is.
type Nodes interface {
	Send(ctx context.Context, nodeID int64, command nodeproto.Command) error
	Enqueue(nodeID int64, command nodeproto.Command)
	IsConnected(ctx context.Context, nodeID int64) bool
	// NodeConnection is whether a node is connected, and when it was last heard.
	NodeConnection(ctx context.Context, nodeID int64) (nodes.Connection, time.Time)
	Unanswered(ctx context.Context, nodeID int64, err error) error
	// SlowestReadBytesPerSecond is the slowest a node reads a whole file, as its hello said;
	// zero when not known.
	SlowestReadBytesPerSecond(nodeID int64) int64
}

// Queue is the job queue: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, job jobs.Request) (bool, error)
	SetProgress(ctx context.Context, id int64, fraction float64) error
	// KeepWanted says a person still waits for a job's work: a player asking for a rendition being
	// prepared (D234 §3).
	KeepWanted(ctx context.Context, kind jobs.Kind, target jobs.Target) error
}

// Parts are what the playback service is built from.
type Parts struct {
	Database *store.Database
	Cache    *store.Cache
	Queue    Queue
	Nodes    Nodes
	// Exchanges are how a job asks a node for work and waits for its results; the node channel
	// hands them the results (nodes.Service.SetResultSink).
	Exchanges *nodes.Exchanges
	Settings  Settings
	Clock     clock.Clock
	// Random makes media tokens and the run ids of renditions.
	Random io.Reader
	Logger *slog.Logger
	// MaxSilence is how long a running prepare or extraction may go without a word from its node
	// before it is given up and retried.
	MaxSilence time.Duration
	// Subtitler fetches subtitles on request (D224); nil when the server has none.
	Subtitler Subtitler
	// Preferences keeps a person's choices, which a playback starts with (D230).
	Preferences *preferences.Service
	// Loudness says the gains that level a file's sounds (D226).
	Loudness Loudness
}

// Loudness says the gains that level each measured sound of a file's bytes: loudness.Service.
type Loudness interface {
	Levels(ctx context.Context, payloadHash []byte) (decide.Levels, error)
}

// Service starts playbacks, says where their bytes are, and has renditions prepared.
type Service struct {
	database   *store.Database
	cache      *store.Cache
	queue      Queue
	nodes      Nodes
	settings   Settings
	clock      clock.Clock
	random     io.Reader
	maxSilence time.Duration
	tokens     *tokens
	prepares   *prepares
	// exchanges are the jobs waiting on a node: a prepare, an extraction, a measurement.
	exchanges   *nodes.Exchanges
	subtitler   Subtitler
	preferences *preferences.Service
	// keptTexts tells a fetch waiting for its conversion that the text was kept.
	keptTexts *keptTexts
	loudness  Loudness
}

// NewService returns the playback service.
func NewService(parts Parts) *Service {
	exchanges := parts.Exchanges
	service := &Service{
		database: parts.Database, cache: parts.Cache, queue: parts.Queue, nodes: parts.Nodes, settings: parts.Settings,
		clock: parts.Clock, random: parts.Random, maxSilence: parts.MaxSilence,
		tokens: newTokens(), prepares: newPrepares(parts, exchanges), exchanges: exchanges,
		subtitler: parts.Subtitler, preferences: parts.Preferences, keptTexts: newKeptTexts(),
		loudness: parts.Loudness,
	}
	service.recordResults(exchanges)
	return service
}

// Request is what a client asks to play, and what it can play (D21, D217 §1).
type Request struct {
	FileID int64
	// DeviceID is the device asking, whose sound decides whether we fold it down (D222); 0 for
	// a session with no device.
	DeviceID          int64
	PlayableFormats   decide.PlayableFormats
	Network           decide.Network
	PreferredDelivery decide.Delivery
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
	// AudioTracks are the file's own audio tracks, by their place among them.
	AudioTracks []AudioTrack
	// StartingAudio is the audio track the playback starts with: the person's choice, or the one a
	// player starts with; nil when the file has no sound.
	StartingAudio *AudioTrack
	// Preferences are the person's choices for the file, which the player starts with.
	Preferences preferences.Preferences
	// ResumePositionMs is where the player starts: where the person left the file, or 0.
	ResumePositionMs int64
	// AudioOutput is how the device asking makes sound, or empty until it is asked (D222).
	AudioOutput string
	// Levelling is what brings the main sound to the level: the one played as it is, or, when
	// prepared, its stereo mix (D226). Each audio rendition carries its own.
	Levelling decide.Levelling
}

// Start decides how a file plays for this client and issues the playback's media token, bound
// to the person and the session that asked (D217 §2).
func (service *Service) Start(ctx context.Context, userID int64, sessionKey string, request Request) (Started, error) {
	if err := request.PlayableFormats.Validate(); err != nil {
		return Started{}, err
	}
	file, err := library.PlayableFile(ctx, service.database.Read(), request.FileID)
	if err != nil {
		return Started{}, err
	}
	// Nothing is queued for a file that cannot be read now: the person is told why at once (D241 §4).
	refused := library.RefuseUnreadable(ctx, service.database.Read(), service.nodes, file.NodeID, file.ReportedPathRaw)
	if refused != nil {
		return Started{}, refused
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		return Started{}, err
	}
	output, err := service.audioOutput(ctx, userID, request.DeviceID)
	if err != nil {
		return Started{}, err
	}
	started := Started{AudioOutput: output}
	if started.Subtitles, err = service.wantSubtitles(ctx, file.ID); err != nil {
		return Started{}, err
	}
	used, err := service.startingChoices(ctx, userID, file.ID, probe, &started)
	if err != nil {
		return Started{}, err
	}
	decision, err := service.decide(ctx, request, probe, started)
	if err != nil {
		return Started{}, err
	}
	started.Decision = decision
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return Started{}, err
	}
	started.ResumePositionMs, err = service.resumePositionMs(ctx, userID, file.ID, probe.Format.DurationMs)
	if err != nil {
		return Started{}, err
	}
	levels, err := service.loudness.Levels(ctx, file.PayloadHash)
	if err != nil {
		return Started{}, err
	}
	started.Levelling = decide.DirectLevelling(levels, probe)
	playback := tokenPlayback{userID: userID, sessionKey: sessionKey, fileID: file.ID}
	if decision.Delivery == decide.HLS {
		playback.profile = nodeproto.PrepareProfile(decision.Profile)
		if prepareErr := service.wantPrepared(ctx, file, probe, levels, playback.profile, &started); prepareErr != nil {
			return Started{}, prepareErr
		}
	}
	playback.used = used
	started.Token, err = service.tokens.issue(service.random, playback, service.clock.Now(), idle)
	if err != nil {
		return Started{}, err
	}
	return started, nil
}

// decide is how a file plays for the client that asked, with the audio track the playback starts
// with.
func (service *Service) decide(
	ctx context.Context, request Request, probe nodeproto.Probe, started Started,
) (decide.Decision, error) {
	remoteCeilingKbps, err := service.settings.Int(ctx, settings.RemoteBitRateCeilingKbps)
	if err != nil {
		return decide.Decision{}, fmt.Errorf("read the remote bit-rate ceiling: %w", err)
	}
	input := decide.Input{
		Probe: probe, PlayableFormats: request.PlayableFormats, Network: request.Network,
		PreferredDelivery: request.PreferredDelivery,
		RemoteCeilingKbps: remoteCeilingKbps,
		IsStereoOutput:    isStereoOutput(started.AudioOutput),
	}
	if started.StartingAudio != nil {
		input.AudioStreamIndex = new(int(started.StartingAudio.StreamIndex))
	}
	return decide.Decide(input), nil
}

// wantPrepared has a prepared playback's rendition made unless it is, and says in started how far
// it is, its audio renditions and the gain that levels the audio track the playback starts with.
func (service *Service) wantPrepared(
	ctx context.Context, file sqlcgen.GetPlayableFileRow, probe nodeproto.Probe, levels decide.Levels,
	profile nodeproto.PrepareProfile, started *Started,
) error {
	rendition, err := service.prepares.want(ctx, file, profile)
	if err != nil {
		return err
	}
	started.IsPreparing = rendition.State != renditionComplete
	started.AudioRenditions = audioRenditions(prepareCommand(nodeproto.BytesFile{}, file.ID, profile, "", probe))
	levelAudio(started.AudioRenditions, levels, probe, rendition.CarriedGainsDBJSON)
	if played := started.StartingAudio; played != nil {
		started.Levelling = levels[decide.SoundKey{StreamIndex: played.StreamIndex, Mix: decide.MixStereo}]
	}
	return nil
}

// startingChoices works out what a playback of a file starts with, into started: the person's
// preferences against the file's tracks, its audio tracks, and the audio track that plays. It
// returns what the playback uses, which the file keeps once the person starts it (D230 §2).
func (service *Service) startingChoices(
	ctx context.Context, userID, fileID int64, probe nodeproto.Probe, started *Started,
) (usedTracks, error) {
	queries := service.database.Read()
	audio, err := audioTracksOf(ctx, queries, fileID)
	if err != nil {
		return usedTracks{}, err
	}
	started.AudioTracks = audio
	tracks := preferences.Tracks{Subtitles: choiceTracks(started.Subtitles), Audio: audioChoiceTracks(audio)}
	if started.Preferences, err = service.preferences.ForFile(ctx, queries, userID, fileID, tracks); err != nil {
		return usedTracks{}, err
	}
	var mainAudio *preferences.TrackRef
	if main, found := decide.MainAudio(probe); found {
		for index := range audio {
			if audio[index].StreamIndex == int64(main.StreamIndex) {
				mainAudio = &preferences.TrackRef{StreamID: audio[index].StreamID}
			}
		}
	}
	var used usedTracks
	used.subtitle, used.audio = preferences.UsedIn(started.Preferences, tracks, mainAudio)
	if used.audio != nil {
		for index := range audio {
			if audio[index].StreamID == used.audio.StreamID {
				started.StartingAudio = &audio[index]
			}
		}
	}
	return used, nil
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
	file, err := library.PlayableFile(ctx, service.database.Read(), playback.fileID)
	if err != nil {
		return Source{}, err
	}
	bytesFile, err := library.BytesFileOf(ctx, service.database.Read(), library.LocatedOf(file))
	if err != nil {
		return Source{}, err
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		return Source{}, err
	}
	return Source{NodeID: file.NodeID, Target: bytesFile.Target(), ContentType: contentType(probe.Format)}, nil
}

// Unreadable is why a file cannot be read now, or nil when it can (D241 §7); library.ErrNoFile for
// a file that is not a live one to play.
func (service *Service) Unreadable(ctx context.Context, fileID int64) (*library.Unreadable, error) {
	file, err := library.PlayableFile(ctx, service.database.Read(), fileID)
	if err != nil {
		return nil, err
	}
	unreadable, isUnreadable, err := library.UnreadableFile(ctx, service.database.Read(), service.nodes, file.NodeID,
		file.ReportedPathRaw)
	if err != nil || !isUnreadable {
		return nil, err
	}
	return &unreadable, nil
}

func (service *Service) tokenIdleExpiry(ctx context.Context) (time.Duration, error) {
	idleExpiry, err := service.settings.Duration(ctx, settings.MediaTokenIdleExpiryHours)
	if err != nil {
		return 0, fmt.Errorf("read the media token's idle time: %w", err)
	}
	return idleExpiry, nil
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
