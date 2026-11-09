// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// secretBytes is a link's or a membership credential's length before encoding: 256 random bits,
// which no one guesses.
const secretBytes = 32

// The codes a refused command or join is told (cut-a §5.2).
const (
	codeForbidden = "forbidden"
	codeStale     = "stale"
	codeInvalid   = "invalid_request"
	codeNotFound  = "not_found"
)

var (
	// ErrNoSession is a session, link or credential that is not, or no longer, a live one.
	ErrNoSession = errors.New("no such Watch Together session, or the link was used or withdrawn")
	// ErrNotHost is something only the host may do.
	ErrNotHost = errors.New("only the session's host may do that")
	// ErrNoMember is a member the session does not hold.
	ErrNoMember = errors.New("no such member")
	// ErrNoLink is a link the session does not hold.
	ErrNoLink = errors.New("no such link")
	// ErrNoNote is a link without the host's note of who it is for, which it must have (D246 §3).
	ErrNoNote = errors.New("a link needs a note of who it is for")
)

// Person is a signed-in person a member is, or nobody.
type Person struct {
	UserID int64
	Name   string
}

// Sender takes the messages for one device: its WebSocket's writer, which must never block.
type Sender func(message Outbound)

// MayPlay is whether a person may play a file now: the libraries their role reaches (D243 §4).
type MayPlay func(ctx context.Context, userID, fileID int64) bool

// KeyframeAtOrBefore is where the last keyframe of a file's picture at or before positionMs is, the
// start of the chunk holding it, and whether the file is cut so it is known (D285 §1).
type KeyframeAtOrBefore func(ctx context.Context, fileID, positionMs int64) (int64, bool)

// Hub holds the live sessions, in memory: they end with the process (D119).
type Hub struct {
	clock   clock.Clock
	random  io.Reader
	mayPlay MayPlay
	// keyframeAtOrBefore, when set, snaps a long seek back to a keyframe (SnapSeeksBy).
	keyframeAtOrBefore KeyframeAtOrBefore
	// logger hears every change of each session's group (the owner, 2026-10-06).
	logger *slog.Logger
	// onLinksChanged is told when a link or a locator is made (OnLinksChanged); nil tells nothing.
	onLinksChanged func()
	// onSessionEnded is told each Watch Together session that ends, after the hub's lock is let go
	// (OnSessionEnded); nil tells nothing.
	onSessionEnded func(sessionID int64)

	mutex sync.Mutex
	// lifetimes are what links made now live for, and when an idle session ends (EndStale); guarded
	// by mutex.
	lifetimes Lifetimes
	sessions  map[int64]*session // guarded by mutex
	// endedCasts are the credentials of the casts that ended within endedCastKept, the oldest
	// first, each with why (cast.go); guarded by mutex.
	endedCasts []endedCast
	// endedToTell are the sessions ended since the lock was taken, which onSessionEnded is told as
	// it is let go (unlockTellingEnds); guarded by mutex.
	endedToTell []int64
	lastID      int64 // the last id given a session, link, member or device; guarded by mutex
}

// NewHub returns an empty hub, which asks mayPlay whether a host may play what they put on, and
// logs each group's changes to logger.
func NewHub(clockSource clock.Clock, random io.Reader, mayPlay MayPlay, logger *slog.Logger) *Hub {
	return &Hub{
		clock: clockSource, random: random, mayPlay: mayPlay, logger: logger, sessions: map[int64]*session{},
		lifetimes: startingLifetimes(),
	}
}

