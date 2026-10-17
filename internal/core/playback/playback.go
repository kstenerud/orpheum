// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/clientnet"
	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/core/renditions"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/subtitles"
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

// Nodes is what playback asks of the node channel, whether a file can be read now: nodes.Service.
type Nodes interface {
	library.NodeConnections
}

// Parts are what the playback service is built from.
type Parts struct {
	Database *store.Database
	Nodes    Nodes
	Settings Settings
	Clock    clock.Clock
	// Random makes media tokens.
	Random io.Reader
	// Renditions are a prepared playback's copies, made by its node.
	Renditions *renditions.Service
	// Subtitles are a played video's subtitles, made by its node, and those fetched on request.
	Subtitles *subtitles.Service
	// Preferences keeps a person's choices, which a playback starts with (D230).
	Preferences *preferences.Service
	// Loudness says the gains that level a file's sounds (D226).
	Loudness Loudness
	// Chunks offers a stalled playback a lower quality (D247 §2); nil offers none.
	Chunks Chunks
	// Reads has a file not yet read taken first, for a person who asked to play it (D304 §4); nil
	// asks for none.
	Reads FileReads
}

// FileReads has a file its listing stored unread read before what else waits: library.Scans.
type FileReads interface {
	ReadNow(ctx context.Context, libraryID, nodeID int64, reportedPath []byte) error
}

// Loudness says the gains that level each measured sound of a file's bytes: loudness.Service.
type Loudness interface {
	Levels(ctx context.Context, payloadHash []byte) (decide.Levels, error)
}

// Service starts playbacks, says where their bytes are, and has renditions prepared.
type Service struct {
	database    *store.Database
	nodes       Nodes
	settings    Settings
	clock       clock.Clock
	random      io.Reader
	tokens      *tokens
	renditions  *renditions.Service
	subtitles   *subtitles.Service
	preferences *preferences.Service
	loudness    Loudness
	chunks      Chunks
	reads       FileReads
	// speeds are the link speeds players measured, by player and network (D257 §1).
	speeds *linkSpeeds
	// unplayablePictures are the films whose own picture each player gave up as unplayable.
	unplayablePictures *unplayablePictures
	// sendAway sends away the newest Watch Together member with a player (SendAwayBy); nil sends
	// no one.
	sendAway SendAway
	// groupPlayers counts the players of a host's Watch Together session (GroupPlayersBy); nil
	// counts none.
	groupPlayers GroupPlayers
	// crowdedShareKbps is the films' share while other traffic crowds the house's upload, zero while
	// it is the setting's whole share (KeepUnderShare).
	crowdedShareKbps atomic.Int64
}

// NewService returns the playback service.
func NewService(parts Parts) *Service {
	return &Service{
		database: parts.Database, nodes: parts.Nodes, settings: parts.Settings,
		clock: parts.Clock, random: parts.Random,
		tokens: newTokens(), renditions: parts.Renditions,
		subtitles: parts.Subtitles, preferences: parts.Preferences, loudness: parts.Loudness,
		chunks: parts.Chunks, reads: parts.Reads, speeds: newLinkSpeeds(), unplayablePictures: newUnplayablePictures(),
	}
}

// Request is what a client asks to play, and what it can play (D21, D217 §1).
type Request struct {
	FileID int64
	// DeviceID is the device asking, whose sound decides whether we fold it down (D222); 0 for
	// a session with no device.
	DeviceID          int64
	PlayableFormats   decide.PlayableFormats
	PreferredDelivery decide.Delivery
	// AudioOutput is how a device with no device row makes sound, a Watch Together viewer's, asked
	// as they join (D246 §6); a person's device's own answer is read instead.
	AudioOutput string
	// ClientAddress is the address the request came from, whose network the link speed remembered
	// for the player is kept by (D257 §1).
	ClientAddress string
}

