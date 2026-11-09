// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/clientnet"
	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/subtitles"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The errors a playback's caller distinguishes.
var (
	// ErrNoToken is a media token that refers to no playback of this person's session.
	ErrNoToken = errors.New("no such playback")
	// ErrCannotConvert is a file its device cannot play as it is that cannot start on its ladder
	// either: it has no picture, or neither a known length nor keyframes to cut it by (D344 §5).
	ErrCannotConvert = errors.New("this file cannot be converted for this device")
	// ErrTooSlowToConvert is a file its device cannot play as it is, nor its picture copied, whose
	// node encodes even the lowest quality slower than playback (D457): converted, it would stall
	// over and over.
	ErrTooSlowToConvert = errors.New("this file's node converts too slowly to play it on this device")
)

// notOnLadderTooSlow is why a playback is not on its ladder when its film's node encodes too slowly
// for the only pictures its player could play (D457).
const notOnLadderTooSlow = "tooSlowToEncode"

// cannotConvertBecause is the refusal of a playback its device cannot play as it is that is not on
// its ladder for why: ErrTooSlowToConvert when its node encodes too slowly, else ErrCannotConvert.
func cannotConvertBecause(notOnLadderBecause string) error {
	if notOnLadderBecause == notOnLadderTooSlow {
		return ErrTooSlowToConvert
	}
	return ErrCannotConvert
}

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
	// Subtitles are a played video's subtitles, made by its node, and those fetched on request.
	Subtitles *subtitles.Service
	// Preferences keeps a person's choices, which a playback starts with (D230).
	Preferences *preferences.Service
	// Loudness says the gains that level a file's sounds (D226).
	Loudness Loudness
	// Chunks offers a stalled playback a lower quality (D247 §2); nil offers none.
	Chunks Chunks
	// Offers says what a film's ladder offers a playback, which its player names (Tracks); nil names
	// none.
	Offers Offers
	// Reads has a file not yet read taken first, for a person who asked to play it (D304 §4); nil
	// asks for none.
	Reads FileReads
	// Roots asks a root's node whether it can be read now, and brings it back when it can (D327); nil
	// asks none.
	Roots RootProbes
	// Absences are the places a read found a file not at, which a play reads from last (D363 §8); nil
	// knows of none.
	Absences *library.Absences
}

// RootProbes asks a root's node whether it can be read now, and brings it back when it can:
// library.Scans.
type RootProbes interface {
	ProbeAndBringBack(ctx context.Context, nodeID int64, root string) (nodeproto.PlaceProbe, bool, error)
}

// FileReads has a file its listing stored unread read before what else waits: library.Scans.
type FileReads interface {
	ReadNow(ctx context.Context, nodeID int64, reportedPath []byte) error
}

// Loudness says the gains that level each measured sound of a file's bytes: loudness.Service.
type Loudness interface {
	Levels(ctx context.Context, payloadHash []byte) (decide.Levels, error)
}

// Service starts playbacks and says where their bytes are.
type Service struct {
	database    *store.Database
	nodes       Nodes
	settings    Settings
	clock       clock.Clock
	random      io.Reader
	tokens      *tokens
	subtitles   *subtitles.Service
	preferences *preferences.Service
	loudness    Loudness
	chunks      Chunks
	offers      Offers
	reads       FileReads
	roots       RootProbes
	absences    *library.Absences
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
	// timing is how long it holds what its players tell it, as last read (playbackTiming); nil before.
	timing atomic.Pointer[playbackTiming]
}

// NewService returns the playback service.
func NewService(parts Parts) *Service {
	service := &Service{
		database: parts.Database, nodes: parts.Nodes, settings: parts.Settings,
		clock: parts.Clock, random: parts.Random,
		tokens:    newTokens(),
		subtitles: parts.Subtitles, preferences: parts.Preferences, loudness: parts.Loudness,
		chunks: parts.Chunks, offers: parts.Offers, reads: parts.Reads, roots: parts.Roots, absences: parts.Absences,
		speeds:             newLinkSpeeds(parts.Database),
		unplayablePictures: newUnplayablePictures(),
	}
	// The runs that go on are those its playbacks' players want (D309).
	if parts.Chunks != nil {
		parts.Chunks.InterestsBy(service.InterestsIn)
	}
	return service
}