// session is one Watch Together session (D122): what plays is state on it, the host's to change.
type session struct {
	id         int64
	hostUserID int64
	// isCast is a cast's session of one (cast.go), which only the cast's own calls find.
	isCast bool
	group  group
	// leaderID is the device that leads the session, the screen that is the film (D371 §1): the
	// host's device that plays, the newest attached or the one that put the film on, else the player
	// present longest (D372 §1; leader.go); 0 while none.
	leaderID int64
	// leadsBecause is why leaderID leads, as its leaderChanged line says it: the why, and a leader
	// gone's goneBecause after a colon ("leaderGone:hidden"); the roster carries it for a debug view.
	leadsBecause string
	// driftLeftAlone is how far off the group a follower is left alone (together_drift_left_alone_ms,
	// D250 §1), as the last device to attach brought it: within it, a host's device that came back
	// plays in step and takes the lead back (D372 §3).
	driftLeftAlone time.Duration
	// snapSeeksBeyond is how far a seek must jump to be snapped to a keyframe: a jump along the
	// timeline is imprecise, and no one notices it land a little early, but a short skip, 5 to 15 s by
	// a key or a button, snapped back to where it started would never move on (D285 §1). The
	// setting's, as the last device to attach brought it.
	snapSeeksBeyond time.Duration
	links           map[int64]*link
	members         map[int64]*member
	// lastRights are the rights of the last link made, which the next one starts with (D271 §4).
	lastRights Rights
	// lastRoster is the roster every device was last told (sendRoster).
	lastRoster []MemberView
	// logger is the hub's: each change of the group is logged with the session's id.
	logger *slog.Logger
	// lastActiveAt is when a member last did something in it, or it last played (EndStale, D346).
	lastActiveAt time.Time
}

// group is the group's state: Position is where it was at At, moving at Rate while it plays.
type group struct {
	seq      int64
	state    GroupState
	position time.Duration
	at       time.Time
	rate     float64
	fileID   int64
	event    EventKind
	byMember int64
	// seekedFrom is where the last event, a seek, jumped from.
	seekedFrom time.Duration
	lastSeek   int64 // the seq of the last seek, for a stale one's conflict
	// changedAt is when the change numbered seq was announced: a player's note that it followed that
	// change says how long after it the player moved (NoteGroupFollowed).
	changedAt time.Time
	// isHeldPlaying is whether a Watch Together group held for want of a player, Paused where its
	// last contender went, was playing then, and so plays on as one comes back (D375 §1, §2): the
	// hold is no person's pause. Only a Paused group has it; a person's pause, play, film put on or
	// stop ends it.
	isHeldPlaying bool
}

// member is a person in the session, with their devices (D120).
type member struct {
	id     int64
	name   string
	isHost bool
	// hostNote is the host's note of who the member is, from their link; only the host sees it.
	hostNote string
	// placeID is the place their link gave, which a new link for them gives again.
	placeID int64
	// rights are a guest's, from their link, for as long as they are in; the host may do anything.
	rights Rights
	person *Person
	// credentials are the credentials its devices say hello with, the newest last: one per
	// device that had the membership, so having it back on one cuts off no other (D267 §3).
	credentials []credential
	devices     map[int64]*device
}

// device is one connected client of a member.
type device struct {
	id int64
	// signedInDevice is the person's device it said hello signed in as (SignedIn.DeviceID), or 0
	// for a device that said hello signed in to nothing, as a guest's by link alone.
	signedInDevice int64
	role           DeviceRole
	canSetRate     bool
	buffering      bool
	suspended      SuspendReason
	rttMs          int64
	// hasBeenReady is whether it has once said it is ready to play: until then the roster shows it
	// loading.
	hasBeenReady bool
	// cannotKeepUp is what the device last said of a link too slow for its lowest quality (D247 §8).
	cannotKeepUp *CannotKeepUp
	// playing is whether its player last said it plays, and playerChangedAt when it last said it
	// began or stopped playing or buffering; zero until its first state.
	playing         bool
	playerChangedAt time.Time
	// standing is what its page last said it waits on.
	standing Standing
	// playback is how its player last said it plays the film: delivered as it is or converted, at
	// which quality, on what link (D371 §4); nil until it says.
	playback *Playback
	// runSince is the start of its current run, its place in line for the lead: when the core first
	// heard it playing or ready with the session's film since its last run ended, by the core's clock
	// at microsecond resolution; zero while it has none (D377 §2; takeRun).
	runSince time.Time
	// isPageHidden is its page's word that its browser hid it, its film paused (a locked phone): it
	// cannot lead while it is (D372 §5).
	isPageHidden bool
	// stalledSince is when its state first said it buffers while its Watch Together group played,
	// which the group does not wait for (D373 §1); zero while it plays on, and once the group stops
	// playing, its loading then the start's (takeStall).
	stalledSince time.Time
	// playerFileID is the film its player has loaded, as its latest state said, or as it put it on
	// itself; 0 while it has none: not yet loaded, its player gone, or its film handed to a cast's
	// receiver. Only a device whose player has the session's film contends for the lead (D374 §1).
	playerFileID int64
	// hasContended is whether it has contended for the lead since it attached: its first time is its
	// arriving as a player, when the newest of the host's takes the lead (arrived, D371 §3).
	hasContended bool
	// catchingUpSince is when it, the leader, played again after a stall its group played on through
	// within the grace: until its state is in step with the group it catches up as a follower would,
	// and its states do not place the group, so no one sees a stretch twice (D374); zero otherwise.
	catchingUpSince time.Time
	// place is where its latest state put its player, and whether that is a position of its own, to
	// present should it be chosen to lead a group no one leads (D377 §4; placeByNewLeader).
	place devicePlace
	send  Sender
}