// Started is a playback: its media token and the decision.
type Started struct {
	Token    string
	Decision decide.Decision
	// IsPreparing is whether a prepared playback's rendition is still being made.
	IsPreparing bool
	// AudioRenditions are a prepared playback's audio renditions, in the master playlist's order.
	AudioRenditions []renditions.AudioRendition
	// Subtitles are the file's text subtitle tracks, each extracted or on its way (D217 §6).
	Subtitles []subtitles.Track
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
	// FileBitRateBps is the file's own average bit rate, by which a player playing it as it is
	// measures its link from how fast its buffer grows (D247 §1); 0 when the probe has none.
	FileBitRateBps int64
	// OnLadder is where on its film's ladder the playback starts, its player moving along it by
	// itself from there (D260); nil for a film not cut yet, or one that cannot be, which plays as
	// Decision says.
	OnLadder *chunks.Offer
	// NotOnLadderBecause is why a playback does not start on its film's ladder, for the log:
	// "noGridYet" (its keyframes not read within play_ladder_wait_ms), "cannotBeCut",
	// "noQualityOffered" or "noLadder"; empty when it does. A film played as its decision says, with
	// nothing to say why, left a browser test's failure to a guess (the M4 closing audit, 2026-10-08).
	NotOnLadderBecause string
}

// Start decides how a file plays for this client and issues the playback's media token, bound
// to the person and the session that asked (D217 §2). A file in a library the holder does not
// reach is library.ErrNoFile, as one that does not exist (D243 §4).
func (service *Service) Start(ctx context.Context, holder Holder, request Request) (Started, error) {
	if err := request.PlayableFormats.Validate(); err != nil {
		return Started{}, err
	}
	userID := holder.UserID
	file, err := service.reachedFile(ctx, holder, request.FileID)
	if err != nil {
		return Started{}, service.readIfStillReading(ctx, file, err)
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
	output, err := service.audioOutputOf(ctx, userID, request)
	if err != nil {
		return Started{}, err
	}
	started := Started{AudioOutput: output}
	if started.Subtitles, err = service.subtitles.Want(ctx, file.ID); err != nil {
		return Started{}, err
	}
	used, err := service.startingChoices(ctx, userID, file.ID, probe, &started)
	if err != nil {
		return Started{}, err
	}
	decision := decisionFor(request, probe, started)
	started.Decision, started.FileBitRateBps = decision, probe.Format.BitRateBps
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return Started{}, err
	}
	levels, err := service.startingPoint(ctx, userID, file, probe, &started)
	if err != nil {
		return Started{}, err
	}
	playback := tokenPlayback{
		userID: userID, sessionKey: holder.SessionKey, fileID: file.ID, libraryID: file.LibraryID,
		deviceID: request.DeviceID, decision: decision,
		isOwnPictureUnplayable: service.unplayablePictures.has(
			playerOf(request.DeviceID, holder.SessionKey), file.ID),
	}
	playback.used, playback.startingAudio = used, started.StartingAudio
	started.OnLadder, started.NotOnLadderBecause, err = service.startOnLadder(ctx, holder, request, playback,
		started.ResumePositionMs)
	if err != nil {
		return Started{}, err
	}
	playback.isOnLadder = started.OnLadder != nil
	service.keepStartsNeed(&playback, started.OnLadder)
	// A film on its ladder needs no prepared copy (D260 §1).
	if decision.Delivery == decide.HLS && !playback.isOnLadder {
		playback.profile = nodeproto.PrepareProfile(decision.Profile)
		if prepareErr := service.wantPrepared(ctx, file, probe, levels, playback.profile, &started); prepareErr != nil {
			return Started{}, prepareErr
		}
	}
	started.Token, err = service.tokens.issue(service.random, playback, service.clock.Now(), idle)
	if err != nil {
		return Started{}, err
	}
	return started, nil
}

// startingPoint is where a playback of file starts, in started, its resume position and the
// levelling of the sound played as it is, and returns the file's loudness levels.
func (service *Service) startingPoint(
	ctx context.Context, userID int64, file sqlcgen.GetPlayableFileRow, probe nodeproto.Probe, started *Started,
) (decide.Levels, error) {
	var err error
	started.ResumePositionMs, err = service.resumePositionMs(ctx, userID, file.ID, probe.Format.DurationMs)
	if err != nil {
		return nil, err
	}
	levels, err := service.loudness.Levels(ctx, file.PayloadHash)
	if err != nil {
		return nil, err
	}
	started.Levelling = decide.DirectLevelling(levels, probe)
	return levels, nil
}