// Request is what a client asks to play, and what it can play (D21, D217 §1).
type Request struct {
	FileID int64
	// DeviceID is the device asking, whose sound offset the playback carries; 0 for a session with
	// no device.
	DeviceID          int64
	PlayableFormats   decide.PlayableFormats
	PreferredDelivery decide.Delivery
	// ClientAddress is the address the request came from, whose network the link speed remembered
	// for the player is kept by (D257 §1).
	ClientAddress string
	// HasGivenUpOwnPicture is the player's word that it gave the film's own picture up as unplayable
	// on its device (D266): kept before the playback is decided, so it starts at the twin (D323 §2).
	HasGivenUpOwnPicture bool
	// IsWebAudioLate is the player's word that its device's sound through Web Audio plays later than
	// D436's bound, so it raises no sound itself: a sound levelling raises is the ladder's (D436).
	IsWebAudioLate bool
}

// Started is a playback: its media token and the decision.
type Started struct {
	Token    string
	Decision decide.Decision
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
	// SoundOffsetMs is how far the device asking moves its sound from its picture, later when
	// positive and earlier when negative; 0 for none, and for a viewer with no device.
	SoundOffsetMs int64
	// Levelling is what brings the starting track's sound, played as it is, to the level (D226); each
	// of its sounds says its own beside it (Tracks), which a player levels the sound it hears by.
	Levelling decide.Levelling
	// FileBitRateBps is the file's own average bit rate, by which a player playing it as it is
	// measures its link from how fast its buffer grows (D247 §1); 0 when the probe has none.
	FileBitRateBps int64
	// OnLadder is where on its film's ladder the playback starts, its player moving along it by
	// itself from there (D260); nil for a film not cut yet, or one that cannot be, which plays as
	// Decision says.
	OnLadder *chunks.Offer
	// NotOnLadderBecause is why a playback does not start on its film's ladder, for the log:
	// "noGridYet" (its keyframes not read within play_ladder_wait_ms, of a film its device plays as it
	// is; one it cannot is cut on time, D344 §2), "cannotBeCut",
	// "noQualityOffered", "tooSlowToEncode" (its node encodes too slowly for any picture its player
	// plays, D457) or "noLadder"; empty when it does. A film played as its decision says, with
	// nothing to say why, left a browser test's failure to a guess (the M4 closing audit, 2026-10-08).
	NotOnLadderBecause string
	// Steadiness is the steadiness level its player starts on the ladder at: the one its last
	// playback on the request's network ended at, relaxed by the time since (D352 Q2); 0 for none.
	Steadiness int
	// RememberedLink is the link its player last measured on the request's network, within
	// link_speed_fresh_hours (D257 §1), which the server chose its starting quality by and its player
	// takes into its estimate as a prior (bandwidth-sampling-2026-10-29.md P2); zero for none.
	RememberedLink RememberedLink
	// Tracks are what the playback may play and what the file holds, which its player names.
	Tracks Tracks
	// levels are the film's measured loudness, which each of its sounds is levelled by (Tracks).
	levels decide.Levels
}