// takeState keeps what a device says of itself: a state report's buffering, round trip and playback,
// the first that is not buffering making it one that has been ready; a suspension, or its end.
func (each *device) takeState(message Inbound) {
	switch message.Type {
	case MessageState:
		each.buffering = message.IsBuffering
		each.playerFileID = message.FileID
		if !message.IsBuffering && message.FileID != 0 {
			each.hasBeenReady = true
		}
		if message.RTTMs > 0 {
			each.rttMs = message.RTTMs
		}
		each.cannotKeepUp = message.CannotKeepUp
		if message.Playback != nil && message.Playback.isKnown() {
			each.playback = message.Playback
		}
	case MessageSuspend:
		each.suspended = message.Reason
	case MessageResume:
		each.suspended = ""
	case MessagePlayback:
		if message.Playback != nil && message.Playback.isKnown() {
			each.playback = message.Playback
		}
	case MessageHello, MessagePing, MessageCommand, MessageRTC, MessageNote, MessageStanding, MessageSound,
		MessagePicture, MessageWelcome, MessagePong, MessageGroup,
		MessageRoster, MessageRejected, MessageEnded:
	}
}

// Opened is the host's session: its id, and a membership credential the host's devices say hello
// with.
type Opened struct {
	SessionID            int64
	MembershipCredential string
}

// Open starts a session hosted by host, nothing playing. A person is in one session at most (D372
// §8): starting one ends the one they host, its devices told the host ended it, and takes them out
// of one they joined, its devices told they left (the owner: starting a new session "signals an
// intent to end the old one"). A page reloaded has its session back by Mine.
func (hub *Hub) Open(host Person) (Opened, error) {
	credential, err := randbytes.Base64URLSecret(hub.random, secretBytes)
	if err != nil {
		return Opened{}, fmt.Errorf("make a membership credential: %w", err)
	}
	hub.mutex.Lock()
	defer hub.unlockTellingEnds()
	for _, held := range hub.placesOf(&host, "", 0) {
		hub.leavePlace(held)
	}
	hub.lastID++
	opened := &session{
		id: hub.lastID, hostUserID: host.UserID, links: map[int64]*link{}, members: map[int64]*member{},
		group: group{state: GroupIdle, rate: 1, at: hub.clock.Now()}, logger: hub.logger,
		snapSeeksBeyond: StartingSnapSeeksBeyond, lastRights: everyRight(),
		lastActiveAt: hub.clock.Now(),
	}
	hub.lastID++
	hostPerson := host
	opened.members[hub.lastID] = &member{
		id: hub.lastID, name: host.Name, isHost: true, person: &hostPerson, devices: map[int64]*device{},
	}
	opened.members[hub.lastID].addCredential(madeCredential{credential: credential})
	hub.sessions[opened.id] = opened
	return Opened{SessionID: opened.id, MembershipCredential: credential}, nil
}

// LinkMade is a new link: its id, and its secret, which goes after # in the address (D243 §3).
type LinkMade struct {
	LinkID int64
	Secret string
}

// LinkView is a link as its host sees it.
type LinkView struct {
	ID       int64
	HostNote string
	// Secret is the link's secret, for the host to pass on again (D267 §5).
	Secret string
	// UsedBy is the member who joined by it, or 0 while it waits.
	UsedBy int64
	Rights Rights
	// ExpiresAt is when it stops admitting anyone, while it waits (D346).
	ExpiresAt time.Time
}

// MemberDetail is a member as their host sees them: with the note of the link they joined by.
type MemberDetail struct {
	ID       int64
	Name     string
	HostNote string
	IsHost   bool
}

// Details is a session as its host sees it: its links and members, with the host's notes, and
// what plays.
type Details struct {
	SessionID int64
	Links     []LinkView
	Members   []MemberDetail
	Group     GroupView
	// NextRights are what the next link starts with: the last one's, or every right for the first
	// (D271 §4).
	NextRights Rights
}