// ladderLookEvery is how often a playback of a film not cut yet looks again for its keyframes,
// while it waits for them for play_ladder_wait_ms (D260 §1).
const ladderLookEvery = 100 * time.Millisecond

// startOnLadder starts a playback on its film's ladder from positionMs (D260): at the top, or, with
// its player's link on the request's network remembered, where that link calls for (D257 §1). A
// film with no grid yet is waited for, up to play_ladder_wait_ms, while its timeline is read, then nil,
// playing as its decision says; one that cannot be cut, or offers no picture, is nil at once. A nil
// offer comes with why, for the log.
func (service *Service) startOnLadder(
	ctx context.Context, holder Holder, request Request, playback tokenPlayback, positionMs int64,
) (*chunks.Offer, string, error) {
	if service.chunks == nil {
		return nil, "noLadder", nil
	}
	key := speedKey{player: playerOf(request.DeviceID, holder.SessionKey), network: clientnet.Of(request.ClientAddress)}
	kbps, _ := service.speeds.fresh(key, service.clock.Now())
	playback.positionMs = positionMs
	waitMs, err := service.settings.Int(ctx, settings.PlayLadderWaitMs)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", settings.PlayLadderWaitMs, err)
	}
	waitUntil := service.clock.Now().Add(time.Duration(waitMs) * time.Millisecond)
	chunked := chunksPlaybackOf(playback)
	if chunked.StartAtMostKbps, err = service.startAtMostKbps(ctx, holder, playback.fileID); err != nil {
		return nil, "", err
	}
	for {
		offer, startErr := service.chunks.StartOnLadder(ctx, chunked, positionMs, kbps)
		isWaiting := errors.Is(startErr, chunks.ErrNoGrid) && service.clock.Now().Before(waitUntil)
		if isWaiting && service.clock.Sleep(ctx, ladderLookEvery) == nil {
			continue
		}
		switch {
		case errors.Is(startErr, chunks.ErrNoGrid):
			return nil, "noGridYet", nil
		case errors.Is(startErr, chunks.ErrCannotBeCut):
			return nil, "cannotBeCut", nil
		case errors.Is(startErr, chunks.ErrNoQualityOffered):
			return nil, "noQualityOffered", nil
		case startErr != nil:
			return nil, "", startErr
		}
		return &offer, "", nil
	}
}

// reachedFile is a file the holder may play: one in a library they do not reach is library.ErrNoFile,
// as one that does not exist (D243 §4).
func (service *Service) reachedFile(ctx context.Context, holder Holder, fileID int64) (
	sqlcgen.GetPlayableFileRow, error,
) {
	file, err := library.PlayableFile(ctx, service.database.Read(), fileID)
	isOtherFile := holder.OnlyFileID != 0 && holder.OnlyFileID != fileID
	// A file still being read is found too, so it is judged by reach as a readable one is.
	isFound := err == nil || errors.As(err, new(*library.UnreadableError))
	if isFound && (!holder.Reach.Reaches(&file.LibraryID) || isOtherFile) {
		return file, fmt.Errorf("file %d: %w", fileID, library.ErrNoFile)
	}
	return file, err
}

// readIfStillReading asks for the read of a file a person asked to play that its listing stored
// unread, before what else waits (D304 §4); the play is refused as still reading all the same.
func (service *Service) readIfStillReading(
	ctx context.Context, file sqlcgen.GetPlayableFileRow, refusal error,
) error {
	unreadable, isUnreadable := errors.AsType[*library.UnreadableError](refusal)
	if !isUnreadable || unreadable.Reason != library.StillReading || service.reads == nil {
		return refusal
	}
	if err := service.reads.ReadNow(ctx, file.LibraryID, file.NodeID, file.ReportedPathRaw); err != nil {
		return errors.Join(refusal, fmt.Errorf("ask for file %d's read: %w", file.ID, err))
	}
	return refusal
}

