// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"cmp"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

	mutex    sync.Mutex
	sessions map[int64]*session // guarded by mutex
	lastID   int64              // the last id given a session, link, member or device; guarded by mutex
}

// NewHub returns an empty hub, which asks mayPlay whether a host may play what they put on, and
// logs each group's changes to logger.
func NewHub(clockSource clock.Clock, random io.Reader, mayPlay MayPlay, logger *slog.Logger) *Hub {
	return &Hub{
		clock: clockSource, random: random, mayPlay: mayPlay, logger: logger, sessions: map[int64]*session{},
	}
}

// session is one Watch Together session (D122): what plays is state on it, the host's to change.
type session struct {
	id         int64
	hostUserID int64
	group      group
	// syncWait is how long a sync point waits for the players (D250 §3): the setting's, as the last
	// device to attach brought it.
	syncWait time.Duration
	// startLead is how long after a sync point's wait ends the group starts moving (D268 §3): the
	// setting's, as the last device to attach brought it; none until one has.
	startLead time.Duration
	// runUp is the start's lead while a player has not yet learned how late it starts (D282): the
	// setting's, as the last device to attach brought it; none until one has.
	runUp   time.Duration
	links   map[int64]*link
	members map[int64]*member
	// lastRights are the rights of the last link made, which the next one starts with (D271 §4).
	lastRights Rights
	// logger is the hub's: each change of the group is logged with the session's id.
	logger *slog.Logger
}

// link is one person's way in (D246 §3): single use, with the host's note of who it is for.
type link struct {
	id         int64
	secretHash [sha256.Size]byte
	// secret is the link itself, kept for the host to pass on again: a page reloaded before it was
	// shared lost it, and a guest comes back by it on the browser that joined (D267 §1, §5).
	secret   string
	hostNote string
	// rights are what the person who joins by it may do to playback (D271 §4).
	rights Rights
	// usedBy is the member who joined by it, or 0 while it waits.
	usedBy int64
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
	// waitUntil is when a group Waiting at a sync point stops waiting for players not yet ready.
	waitUntil time.Time
}

// member is a person in the session, with their devices (D120).
type member struct {
	id     int64
	name   string
	isHost bool
	// hostNote is the host's note of who the member is, from their link; only the host sees it.
	hostNote string
	// rights are a guest's, from their link, for as long as they are in; the host may do anything.
	rights Rights
	person *Person
	// credentialHashes are the credentials its devices say hello with, the newest last: one per
	// device that had the membership, so having it back on one cuts off no other (D267 §3).
	credentialHashes [][sha256.Size]byte
	devices          map[int64]*device
}

// device is one connected client of a member.
type device struct {
	id         int64
	role       DeviceRole
	canSetRate bool
	buffering  bool
	suspended  SuspendReason
	rttMs      int64
	// hasBeenReady is whether it has once said it is ready to play along: until then the group
	// does not wait for it, so one still choosing its speakers or loading holds no one up (D246 §6).
	hasBeenReady bool
	// isCatchingUp is whether the group plays on without it until it is ready again (D247 §4).
	isCatchingUp bool
	// knowsStartLag is whether its player has learned how late it starts, which it said in its last
	// state: one that has not is given a run-up (D282).
	knowsStartLag bool
	// cannotKeepUp is what the device last said of a link too slow for its lowest quality (D247 §8).
	cannotKeepUp *CannotKeepUp
	// playing is whether its player last said it plays, and playerChangedAt when it last said it
	// began or stopped playing or buffering; zero until its first state.
	playing         bool
	playerChangedAt time.Time
	send            Sender
}

// takeState keeps what a device says of itself: a state report's buffering and round trip, the first
// that is not buffering making it one the group waits for, and one catching up caught up; a
// suspension, or its end. It returns whether the device stalled: it was ready and now buffers.
func (each *device) takeState(message Inbound) (hasStalled bool) {
	wasReady := each.hasBeenReady && each.isReady()
	switch message.Type {
	case MessageState:
		each.buffering = message.Buffering
		if !message.Buffering {
			each.hasBeenReady, each.isCatchingUp = true, false
		}
		if message.RTTMs > 0 {
			each.rttMs = message.RTTMs
		}
		each.cannotKeepUp = message.CannotKeepUp
		each.knowsStartLag = message.StartLagMs != nil
	case MessageSuspend:
		each.suspended = message.Reason
	case MessageResume:
		each.suspended = ""
	case MessageHello, MessagePing, MessageCommand, MessageRTC, MessageNote, MessageWelcome, MessagePong, MessageGroup,
		MessageRoster, MessageRejected, MessageEnded:
	}
	return wasReady && each.buffering
}

