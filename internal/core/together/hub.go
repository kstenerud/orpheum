// SPDX-License-Identifier: GPL-3.0-or-later

package together

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
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

// Hub holds the live sessions, in memory: they end with the process (D119).
type Hub struct {
	clock   clock.Clock
	random  io.Reader
	mayPlay MayPlay

	mutex    sync.Mutex
	sessions map[int64]*session // guarded by mutex
	lastID   int64              // the last id given a session, link, member or device; guarded by mutex
}

// NewHub returns an empty hub, which asks mayPlay whether a host may play what they put on.
func NewHub(clockSource clock.Clock, random io.Reader, mayPlay MayPlay) *Hub {
	return &Hub{clock: clockSource, random: random, mayPlay: mayPlay, sessions: map[int64]*session{}}
}

// session is one Watch Together session (D122): what plays is state on it, the host's to change.
type session struct {
	id         int64
	hostUserID int64
	group      group
	// lastStallWait is when the group last waited for a player that stalled; zero for never.
	lastStallWait time.Time
	links         map[int64]*link
	members       map[int64]*member
}

// link is one person's way in (D246 §3): single use, with the host's note of who it is for.
type link struct {
	id         int64
	secretHash [sha256.Size]byte
	adminNote  string
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
	// waitUntil is when a Waiting group stops waiting for players not yet ready.
	waitUntil time.Time
}

// member is a person in the session, with their devices (D120).
type member struct {
	id     int64
	name   string
	isHost bool
	// adminNote is the host's note of who the member is, from their link; only the host sees it.
	adminNote      string
	person         *Person
	credentialHash [sha256.Size]byte
	devices        map[int64]*device
	// lastStall is when one of the member's players last stalled while the group played; zero for
	// never.
	lastStall time.Time
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
	send         Sender
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
	case MessageSuspend:
		each.suspended = message.Reason
	case MessageResume:
		each.suspended = ""
	case MessageHello, MessagePing, MessageCommand, MessageRTC, MessageWelcome, MessagePong, MessageGroup,
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
				hostMember.credentialHash = sha256.Sum256([]byte(credential))
			}
		}
		return Opened{SessionID: each.id, MembershipCredential: credential}, nil
	}
	hub.lastID++
	opened := &session{
		id: hub.lastID, hostUserID: host.UserID, links: map[int64]*link{}, members: map[int64]*member{},
		group: group{state: GroupIdle, rate: 1, at: hub.clock.Now()},
	}
	hub.lastID++
	hostPerson := host
	opened.members[hub.lastID] = &member{
		id: hub.lastID, name: host.Name, isHost: true, person: &hostPerson,
		credentialHash: sha256.Sum256([]byte(credential)), devices: map[int64]*device{},
	}
	hub.sessions[opened.id] = opened
	return Opened{SessionID: opened.id, MembershipCredential: credential, IsNew: true}, nil
}

// LinkMade is a new link: its id, and its secret, which goes after # in the address (D243 §3).
type LinkMade struct {
	LinkID int64
	Secret string
}

// AddLink makes a link for one person to join by, once, with the host's note of who it is for,
// which only the host sees (D246 §3).
func (hub *Hub) AddLink(sessionID, hostUserID int64, adminNote string) (LinkMade, error) {
	if adminNote == "" {
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
	hosted.links[hub.lastID] = &link{id: hub.lastID, secretHash: sha256.Sum256([]byte(secret)), adminNote: adminNote}
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
	ID        int64
	AdminNote string
	// UsedBy is the member who joined by it, or 0 while it waits.
	UsedBy int64
}

// MemberDetail is a member as their host sees them: with the note of the link they joined by.
type MemberDetail struct {
	ID        int64
	Name      string
	AdminNote string
	IsHost    bool
}

// Details is a session as its host sees it: its links and members, with the host's notes, and
// what plays.
type Details struct {
	SessionID int64
	Links     []LinkView
	Members   []MemberDetail
	Group     GroupView
}

// DetailsOf is the session sessionID, for its host.
func (hub *Hub) DetailsOf(sessionID, hostUserID int64) (Details, error) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hosted, err := hub.hostedSession(sessionID, hostUserID)
	if err != nil {
		return Details{}, err
	}
	details := Details{SessionID: hosted.id, Group: *hosted.groupView(hub.clock.Now())}
	for _, each := range hosted.links {
		details.Links = append(details.Links, LinkView{ID: each.id, AdminNote: each.adminNote, UsedBy: each.usedBy})
	}
	for _, each := range hosted.members {
		details.Members = append(details.Members, MemberDetail{
			ID: each.id, Name: each.name, AdminNote: each.adminNote, IsHost: each.isHost,
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
// see (D246 §3).
func (hub *Hub) Join(secret, name string) (Joined, error) {
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
		id: hub.lastID, name: name, adminNote: used.adminNote,
		credentialHash: sha256.Sum256([]byte(credential)), devices: map[int64]*device{},
	}
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
// send, and welcomes it: the session, its member, the group and the roster. A device of a person
// signed in makes the membership theirs (D243 §2).
func (hub *Hub) Attach(credential string, role DeviceRole, canSetRate bool, signedIn *Person, send Sender) (
	Attachment, error,
) {
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
	hub.lastID++
	attachedMember.devices[hub.lastID] = &device{id: hub.lastID, role: role, canSetRate: canSetRate, send: send}
	send(Outbound{
		V: ProtocolVersion, Type: MessageWelcome, Seq: attached.group.seq, SessionID: attached.id,
		MemberID: attachedMember.id, DeviceID: hub.lastID, IsHost: attachedMember.isHost,
		Group: attached.groupView(hub.clock.Now()), Roster: attached.roster(),
	})
	attached.sendRoster()
	return Attachment{
		SessionID: attached.id, MemberID: attachedMember.id, DeviceID: hub.lastID, Person: attachedMember.person,
	}, nil
}

// Detach removes a device whose connection ended. A playing group waits for it if it was playing
// along: a drop is a state, "reconnecting", not an ending (watch-together.md, What each person sees).
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
	now := hub.clock.Now()
	if gone.role.plays() && gone.hasBeenReady && detached.group.state == GroupPlaying {
		detached.change(now, GroupWaiting, EventLeft, left.id)
	}
	detached.sendRoster()
}

// Handle takes one message from an attached device. A change of what plays is checked against
// what the host may play before the hub is held, since that reads the store.
func (hub *Hub) Handle(ctx context.Context, attachment Attachment, message Inbound) {
	if message.Type == MessageCommand && message.Command == CommandChangeItem &&
		!hub.mayPlay(ctx, hub.hostOf(attachment.SessionID), message.FileID) {
		message.FileID = 0 // refused below, as one the host may not play
	}
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
		from.send(Outbound{V: ProtocolVersion, Type: MessagePong, TMono: message.TMono})
	case MessageState, MessageSuspend, MessageResume:
		if from.takeState(message) {
			handled.stalled(now, sender, from)
		}
		handled.settle(now)
		handled.sendRoster()
	case MessageCommand:
		handled.command(now, sender, from, message)
	case MessageRTC:
		handled.relay(from, message)
	case MessageHello, MessageWelcome, MessagePong, MessageGroup, MessageRoster, MessageRejected, MessageEnded:
		from.send(Outbound{V: ProtocolVersion, Type: MessageRejected, Code: codeInvalid})
	}
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
				Suspended: attached.suspended, RTTMs: attached.rttMs,
			})
		}
		views = append(views, view)
	}
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
			if subtle.ConstantTimeCompare(candidate.credentialHash[:], hash[:]) == 1 {
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