// decisionFor is how a file plays for the client that asked, with the audio track the playback
// starts with.
func decisionFor(request Request, probe nodeproto.Probe, started Started) decide.Decision {
	input := decide.Input{
		Probe: probe, PlayableFormats: request.PlayableFormats,
		PreferredDelivery: request.PreferredDelivery,
		IsStereoOutput:    isStereoOutput(started.AudioOutput),
	}
	if started.StartingAudio != nil {
		input.AudioStreamIndex = new(int(started.StartingAudio.StreamIndex))
	}
	return decide.Decide(input)
}

// wantPrepared has a prepared playback's rendition made unless it is, and says in started how far
// it is, its audio renditions and the gain that levels the audio track the playback starts with.
func (service *Service) wantPrepared(
	ctx context.Context, file sqlcgen.GetPlayableFileRow, probe nodeproto.Probe, levels decide.Levels,
	profile nodeproto.PrepareProfile, started *Started,
) error {
	wanted, err := service.renditions.Want(ctx, file, probe, levels, profile)
	if err != nil {
		return err
	}
	started.IsPreparing, started.AudioRenditions = wanted.IsPreparing, wanted.Audio
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

// audioOutputOf is how the device asking makes sound: a person's device's own answer, or, for a
// viewer with no device row, what they chose as they joined (D246 §6).
func (service *Service) audioOutputOf(ctx context.Context, userID int64, request Request) (string, error) {
	if request.DeviceID == 0 {
		return request.AudioOutput, nil
	}
	return service.audioOutput(ctx, userID, request.DeviceID)
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
func (service *Service) DirectSource(ctx context.Context, token string, holder Holder) (Source, error) {
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return Source{}, err
	}
	playback, found := service.tokens.use(token, holder, service.clock.Now(), idle)
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

// Subtitle is the text, in format, of a subtitle track of the file a media token refers to
// (cut-a §5). A track of any other file is subtitles.ErrNoSubtitle.
func (service *Service) Subtitle(
	ctx context.Context, token string, track preferences.TrackRef, format subtitles.Format, holder Holder,
) (subtitles.Text, error) {
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return subtitles.Text{}, err
	}
	playback, found := service.tokens.use(token, holder, service.clock.Now(), idle)
	if !found {
		return subtitles.Text{}, ErrNoToken
	}
	return service.subtitles.TextOf(ctx, track, playback.fileID, format)
}

// FetchSubtitle fetches the subtitle a person chose for a file and, once its node converted it,
// makes it their choice in the file: nothing unparsed becomes a choice (subtitles.Service.Fetch,
// D224 §5).
func (service *Service) FetchSubtitle(
	ctx context.Context, userID, fileID int64, request subtitles.FetchRequest,
) (subtitles.Fetched, error) {
	fetched, err := service.subtitles.Fetch(ctx, userID, fileID, request)
	if err != nil {
		return subtitles.Fetched{}, err
	}
	if err := service.ChooseSubtitle(ctx, userID, fileID, preferences.Used{TrackRef: fetched.Track}); err != nil {
		return subtitles.Fetched{}, err
	}
	return fetched, nil
}

// ReadWhenViewed asks for the read of each of a title's files its listing stored unread, before
// what else waits, for a person who opened its page (D304 §4): by the time they press Play it is
// read, or nearly. A play of a file still being read asks the same.
func (service *Service) ReadWhenViewed(ctx context.Context, titleID int64) error {
	if service.reads == nil {
		return nil
	}
	files, err := service.database.Read().ListUnreadFilesOfTitle(ctx, titleID)
	if err != nil {
		return fmt.Errorf("list title %d's files still being read: %w", titleID, err)
	}
	for _, file := range files {
		if err := service.reads.ReadNow(ctx, file.LibraryID, file.NodeID, file.ReportedPathRaw); err != nil {
			return fmt.Errorf("ask for file %d's read: %w", file.ID, err)
		}
	}
	return nil
}