// DetailsOf is the session sessionID, for its host.
func (hub *Hub) DetailsOf(sessionID, hostUserID int64) (Details, error) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hosted, err := hub.hostedSession(sessionID, hostUserID)
	if err != nil {
		return Details{}, err
	}
	now := hub.clock.Now()
	details := Details{SessionID: hosted.id, Group: *hosted.groupView(now), NextRights: hosted.lastRights}
	for _, each := range hosted.links {
		if each.usedBy == 0 && !each.waits(now) {
			continue // expired: EndStale has not yet removed it
		}
		details.Links = append(details.Links, LinkView{
			ID: each.id, HostNote: each.hostNote, Secret: each.secret, UsedBy: each.usedBy, Rights: each.rights,
			ExpiresAt: each.expiresAt,
		})
	}
	for _, each := range hosted.members {
		details.Members = append(details.Members, MemberDetail{
			ID: each.id, Name: each.name, HostNote: each.hostNote, IsHost: each.isHost,
		})
	}
	return details, nil
}

// Joined is a membership: its session, and the credential its devices say hello with, which is
// also what its player plays by (D246 §3).
type Joined struct {
	SessionID            int64
	MemberID             int64
	MembershipCredential string
	// Locator is what the membership's device finds the core by through a discovery (OfferKey).
	Locator string
}

// Eject removes a member: each of their devices is told the session ended for them, and their
// credential admits nothing more (D119). Only the host ejects, and never themselves.
func (hub *Hub) Eject(sessionID, hostUserID, memberID int64) error {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hosted, err := hub.hostedSession(sessionID, hostUserID)
	if err != nil {
		return err
	}
	ejected, found := hosted.members[memberID]
	if !found || ejected.isHost {
		return ErrNoMember
	}
	hosted.remove(hub.clock.Now(), ejected, "ejected")
	return nil
}

// Leave takes a member out of the session at their own asking (D246 §5); the host ends it instead.
func (hub *Hub) Leave(credential string) error {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	left, leaving := hub.memberByCredential(credential)
	if leaving == nil {
		return ErrNoSession
	}
	if leaving.isHost {
		return ErrNotHost
	}
	left.remove(hub.clock.Now(), leaving, "left")
	return nil
}

// End ends the session: every device is told, and every credential and link admits nothing more.
func (hub *Hub) End(sessionID, hostUserID int64) error {
	hub.mutex.Lock()
	defer hub.unlockTellingEnds()
	hosted, err := hub.hostedSession(sessionID, hostUserID)
	if err != nil {
		return err
	}
	hub.endSession(hosted, "hostEnded")
	return nil
}

// OnSessionEnded has ended called with each Watch Together session that ends, however it ends: its
// host's end, its host starting another, or its idle end; a cast's own end is not one. The casts
// whose receiver takes a part in it end with it (D389 §1). Called before the hub is used; ended is
// called once the hub's lock is let go, so it may call the hub.
func (hub *Hub) OnSessionEnded(ended func(sessionID int64)) {
	hub.onSessionEnded = ended
}

// endSession ends the Watch Together session ended: each of its devices is told why, every
// credential and link admits nothing more, and onSessionEnded is told as the lock is let go
// (unlockTellingEnds). The one way a Watch Together session ends; the caller holds the mutex.
func (hub *Hub) endSession(ended *session, reason string) {
	ended.broadcast(Outbound{V: ProtocolVersion, Type: MessageEnded, Reason: reason})
	delete(hub.sessions, ended.id)
	hub.endedToTell = append(hub.endedToTell, ended.id)
}

// unlockTellingEnds lets the mutex go, then tells onSessionEnded each session ended while it was
// held (endSession): every method that may end a session lets its lock go here.
func (hub *Hub) unlockTellingEnds() {
	ended := hub.endedToTell
	hub.endedToTell = nil
	hub.mutex.Unlock()
	if hub.onSessionEnded == nil {
		return
	}
	for _, id := range ended {
		hub.onSessionEnded(id)
	}
}

// Viewer is a member who is not the host, as their player plays: the film the session plays now,
// and whose libraries it must be in, the host's (D246 §3, D243 §4).
type Viewer struct {
	SessionID  int64
	MemberID   int64
	HostUserID int64
	// FileID is what the session plays now, or 0 while nothing does.
	FileID int64
}