// isReady is whether the device plays along: not buffering, not suspended.
func (each *device) isReady() bool { return !each.buffering && each.suspended == "" }

// Opened is the host's session: its id, and a membership credential the host's devices say hello
// with.
type Opened struct {
	SessionID            int64
	MembershipCredential string
	// IsNew is whether the session was started now, rather than one the host already had.
	IsNew bool
}

// Open starts a session hosted by host, nothing playing, or gives back the one they already host,
// with a new credential for their devices, for a page that was reloaded. A host has one session.
func (hub *Hub) Open(host Person) (Opened, error) {
	credential, err := randbytes.Base64URLSecret(hub.random, secretBytes)
	if err != nil {
		return Opened{}, fmt.Errorf("make a membership credential: %w", err)
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	for _, each := range hub.sessions {
		if each.hostUserID != host.UserID {
			continue
		}
		for _, hostMember := range each.members {
			if hostMember.isHost {
				hostMember.addCredential(credential)
			}
		}
		return Opened{SessionID: each.id, MembershipCredential: credential}, nil
	}
	hub.lastID++
	opened := &session{
		id: hub.lastID, hostUserID: host.UserID, links: map[int64]*link{}, members: map[int64]*member{},
		group: group{state: GroupIdle, rate: 1, at: hub.clock.Now()}, logger: hub.logger,
		syncWait: startingSyncWait, lastRights: everyRight(),
	}
	hub.lastID++
	hostPerson := host
	opened.members[hub.lastID] = &member{
		id: hub.lastID, name: host.Name, isHost: true, person: &hostPerson, devices: map[int64]*device{},
	}
	opened.members[hub.lastID].addCredential(credential)
	hub.sessions[opened.id] = opened
	return Opened{SessionID: opened.id, MembershipCredential: credential, IsNew: true}, nil
}

// LinkMade is a new link: its id, and its secret, which goes after # in the address (D243 §3).
type LinkMade struct {
	LinkID int64
	Secret string
}

// AddLink makes a link for one person to join by, once, with the host's note of who it is for,
// which only the host sees (D246 §3), and what they may do to playback (D271 §4).
func (hub *Hub) AddLink(sessionID, hostUserID int64, hostNote string, rights Rights) (LinkMade, error) {
	if hostNote == "" {
		return LinkMade{}, ErrNoNote
	}
	secret, err := randbytes.Base64URLSecret(hub.random, secretBytes)
	if err != nil {
		return LinkMade{}, fmt.Errorf("make a link: %w", err)
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hosted, err := hub.hostedSession(sessionID, hostUserID)
	if err != nil {
		return LinkMade{}, err
	}
	hub.lastID++
	hosted.links[hub.lastID] = &link{
		id: hub.lastID, secretHash: sha256.Sum256([]byte(secret)), secret: secret, hostNote: hostNote,
		rights: rights,
	}
	hosted.lastRights = rights
	return LinkMade{LinkID: hub.lastID, Secret: secret}, nil
}

// RemoveLink withdraws a link: it admits no one from now on, and a member who joined by it stays.
func (hub *Hub) RemoveLink(sessionID, hostUserID, linkID int64) error {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hosted, err := hub.hostedSession(sessionID, hostUserID)
	if err != nil {
		return err
	}
	if _, found := hosted.links[linkID]; !found {
		return ErrNoLink
	}
	delete(hosted.links, linkID)
	return nil
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
	details := Details{
		SessionID: hosted.id, Group: *hosted.groupView(hub.clock.Now()), NextRights: hosted.lastRights,
	}
	for _, each := range hosted.links {
		details.Links = append(details.Links, LinkView{
			ID: each.id, HostNote: each.hostNote, Secret: each.secret, UsedBy: each.usedBy, Rights: each.rights,
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
}

// Join admits the person a link was made for, once, as a member named name, the name the others
// see (D246 §3); signed in, as person, whose devices have the membership back without the link
// (D267 §2). Not signed in, person is nil.
func (hub *Hub) Join(secret, name string, person *Person) (Joined, error) {
	credential, err := randbytes.Base64URLSecret(hub.random, secretBytes)
	if err != nil {
		return Joined{}, fmt.Errorf("make a membership credential: %w", err)
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	joined, used := hub.sessionByLink(secret)
	if used == nil || used.usedBy != 0 {
		return Joined{}, ErrNoSession
	}
	hub.lastID++
	used.usedBy = hub.lastID
	joined.members[hub.lastID] = &member{
		id: hub.lastID, name: name, hostNote: used.hostNote, rights: used.rights, person: person,
		devices: map[int64]*device{},
	}
	joined.members[hub.lastID].addCredential(credential)
	joined.sendRoster()
	return Joined{SessionID: joined.id, MemberID: hub.lastID, MembershipCredential: credential}, nil
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
	for _, each := range ejected.devices {
		each.send(Outbound{V: ProtocolVersion, Type: MessageEnded, Reason: "ejected"})
	}
	delete(hosted.members, memberID)
	hosted.settle(hub.clock.Now())
	hosted.sendRoster()
	return nil
}

// SendAwayNewest sends away the member who joined most recently of those with a player, in any
// session, never a host: each of their devices is told the session ended for them with reason, and
// their membership is kept, so they may come back at once (D286 §5: the server's upload cannot carry
// every player at the lowest qualities). It says whom, and whether there was anyone.
func (hub *Hub) SendAwayNewest(reason string) (sessionID, memberID int64, isSent bool) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	var newest *member
	for _, each := range hub.sessions {
		for _, candidate := range each.members {
			if !candidate.isHost && candidate.hasPlayer() && (newest == nil || candidate.id > newest.id) {
				newest, sessionID = candidate, each.id
			}
		}
	}
	if newest == nil {
		return 0, 0, false
	}
	for _, each := range newest.devices {
		each.send(Outbound{V: ProtocolVersion, Type: MessageEnded, Reason: reason})
	}
	return sessionID, newest.id, true
}

// hasPlayer is whether any of the member's devices plays the film.
func (each *member) hasPlayer() bool {
	for _, attached := range each.devices {
		if attached.role.plays() {
			return true
		}
	}
	return false
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
	for _, each := range leaving.devices {
		each.send(Outbound{V: ProtocolVersion, Type: MessageEnded, Reason: "left"})
	}
	delete(left.members, leaving.id)
	left.settle(hub.clock.Now())
	left.sendRoster()
	return nil
}

// End ends the session: every device is told, and every credential and link admits nothing more.
func (hub *Hub) End(sessionID, hostUserID int64) error {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hosted, err := hub.hostedSession(sessionID, hostUserID)
	if err != nil {
		return err
	}
	hosted.broadcast(Outbound{V: ProtocolVersion, Type: MessageEnded, Reason: "hostEnded"})
	delete(hub.sessions, sessionID)
	return nil
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
// tuning. A device of a person signed in makes the membership theirs (D243 §2).
func (hub *Hub) Attach(
	credential string, role DeviceRole, canSetRate bool, signedIn *Person, tuning Tuning, send Sender,
) (Attachment, error) {
	if !role.plays() && role != RoleVoice {
		return Attachment{}, fmt.Errorf("role %q: want playback, voice or both", role)
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	attached, attachedMember := hub.memberByCredential(credential)
	if attachedMember == nil {
		return Attachment{}, ErrNoSession
	}
	if signedIn != nil && attachedMember.person == nil {
		person := *signedIn
		attachedMember.person = &person
	}
	if tuning.SyncWaitMs > 0 {
		attached.syncWait = time.Duration(tuning.SyncWaitMs) * time.Millisecond
	}
	attached.startLead = time.Duration(tuning.StartLeadMs) * time.Millisecond
	attached.runUp = time.Duration(tuning.RunUpMs) * time.Millisecond
	hub.lastID++
	attachedMember.devices[hub.lastID] = &device{id: hub.lastID, role: role, canSetRate: canSetRate, send: send}
	send(Outbound{
		V: ProtocolVersion, Type: MessageWelcome, Seq: attached.group.seq, SessionID: attached.id,
		MemberID: attachedMember.id, DeviceID: hub.lastID, IsHost: attachedMember.isHost,
		Rights: new(attachedMember.rightsHeld()), Group: attached.groupView(hub.clock.Now()), Roster: attached.roster(),
		Tuning: &tuning,
	})
	attached.sendRoster()
	return Attachment{
		SessionID: attached.id, MemberID: attachedMember.id, DeviceID: hub.lastID, Person: attachedMember.person,
	}, nil
}

// Detach removes a device whose connection ended. The group never stops for it (D250 §2): its
// member stays, and it catches up alone once it is back. A sync point no longer waits for it. A
// group it was the last player to play along with waits until a player is ready (dropped).
func (hub *Hub) Detach(attachment Attachment) {
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
	detached.dropped(hub.clock.Now(), left, gone)
	detached.settle(hub.clock.Now())
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
	defer handled.endWait(now) // a ready player reports each second while the group waits for others
	switch message.Type {
	case MessagePing:
		// The pong says where the group is as it answers, so a follower can place the group by the
		// pong whose round trip it measured shortest, not by a message whose wait it cannot know (D264).
		from.send(Outbound{
			V: ProtocolVersion, Type: MessagePong, TMono: message.TMono, Seq: handled.group.seq,
			Group: handled.groupView(now),
		})
	case MessageState, MessageSuspend, MessageResume:
		handled.takeState(ctx, now, sender, from, message)
	case MessageCommand:
		handled.command(now, sender, from, message)
	case MessageRTC:
		handled.relay(from, message)
	case MessageNote:
		handled.logNote(ctx, sender, from, message.Note)
	case MessageHello, MessageWelcome, MessagePong, MessageGroup, MessageRoster, MessageRejected, MessageEnded:
		from.send(Outbound{V: ProtocolVersion, Type: MessageRejected, Code: codeInvalid})
	}
}

// takeState takes what a device says of itself: a stall it tells of, the group settled on its
// readiness, and everyone told. Its first word that even its lowest quality is beyond its link is
// logged.
func (current *session) takeState(ctx context.Context, now time.Time, who *member, from *device, message Inbound) {
	couldKeepUp := from.cannotKeepUp == nil
	current.logPlayer(ctx, now, who, from, message)
	if from.takeState(message) {
		current.stalled(now, who, from)
	}
	if couldKeepUp && from.cannotKeepUp != nil {
		current.logger.InfoContext(ctx, "watch together: a device cannot keep up even at its lowest quality",
			append(current.deviceAttributes(who, from),
				"linkKbps", from.cannotKeepUp.LinkKbps, "lowestKbps", from.cannotKeepUp.LowestKbps)...)
	}
	current.settle(now)
	current.sendRoster()
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
			view.Devices = append(view.Devices, DeviceView{
				ID: attached.id, Role: attached.role, IsBuffering: attached.buffering,
				Suspended: attached.suspended, RTTMs: attached.rttMs, CannotKeepUp: attached.cannotKeepUp,
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
	current.broadcast(Outbound{V: ProtocolVersion, Type: MessageRoster, Roster: current.roster()})
}

// broadcast sends message to every device of every member.
func (current *session) broadcast(message Outbound) {
	for _, each := range current.members {
		for _, attached := range each.devices {
			attached.send(message)
		}
	}
}

// sessionByLink is the session and link a link's secret names, or nils.
func (hub *Hub) sessionByLink(secret string) (*session, *link) {
	hash := sha256.Sum256([]byte(secret))
	for _, each := range hub.sessions {
		for _, candidate := range each.links {
			if subtle.ConstantTimeCompare(candidate.secretHash[:], hash[:]) == 1 {
				return each, candidate
			}
		}
	}
	return nil, nil
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
	if !found {
		return nil, ErrNoSession
	}
	if hosted.hostUserID != hostUserID {
		return nil, ErrNotHost
	}
	return hosted, nil
}