// RememberedLink is the link a player last measured on a network, as its next playback there starts
// by it: its speed, the spread of the natural logs of the samples it rested on (0 where its player
// said none), how old it is, and how many of the playback's own samples it counts as
// (measuredSpeed.countsAsSamples). The zero value is none.
type RememberedLink struct {
	Kbps            int64
	LogSpread       float64
	AgeMs           int64
	CountsAsSamples float64
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
	// It plays from a place the person reaches, the first that can be read now (D363 §8); nothing is
	// queued for a file that cannot be read now at any: the person is told why at once (D241 §4).
	place, err := library.PlaceToRead(ctx, service.database.Read(), service.nodes, service.absences, holder.Reach, file)
	if err != nil {
		return Started{}, err
	}
	refused := library.RefuseUnreadable(ctx, service.database.Read(), service.nodes, place.NodeID,
		place.ReportedPathRaw)
	if refused != nil {
		return Started{}, refused
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		return Started{}, err
	}
	offsetMs, err := service.soundOffsetMs(ctx, userID, request.DeviceID)
	if err != nil {
		return Started{}, err
	}
	started := Started{SoundOffsetMs: offsetMs, FileBitRateBps: probe.Format.BitRateBps}
	if started.Subtitles, err = service.subtitles.Want(ctx, holder.Reach, file.ID); err != nil {
		return Started{}, err
	}
	used, err := service.startingChoices(ctx, userID, file.ID, probe, &started)
	if err != nil {
		return Started{}, err
	}
	// The starting sound's levelling is read before the decision, which a raise the device leaves to
	// the server changes (D436).
	err = service.startingPoint(ctx, userID, file, probe, &started)
	if err != nil {
		return Started{}, err
	}
	decision := decisionFor(request, probe, started)
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return Started{}, err
	}
	playback := tokenPlayback{
		userID: userID, sessionKey: holder.SessionKey, fileID: file.ID, libraryIDs: place.LibraryIDs,
		deviceID: request.DeviceID, decision: decision,
		isOwnPictureUnplayable: service.isOwnPictureUnplayable(holder, request, file.ID),
		ownSoundTracks:         ownSoundTracksOf(probe, request.PlayableFormats),
		// A person who plays without levelling hears the film's sound unlevelled, carrying no gain
		// for the player to take back out through Web Audio, whose latency, half a second on an
		// Android TV, plays the sound that much later (the owner's TCL TV, 2026-10-24).
		isSoundUnlevelled: !started.Preferences.IsLevellingOn,
	}
	playback.used, playback.startingAudio = used, started.StartingAudio
	started.OnLadder, started.NotOnLadderBecause, err = service.startOnLadder(ctx, holder, request, playback,
		started.ResumePositionMs)
	if err != nil {
		return Started{}, err
	}
	decision = playback.settleRaise(&started)
	service.keepStart(&playback, started.OnLadder)
	timing := service.timingNow()
	link := speedKey{
		player: playerOf(request.DeviceID, holder.SessionKey), network: clientnet.Of(request.ClientAddress),
	}
	if started.Steadiness, started.RememberedLink, err = service.rememberedAtStart(ctx, link, timing); err != nil {
		return Started{}, err
	}
	// A film its device cannot play as it is plays from its ladder, the one way to convert it (D344 §1).
	if decision.Delivery == decide.HLS && !playback.isOnLadder {
		return Started{}, fmt.Errorf("file %d, %s: %w", file.ID, started.NotOnLadderBecause,
			cannotConvertBecause(started.NotOnLadderBecause))
	}
	return service.issued(ctx, playback, probe, started, idle)
}

// rememberedAtStart is what a player's start on a network is told of its link there: the steadiness
// level its last playback there ended at, relaxed by the time since (D352 Q2), and the link it last
// measured there, the zero value for none (D257 §1, bandwidth-sampling-2026-10-29.md P2).
func (service *Service) rememberedAtStart(
	ctx context.Context, link speedKey, timing playbackTiming,
) (int, RememberedLink, error) {
	now := service.clock.Now()
	steadiness, err := service.speeds.steadiness(ctx, link, now, timing.speedFreshFor, timing.steadinessRelaxesEvery)
	if err != nil {
		return 0, RememberedLink{}, err
	}
	speed, isRemembered, err := service.speeds.fresh(ctx, link, now, timing.speedFreshFor)
	if err != nil || !isRemembered {
		return steadiness, RememberedLink{}, err
	}
	return steadiness, RememberedLink{
		Kbps: speed.kbps, LogSpread: speed.logSpread, AgeMs: now.Sub(speed.at).Milliseconds(),
		CountsAsSamples: speed.countsAsSamples(now, timing.speedFreshFor),
	}, nil
}

// ownSoundTracksOf are the file's audio tracks, by their place among them, a device of formats plays
// as they are, which the top of the file's ladder plays copied (D368 §1, D336 §1).
func ownSoundTracksOf(probe nodeproto.Probe, formats decide.PlayableFormats) []int {
	var tracks []int
	for index, stream := range decide.AudioStreams(probe) {
		if decide.PlaysAsItIs(stream, formats) {
			tracks = append(tracks, index)
		}
	}
	return tracks
}