// ViewerOf is the viewer a membership credential names, while the membership lasts; false for an
// unknown credential and for the host's, who plays as themselves.
func (hub *Hub) ViewerOf(credential string) (Viewer, bool) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	viewed, viewing := hub.memberByCredential(credential)
	if viewing == nil || viewing.isHost {
		return Viewer{}, false
	}
	return Viewer{
		SessionID: viewed.id, MemberID: viewing.id, HostUserID: viewed.hostUserID, FileID: viewed.group.fileID,
	}, true
}

// Attachment is a device attached to its member: what it is, and how it leaves.
type Attachment struct {
	SessionID int64
	MemberID  int64
	DeviceID  int64
	// Person is the member's person, when any of their devices said hello signed in (D243 §2).
	Person *Person
}

// Attach attaches a device to the member its credential names, with a role, its messages going to
// send, and welcomes it: the session, its member, the group, the roster and how to follow the group,
// tuning. A device of a person signed in makes the membership theirs (D243 §2), and is known as
// that person's device (SignedInDevicesWith). A credential that names no member is ErrNoSession, a
// *CastEndedError saying why where a cast's just ended.
func (hub *Hub) Attach(
	credential string, role DeviceRole, canSetRate bool, signedIn *SignedIn, tuning Tuning, send Sender,
) (Attachment, error) {
	if !role.plays() && role != RolePanel {
		return Attachment{}, fmt.Errorf("role %q: want video, panel or both", role)
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	attached, attachedMember := hub.memberByCredential(credential)
	if attachedMember == nil {
		return Attachment{}, hub.noSessionFor(credential)
	}
	signedInDevice := attachedMember.takeSignedIn(signedIn)
	if tuning.SnapSeeksBeyondMs > 0 {
		attached.snapSeeksBeyond = time.Duration(tuning.SnapSeeksBeyondMs) * time.Millisecond
	}
	if tuning.LeaveDriftUpToMs > 0 {
		attached.driftLeftAlone = time.Duration(tuning.LeaveDriftUpToMs) * time.Millisecond
	}
	hub.lastID++
	now := hub.clock.Now()
	attachedDevice := &device{
		id: hub.lastID, signedInDevice: signedInDevice, role: role, canSetRate: canSetRate, send: send,
	}
	attachedMember.devices[hub.lastID] = attachedDevice
	send(Outbound{
		V: ProtocolVersion, Type: MessageWelcome, Seq: attached.group.seq, SessionID: attached.id,
		MemberID: attachedMember.id, DeviceID: hub.lastID, IsHost: attachedMember.isHost,
		Rights: new(attachedMember.rightsHeld()), Group: attached.groupView(now), Roster: attached.roster(),
		Tuning: &tuning,
	})
	attached.sendRoster()
	return Attachment{
		SessionID: attached.id, MemberID: attachedMember.id, DeviceID: hub.lastID, Person: attachedMember.person,
	}, nil
}

// Detach removes a device whose connection ended, because it closed or went silent. The group never
// stops for it (D371 §2): its member stays, and it catches up once it is back, its run ended (D377 §2).
// The leader leaving hands the lead on at once (leaderGone, D372 §5).
func (hub *Hub) Detach(attachment Attachment, because GoneBecause) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	detached, found := hub.sessions[attachment.SessionID]
	if !found {
		return
	}
	left, found := detached.members[attachment.MemberID]
	if !found {
		return
	}
	gone, found := left.devices[attachment.DeviceID]
	if !found {
		return
	}
	delete(left.devices, attachment.DeviceID)
	detached.endRun(left, gone, string(because))
	if gone.id == detached.leaderID {
		detached.leaderGone(hub.clock.Now(), left.isHost, because)
	}
	detached.sendRoster()
}

// SnapSeeksBy has a long seek snapped back to the keyframe keyframeAtOrBefore finds (D285 §1).
func (hub *Hub) SnapSeeksBy(keyframeAtOrBefore KeyframeAtOrBefore) {
	hub.keyframeAtOrBefore = keyframeAtOrBefore
}

// Handle takes one message from an attached device. A change of what plays is checked against
// what the host may play, and a seek's keyframe found, before the hub is held, since they read the
// store.
func (hub *Hub) Handle(ctx context.Context, attachment Attachment, message Inbound) {
	if message.Type == MessageCommand && message.Command == CommandChangeItem &&
		!hub.mayPlay(ctx, hub.hostOf(attachment.SessionID), message.FileID) {
		message.FileID = 0 // refused below, as one the host may not play
	}
	hub.findKeyframe(ctx, attachment, &message)
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	handled, found := hub.sessions[attachment.SessionID]
	if !found {
		return
	}
	sender, found := handled.members[attachment.MemberID]
	if !found {
		return
	}
	from, found := sender.devices[attachment.DeviceID]
	if !found {
		return
	}
	now := hub.clock.Now()
	switch message.Type {
	case MessagePing:
		// The pong says where the group is as it answers, so a follower can place the group by the
		// pong whose round trip it measured shortest, not by a message whose wait it cannot know (D264).
		from.send(Outbound{
			V: ProtocolVersion, Type: MessagePong, TMono: message.TMono, Seq: handled.group.seq,
			Group: handled.groupView(now),
		})
		handled.passStalledLead(now)
	case MessageState, MessageSuspend, MessageResume:
		handled.takeState(ctx, now, sender, from, message)
		handled.passStalledLead(now)
	case MessageCommand:
		handled.touch(now)
		handled.command(now, sender, from, message)
	case MessageRTC:
		handled.relay(from, message)
	case MessageNote, MessageStanding, MessageSound, MessagePicture, MessagePlayback:
		handled.takeAside(ctx, now, sender, from, message)
	case MessageHello, MessageWelcome, MessagePong, MessageGroup, MessageRoster, MessageRejected, MessageEnded:
		from.send(Outbound{V: ProtocolVersion, Type: MessageRejected, Code: codeInvalid})
	}
}

// takeState takes what a device says of itself, and everyone is told: whether its page is hidden
// and which film its player has, by which it contends for the lead or no longer does, a leader that
// no longer does handing it on (takeContention, D372 §5, D374 §1); a host's device in step taking
// the lead back from a guest's (D372 §3); the leader's state places the group (followLeader), and no
// one else's moves it, a follower's trouble its own to catch up on (D371 §2); no one's stall is
// waited for while another plays the film (takeStall, D373 §1, D374 §3). Its first word that even
// its lowest quality is beyond its link is logged.
func (current *session) takeState(ctx context.Context, now time.Time, who *member, from *device, message Inbound) {
	couldKeepUp := from.cannotKeepUp == nil
	wasContender := current.contends(from)
	current.logPlayer(ctx, now, who, from, message)
	from.takeState(message)
	if message.Type == MessageState {
		from.place = devicePlace{
			fileID: message.FileID, position: current.reportedPosition(from, message), at: now,
			isMoving: message.IsPlaying && !message.IsBuffering, isRemembered: message.RemembersPlace,
		}
		current.takeVisibility(who, from, message.IsPageHidden)
		current.takeRun(now, who, from, message)
		current.takeStall(ctx, now, who, from, message)
		current.takeContention(now, who, from, wasContender)
		current.takeLeadInStep(now, who, from, message)
	}
	if message.Type == MessageState && from.id == current.leaderID {
		current.followLeader(ctx, now, who, from, message)
	}
	if couldKeepUp && from.cannotKeepUp != nil {
		current.logger.InfoContext(ctx, "watch together: a device cannot keep up even at its lowest quality",
			append(current.deviceAttributes(who, from),
				"linkKbps", from.cannotKeepUp.LinkKbps, "lowestKbps", from.cannotKeepUp.LowestKbps)...)
	}
	current.sendRosterIfChanged()
}

// relay passes voice signalling to the device it names, in the same session (D193 §4).
func (current *session) relay(from *device, message Inbound) {
	for _, each := range current.members {
		if to, found := each.devices[message.ToDeviceID]; found {
			to.send(Outbound{
				V: ProtocolVersion, Type: MessageRTC, FromDeviceID: from.id,
				SDP: message.SDP, Candidate: message.Candidate,
			})
			return
		}
	}
}