// isOwnPictureUnplayable is whether the asking player gave fileID's own picture up as unplayable on
// its device (D266): on an earlier playback, as its report said, or as this request says, which is
// kept first, so a page asking again at once is offered the twin (D323 §2).
func (service *Service) isOwnPictureUnplayable(holder Holder, request Request, fileID int64) bool {
	player := playerOf(request.DeviceID, holder.SessionKey)
	if request.HasGivenUpOwnPicture {
		service.unplayablePictures.remember(player, fileID)
	}
	return service.unplayablePictures.has(player, fileID)
}

// issued is started with its tracks named and its media token issued, which an idle playback loses
// after idle.
func (service *Service) issued(
	ctx context.Context, playback tokenPlayback, probe nodeproto.Probe, started Started, idle time.Duration,
) (Started, error) {
	var err error
	if started.Tracks, err = service.tracksOf(ctx, playback, probe, started); err != nil {
		return Started{}, err
	}
	if started.Token, err = service.issueToken(playback, idle); err != nil {
		return Started{}, err
	}
	return started, nil
}

// issueToken issues the playback's media token, and has the chunks forget each film it left with no
// playback: their run tracker otherwise keeps every film ever played (the audit of 2026-10-25, C5).
func (service *Service) issueToken(playback tokenPlayback, idle time.Duration) (string, error) {
	token, unplayed, err := service.tokens.issue(service.random, playback, service.clock.Now(), idle)
	if err != nil {
		return "", err
	}
	if service.chunks != nil {
		for _, fileID := range unplayed {
			service.chunks.ForgetFile(fileID)
		}
	}
	return token, nil
}

// startingPoint is where a playback of file starts, in started, its resume position and the
// levelling of its starting track played as it is.
func (service *Service) startingPoint(
	ctx context.Context, userID int64, file sqlcgen.GetPlayableFileRow, probe nodeproto.Probe, started *Started,
) error {
	var err error
	started.ResumePositionMs, err = service.resumePositionMs(ctx, userID, file.ID, probe.Format.DurationMs)
	if err != nil {
		return err
	}
	levels, err := service.loudness.Levels(ctx, file.PayloadHash)
	if err != nil {
		return err
	}
	started.levels = levels
	started.Levelling = startingLevelling(levels, probe, started.StartingAudio)
	return nil
}

// startingLevelling is the levelling of the sound a playback starts with, played as it is: its
// starting track's own, the main track's where it names none. A playback starting on another track
// was levelled by the main track's gain, wrong by the difference between the two tracks' loudness
// (audio-switch-and-retiming-2026-11-01.md §1.1).
func startingLevelling(levels decide.Levels, probe nodeproto.Probe, starting *AudioTrack) decide.Levelling {
	if starting == nil {
		return decide.DirectLevelling(levels, probe)
	}
	return decide.AsItIsLevelling(levels, probe, starting.AudioTrackIndex)
}

// ladderLookEvery is how often a playback of a film not cut yet looks again for its keyframes,
// while it waits for them for play_ladder_wait_ms (D260 §1).
const ladderLookEvery = 100 * time.Millisecond

// startOnLadder starts a playback on its film's ladder from positionMs (D260): at the top, or, with
// its player's link on the request's network remembered, where that link calls for (D257 §1). A
// film with no grid yet is waited for, up to play_ladder_wait_ms, while its timeline is read, then cut
// on time where its device cannot play it as it is (D344 §2), else nil, playing as it is; one that
// cannot be cut, or offers no picture, is nil at once. A nil
// offer comes with why, for the log. A person who chose the original file plays it, never the ladder
// (web-client.md: playback is never a black box). Its player's playbacks of other films are gone to
// this one (goneToAnother).
func (service *Service) startOnLadder(
	ctx context.Context, holder Holder, request Request, playback tokenPlayback, positionMs int64,
) (*chunks.Offer, string, error) {
	if service.chunks == nil {
		return nil, "noLadder", nil
	}
	if err := service.goneToAnother(ctx, playerOf(request.DeviceID, holder.SessionKey), playback.fileID); err != nil {
		return nil, "", err
	}
	if request.PreferredDelivery == decide.Direct {
		return nil, "originalChosen", nil
	}
	key := speedKey{player: playerOf(request.DeviceID, holder.SessionKey), network: clientnet.Of(request.ClientAddress)}
	speed, _, err := service.speeds.fresh(ctx, key, service.clock.Now(), service.timingNow().speedFreshFor)
	if err != nil {
		return nil, "", err
	}
	kbps := speed.kbps
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
	isConverted := playback.decision.Delivery == decide.HLS
	offer, startErr := service.startWaiting(ctx, chunked, positionMs, kbps, waitUntil, isConverted)
	switch {
	case errors.Is(startErr, chunks.ErrNoGrid):
		return nil, "noGridYet", nil
	case errors.Is(startErr, chunks.ErrCannotBeCut):
		return nil, "cannotBeCut", nil
	case errors.Is(startErr, chunks.ErrNoQualityOffered):
		return nil, "noQualityOffered", nil
	case errors.Is(startErr, chunks.ErrTooSlowToEncode):
		return nil, notOnLadderTooSlow, nil
	case startErr != nil:
		return nil, "", startErr
	}
	return &offer, "", nil
}