// roster is every member and their devices, as everyone sees them: never the host's notes.
func (current *session) roster() []MemberView {
	views := make([]MemberView, 0, len(current.members))
	for _, each := range current.members {
		view := MemberView{ID: each.id, Name: each.name, IsHost: each.isHost, Devices: []DeviceView{}}
		for _, attached := range each.devices {
			isLeader := attached.id == current.leaderID
			leadsBecause := ""
			if isLeader {
				leadsBecause = current.leadsBecause
			}
			view.Devices = append(view.Devices, DeviceView{
				ID: attached.id, Role: attached.role, IsBuffering: attached.buffering,
				Suspended: attached.suspended, RTTMs: attached.rttMs, CannotKeepUp: attached.cannotKeepUp,
				Status: attached.status(current.group.fileID != 0, isLeader), IsLeader: isLeader,
				Playback: attached.playback, LeadsBecause: leadsBecause,
			})
		}
		slices.SortFunc(view.Devices, func(one, other DeviceView) int { return cmp.Compare(one.ID, other.ID) })
		views = append(views, view)
	}
	// In the order they joined, the host first: the members are kept by id, and a map's order is
	// chance, which swapped the names on everyone's screen (the owner's test, 2026-10-05).
	slices.SortFunc(views, func(one, other MemberView) int { return cmp.Compare(one.ID, other.ID) })
	return views
}

// sendRoster tells every device the roster.
func (current *session) sendRoster() {
	current.lastRoster = current.roster()
	current.broadcast(Outbound{V: ProtocolVersion, Type: MessageRoster, Roster: current.lastRoster})
}

// sendRosterIfChanged tells every device the roster when it is not the one they were last told: a
// device's state report comes each second while its group waits, from each device, and each sent
// every device the roster, the same one most of the time (the audit of 2026-10-25, R2).
func (current *session) sendRosterIfChanged() {
	if roster := current.roster(); !reflect.DeepEqual(roster, current.lastRoster) {
		current.sendRoster()
	}
}

// broadcast sends message to every device of every member, encoded once for them all.
func (current *session) broadcast(message Outbound) {
	message.encoded = &sharedEncoding{}
	for _, each := range current.members {
		for _, attached := range each.devices {
			attached.send(message)
		}
	}
}

// memberByCredential is the session and member a membership credential names, or nils.
func (hub *Hub) memberByCredential(credential string) (*session, *member) {
	hash := sha256.Sum256([]byte(credential))
	for _, each := range hub.sessions {
		for _, candidate := range each.members {
			if candidate.admits(hash) {
				return each, candidate
			}
		}
	}
	return nil, nil
}

// MemberOf is the session and member a membership credential names, host or viewer, while the
// membership lasts: whose relay credentials a device asks for (D243 §8).
func (hub *Hub) MemberOf(credential string) (sessionID, memberID int64, isMember bool) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	held, holder := hub.memberByCredential(credential)
	if holder == nil {
		return 0, 0, false
	}
	return held.id, holder.id, true
}

// IsMember is whether memberID is a member of the live session sessionID: what a voice relay's
// credentials are good for, which end with the membership (D243 §8).
func (hub *Hub) IsMember(sessionID, memberID int64) bool {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	if live, found := hub.sessions[sessionID]; found {
		_, isMember := live.members[memberID]
		return isMember
	}
	return false
}

// findKeyframe finds, for a seek, the keyframe at or before where it asks, when the hub snaps seeks.
func (hub *Hub) findKeyframe(ctx context.Context, attachment Attachment, message *Inbound) {
	if message.Type != MessageCommand || message.Command != CommandSeek || hub.keyframeAtOrBefore == nil {
		return
	}
	if fileID := hub.fileOf(attachment.SessionID); fileID != 0 {
		message.keyframeMs, message.isKeyframeKnown = hub.keyframeAtOrBefore(ctx, fileID, message.PositionMs)
	}
}

// fileOf is the file a session plays, or 0.
func (hub *Hub) fileOf(sessionID int64) int64 {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	if playing, found := hub.sessions[sessionID]; found {
		return playing.group.fileID
	}
	return 0
}

// hostOf is the user who hosts a session, or 0.
func (hub *Hub) hostOf(sessionID int64) int64 {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	if hosted, found := hub.sessions[sessionID]; found {
		return hosted.hostUserID
	}
	return 0
}

// hostedSession is the session hostUserID hosts by sessionID.
func (hub *Hub) hostedSession(sessionID, hostUserID int64) (*session, error) {
	hosted, found := hub.sessions[sessionID]
	if !found || hosted.isCast {
		return nil, ErrNoSession
	}
	if hosted.hostUserID != hostUserID {
		return nil, ErrNotHost
	}
	return hosted, nil
}