// startWaiting starts chunked on its film's ladder from positionMs, looking again while the film has
// no grid until waitUntil. Its keyframes not read by then, a film its device cannot play as it is,
// isConverted, is cut on time, which needs none (D344 §2); one it can is ErrNoGrid, and plays as it
// is while they are read (D260 §1).
func (service *Service) startWaiting(
	ctx context.Context, chunked chunks.Playback, positionMs, kbps int64, waitUntil time.Time, isConverted bool,
) (chunks.Offer, error) {
	for {
		offer, err := service.chunks.StartOnLadder(ctx, chunked, positionMs, kbps)
		if !errors.Is(err, chunks.ErrNoGrid) {
			return offer, err
		}
		isWaiting := service.clock.Now().Before(waitUntil)
		if isWaiting && service.clock.Sleep(ctx, ladderLookEvery) == nil {
			continue
		}
		if !isConverted || chunked.IsOnTime {
			return offer, err
		}
		chunked.IsOnTime = true
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
	if isFound && isOtherFile {
		return file, fmt.Errorf("file %d: %w", fileID, library.ErrNoFile)
	}
	if isFound {
		if reachErr := library.ReachFile(ctx, service.database.Read(), holder.Reach, fileID); reachErr != nil {
			return file, reachErr
		}
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
	if err := service.reads.ReadNow(ctx, file.NodeID, file.ReportedPathRaw); err != nil {
		return errors.Join(refusal, fmt.Errorf("ask for file %d's read: %w", file.ID, err))
	}
	return refusal
}

// decisionFor is how a file plays for the client that asked, with the audio track the playback
// starts with, the person's dialogue boost (D336 §2), and a raise levelling wants of the sound as it
// is where the device's sound through Web Audio lags past D436's bound, so the player cannot make it.
func decisionFor(request Request, probe nodeproto.Probe, started Started) decide.Decision {
	input := decide.Input{
		Probe: probe, PlayableFormats: request.PlayableFormats,
		PreferredDelivery: request.PreferredDelivery,
		IsDialogueBoostOn: started.Preferences.IsDialogueBoostOn,
		IsRaiseTheServers: request.IsWebAudioLate && started.Preferences.IsLevellingOn &&
			started.Levelling.IsMeasured && started.Levelling.GainDB > 0,
	}
	if started.StartingAudio != nil {
		input.AudioStreamIndex = new(int(started.StartingAudio.StreamIndex))
	}
	return decide.Decide(input)
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

// soundOffsetMs is how far a person's device moves its sound from its picture; 0 when there is no
// device, as for a Watch Together viewer, or none the person has.
func (service *Service) soundOffsetMs(ctx context.Context, userID, deviceID int64) (int64, error) {
	if deviceID == 0 {
		return 0, nil
	}
	device, err := store.OrNotFound(service.database.Read().GetLiveDeviceOfUser(ctx, sqlcgen.GetLiveDeviceOfUserParams{
		ID: deviceID, UserID: &userID,
	}))
	if errors.Is(err, store.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read device %d: %w", deviceID, err)
	}
	return device.SoundOffsetMs, nil
}

// Source is where a direct playback's bytes are: a node, the target on its byte socket, and
// the type to serve them as; and the place they are read at, the path its node reported.
type Source struct {
	NodeID      int64
	Target      string
	ContentType string
	PlacePath   []byte
}

// DirectSource is where the bytes of the file a media token refers to are.
func (service *Service) DirectSource(ctx context.Context, token string, holder Holder) (Source, error) {
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return Source{}, err
	}
	playback, found := service.tokens.use(token, holder, service.clock.Now(), idle)
	if !found {
		return Source{}, ErrNoToken
	}
	file, err := library.PlayableFile(ctx, service.database.Read(), playback.fileID)
	if err != nil {
		return Source{}, err
	}
	// From a place its holder reaches, the first that can be read now (D363 §8).
	place, err := library.PlaceToRead(ctx, service.database.Read(), service.nodes, service.absences, holder.Reach, file)
	if err != nil {
		return Source{}, err
	}
	bytesFile, err := library.BytesFileOf(ctx, service.database.Read(), place)
	if err != nil {
		return Source{}, err
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		return Source{}, err
	}
	return Source{
		NodeID: place.NodeID, Target: bytesFile.Target(), ContentType: contentType(probe.Format),
		PlacePath: place.ReportedPathRaw,
	}, nil
}

// NotThere is a direct playback's read of source that its node answered with no file there, or not
// the one named, for why: the next DirectSource of the file is another place its holder reaches,
// until a scan sees the file there again (D363 §8).
func (service *Service) NotThere(ctx context.Context, source Source, why string) {
	service.absences.Note(ctx, source.NodeID, source.PlacePath, why)
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

// AwaitReadable is Unreadable, held for up to wait while the file's root is unavailable: its node is
// asked whether the root can be read every probeEvery, outside every queue, and the answer comes as
// soon as one finds it back (D325 §2, D327 §3). A page waiting on a share's return asked every ten
// seconds, and heard of it only from the node's next status (staging, 2026-10-21).
func (service *Service) AwaitReadable(ctx context.Context, fileID int64, wait time.Duration) (
	*library.Unreadable, error,
) {
	deadline := service.clock.Now().Add(wait)
	for {
		unreadable, err := service.Unreadable(ctx, fileID)
		if err != nil || unreadable == nil || unreadable.Reason != library.RootMissing || service.roots == nil {
			return unreadable, err
		}
		_, isBack, err := service.roots.ProbeAndBringBack(ctx, unreadable.NodeID, unreadable.Path)
		if err != nil {
			return nil, err
		}
		if isBack {
			continue
		}
		left := deadline.Sub(service.clock.Now())
		if left <= 0 {
			return unreadable, nil
		}
		if err := service.clock.Sleep(ctx, min(left, probeEvery)); err != nil {
			return unreadable, nil //nolint:nilerr // a page that left needs no answer; this one is as true as any
		}
	}
}

// probeEvery is how often a held ask probes the root of a file it waits on: a probe costs its node
// nothing, and one running is shared, so a waiting page hears of a share back within it (D327 §2).
const probeEvery = 2 * time.Second

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
	files, err := service.database.Read().ListUnreadFilesOfWork(ctx, titleID)
	if err != nil {
		return fmt.Errorf("list title %d's files still being read: %w", titleID, err)
	}
	for _, file := range files {
		if err := service.reads.ReadNow(ctx, file.NodeID, file.ReportedPathRaw); err != nil {
			return fmt.Errorf("ask for file %d's read: %w", file.ID, err)
		}
	}
	return nil
}

// TrackLevellings are the levelling of each of a file's audio tracks, by its place among them, played
// as it is: what a playback started now would level each by, which a player whose film was measured
// as it played takes up without starting again (D226 §6). A file not read yet, or never probed, has
// none.
func (service *Service) TrackLevellings(ctx context.Context, fileID int64) ([]decide.Levelling, error) {
	file, err := library.PlayableFile(ctx, service.database.Read(), fileID)
	if errors.Is(err, library.ErrNoFile) || errors.As(err, new(*library.UnreadableError)) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		return nil, fmt.Errorf("read file %d's probe: %w", fileID, err)
	}
	levels, err := service.loudness.Levels(ctx, file.PayloadHash)
	if err != nil {
		return nil, err
	}
	tracks := decide.AudioStreams(probe)
	levellings := make([]decide.Levelling, 0, len(tracks))
	for index := range tracks {
		levellings = append(levellings, decide.AsItIsLevelling(levels, probe, index))
	}
	return levellings, nil
}
