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

// secretBytes is a join link's or a membership credential's length before encoding: 256 random
// bits, which no one guesses.
const secretBytes = 32

// DefaultCap is a new session's cap on people, which its host may raise: two, the case the design
// is best at (D243 §9, D69).
const DefaultCap = 2

// The codes a refused command or join is told (cut-a §5.2).
const (
	codeForbidden = "forbidden"
	codeStale     = "stale"
	codeInvalid   = "invalid_request"
	codeNotFound  = "not_found"
)

var (
	// ErrNoSession is a session, link or credential that is not, or no longer, a live one.
	ErrNoSession = errors.New("no such Watch Together session, or its link has changed")
	// ErrFull is a join refused because the session holds as many people as its cap.
	ErrFull = errors.New("the session is full")
	// ErrNotHost is something only the host may do.
	ErrNotHost = errors.New("only the session's host may do that")
	// ErrNoMember is a member the session does not hold.
	ErrNoMember = errors.New("no such member")
)

// Transport is who may play, pause and seek (D121): the whole group, or the host alone.
type Transport string

// The transport abilities.
const (
	TransportGroup Transport = "group"
	TransportHost  Transport = "host"
)

// Abilities are what the host granted at the session's start (D121).
type Abilities struct {
	Transport Transport `json:"transport"`
}

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
	lastID   int64              // the last id given a session, member or device; guarded by mutex
}

// NewHub returns an empty hub, which asks mayPlay whether a host may play what they change the
// session to.
func NewHub(clockSource clock.Clock, random io.Reader, mayPlay MayPlay) *Hub {
	return &Hub{clock: clockSource, random: random, mayPlay: mayPlay, sessions: map[int64]*session{}}
}

// session is one Watch Together session (D122): what plays is state on it.
type session struct {
	id         int64
	hostUserID int64
	joinHash   [sha256.Size]byte
	peopleCap  int
	abilities  Abilities
	group      group
	members    map[int64]*member
	// refusedJoins counts the joins refused at the cap since the start: several in a row is the
	// earliest sign a link leaked (watch-together.md).
	refusedJoins int
}

// group is the group's state: Position is where it was at At, moving at Rate while it plays.
type group struct {
	seq      int64
	state    GroupState
	position time.Duration
	at       time.Time
	rate     float64
	fileID   int64
	event    string
	byMember int64
	lastSeek int64 // the seq of the last seek, for a stale one's conflict
}

// member is a person in the session, with their devices (D120).
type member struct {
	id             int64
	name           string
	isHost         bool
	person         *Person
	credentialHash [sha256.Size]byte
	devices        map[int64]*device
}

// device is one connected client of a member.
type device struct {
	id         int64
	role       DeviceRole
	canSetRate bool
	buffering  bool
	suspended  SuspendReason
	rttMs      int64
	send       Sender
}

// isReady is whether the device plays along: not buffering, not suspended.
func (each *device) isReady() bool { return !each.buffering && each.suspended == "" }

// Created is a new session: its id, the join link's secret, and the host's membership credential.
type Created struct {
	SessionID            int64
	JoinSecret           string
	MembershipCredential string
}

// Create starts a session, hosted by host, playing fileID, with abilities and a cap on people.
func (hub *Hub) Create(host Person, fileID int64, abilities Abilities, peopleCap int) (Created, error) {
	if abilities.Transport != TransportGroup && abilities.Transport != TransportHost {
		return Created{}, fmt.Errorf("transport %q: want group or host", abilities.Transport)
	}
	if peopleCap < 1 {
		peopleCap = DefaultCap
	}
	joinSecret, err := randbytes.Base64URLSecret(hub.random, secretBytes)
	if err != nil {
		return Created{}, fmt.Errorf("make a join link: %w", err)
	}
	credential, err := randbytes.Base64URLSecret(hub.random, secretBytes)
	if err != nil {
		return Created{}, fmt.Errorf("make a membership credential: %w", err)
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hub.lastID++
	created := &session{
		id: hub.lastID, hostUserID: host.UserID, joinHash: sha256.Sum256([]byte(joinSecret)), peopleCap: peopleCap,
		abilities: abilities, members: map[int64]*member{},
		group: group{state: GroupPaused, rate: 1, fileID: fileID, at: hub.clock.Now()},
	}
	hub.lastID++
	hostPerson := host
	created.members[hub.lastID] = &member{
		id: hub.lastID, name: host.Name, isHost: true, person: &hostPerson,
		credentialHash: sha256.Sum256([]byte(credential)), devices: map[int64]*device{},
	}
	hub.sessions[created.id] = created
	return Created{SessionID: created.id, JoinSecret: joinSecret, MembershipCredential: credential}, nil
}

// Joined is a membership: its session, and the credential its devices say hello with.
type Joined struct {
	SessionID            int64
	MemberID             int64
	MembershipCredential string
}

// Join admits a person by the join link's secret, as a member named name (D119): the link admits,
// the membership persists. A session at its cap refuses, and counts it for its host.
func (hub *Hub) Join(secret, name string) (Joined, error) {
	credential, err := randbytes.Base64URLSecret(hub.random, secretBytes)
	if err != nil {
		return Joined{}, fmt.Errorf("make a membership credential: %w", err)
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	joined := hub.sessionByJoinSecret(secret)
	if joined == nil {
		return Joined{}, ErrNoSession
	}
	if len(joined.members) >= joined.peopleCap {
		joined.refusedJoins++
		joined.sendRoster()
		return Joined{}, ErrFull
	}
	hub.lastID++
	joined.members[hub.lastID] = &member{
		id: hub.lastID, name: name, credentialHash: sha256.Sum256([]byte(credential)), devices: map[int64]*device{},
	}
	joined.sendRoster()
	return Joined{SessionID: joined.id, MemberID: hub.lastID, MembershipCredential: credential}, nil
}

// Rotate gives the session a new join link: the old admits no one new, and everyone already in is
// undisturbed (D119).
func (hub *Hub) Rotate(sessionID, hostUserID int64) (string, error) {
	secret, err := randbytes.Base64URLSecret(hub.random, secretBytes)
	if err != nil {
		return "", fmt.Errorf("make a join link: %w", err)
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hosted, err := hub.hostedSession(sessionID, hostUserID)
	if err != nil {
		return "", err
	}
	hosted.joinHash = sha256.Sum256([]byte(secret))
	return secret, nil
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
	hosted.sendRoster()
	return nil
}

// SetCap changes the session's cap on people (D119).
func (hub *Hub) SetCap(sessionID, hostUserID int64, peopleCap int) error {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hosted, err := hub.hostedSession(sessionID, hostUserID)
	if err != nil {
		return err
	}
	hosted.peopleCap = max(peopleCap, 1)
	hosted.sendRoster()
	return nil
}

// End ends the session: every device is told, and every credential admits nothing more.
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

// Attachment is a device attached to its member: what it is, and how it leaves.
type Attachment struct {
	SessionID int64
	MemberID  int64
	DeviceID  int64
	// Person is the member's person, when any of their devices said hello signed in (D243 §2).
	Person *Person
}

// Attach attaches a device to the member its credential names, with a role, its messages going to
// send, and welcomes it: the session, its member, the abilities, the group and the roster. A device
// of a person signed in makes the membership theirs (D243 §2).
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
		MemberID: attachedMember.id, DeviceID: hub.lastID, Abilities: &attached.abilities,
		Group: attached.groupView(hub.clock.Now()), Roster: attached.roster(),
	})
	attached.sendRoster()
	if role.plays() {
		// A device that joins a playing group waits for it until it is ready.
		attached.settle(hub.clock.Now())
	}
	return Attachment{
		SessionID: attached.id, MemberID: attachedMember.id, DeviceID: hub.lastID, Person: attachedMember.person,
	}, nil
}

// Detach removes a device whose connection ended. A playing group waits for it: a drop is a state,
// "reconnecting", not an ending (watch-together.md, What each person sees).
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
	if gone.role.plays() && detached.group.state == GroupPlaying {
		detached.change(now, GroupWaiting, "left", left.id)
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
	switch message.Type {
	case MessagePing:
		from.send(Outbound{V: ProtocolVersion, Type: MessagePong, TMono: message.TMono})
	case MessageState:
		from.buffering = message.Buffering
		if message.RTTMs > 0 {
			from.rttMs = message.RTTMs
		}
		handled.settle(now)
		handled.sendRoster()
	case MessageSuspend:
		from.suspended = message.Reason
		handled.settle(now)
		handled.sendRoster()
	case MessageResume:
		from.suspended = ""
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

// command applies a command in arrival order (cut-a §5.1): one the sender may not give is refused;
// a seek based on a group another seek has changed since is the stale one and is refused.
func (current *session) command(now time.Time, sender *member, from *device, message Inbound) {
	refuse := func(code string) {
		from.send(Outbound{V: ProtocolVersion, Type: MessageRejected, BasedOnSeq: message.BasedOnSeq, Code: code})
	}
	if message.Command == CommandChangeItem {
		if code := current.changeItem(now, sender, message.FileID); code != "" {
			refuse(code)
		}
		return
	}
	if current.abilities.Transport == TransportHost && !sender.isHost {
		refuse(codeForbidden)
		return
	}
	switch message.Command {
	case CommandPlay:
		current.change(now, GroupWaiting, "played", sender.id)
		current.settle(now)
	case CommandPause:
		current.change(now, GroupPaused, "paused", sender.id)
	case CommandSeek:
		if message.BasedOnSeq < current.group.lastSeek {
			refuse(codeStale)
			return
		}
		current.seek(now, sender, time.Duration(message.PositionMs)*time.Millisecond)
	case CommandRate:
		if message.Rate <= 0 || !current.everyPlayerSetsRate() {
			refuse(codeInvalid)
			return
		}
		current.group.position, current.group.at = current.positionAt(now), now
		current.group.rate = message.Rate
		current.change(now, current.group.state, "changed the rate", sender.id)
	case CommandChangeItem:
	default:
		refuse(codeInvalid)
	}
}

// changeItem makes the group play fileID from its start, as the host asked; a file the host may not
// play was given as 0 (Handle), and is refused as not found. It answers the refusal's code, or "".
func (current *session) changeItem(now time.Time, sender *member, fileID int64) string {
	if !sender.isHost {
		return codeForbidden
	}
	if fileID == 0 {
		return codeNotFound
	}
	current.group.fileID, current.group.position = fileID, 0
	current.change(now, GroupWaiting, "changed what plays", sender.id)
	current.settle(now)
	return ""
}

// seek moves the group to position; a playing group waits there until every player is ready.
func (current *session) seek(now time.Time, sender *member, position time.Duration) {
	state := current.group.state
	if state == GroupPlaying {
		state = GroupWaiting
	}
	from := current.positionAt(now)
	current.group.position, current.group.at = position, now
	current.change(now, state, fmt.Sprintf("seeked from %d", from.Milliseconds()), sender.id)
	current.group.lastSeek = current.group.seq
	current.settle(now)
}

// settle starts a waiting group playing once every device that plays is ready, or stops a playing
// one that has one not ready; a group with no device to play waits.
func (current *session) settle(now time.Time) {
	allReady, anyPlayer := true, false
	var waitingFor int64
	for _, each := range current.members {
		for _, attached := range each.devices {
			if !attached.role.plays() {
				continue
			}
			anyPlayer = true
			if !attached.isReady() {
				allReady, waitingFor = false, each.id
			}
		}
	}
	switch {
	case current.group.state == GroupWaiting && allReady && anyPlayer:
		current.change(now, GroupPlaying, "everyone is ready", 0)
	case current.group.state == GroupPlaying && !allReady:
		current.change(now, GroupWaiting, "buffering", waitingFor)
	}
}

// change moves the group to state, naming the event and who caused it, and tells every device.
func (current *session) change(now time.Time, state GroupState, event string, byMember int64) {
	current.group.position, current.group.at = current.positionAt(now), now
	current.group.state = state
	current.group.event, current.group.byMember = event, byMember
	current.group.seq++
	current.broadcast(Outbound{
		V: ProtocolVersion, Type: MessageGroup, Seq: current.group.seq, Group: current.groupView(now),
	})
}

// positionAt is where the group is at now: moving at its rate while it plays.
func (current *session) positionAt(now time.Time) time.Duration {
	if current.group.state != GroupPlaying {
		return current.group.position
	}
	elapsed := now.Sub(current.group.at)
	return current.group.position + time.Duration(float64(elapsed)*current.group.rate)
}

// groupView is the group as a client is told it.
func (current *session) groupView(now time.Time) *GroupView {
	return &GroupView{
		State: current.group.state, PositionMs: current.positionAt(now).Milliseconds(), Rate: current.group.rate,
		FileID: current.group.fileID, Event: current.group.event, ByMemberID: current.group.byMember,
	}
}

// everyPlayerSetsRate is whether every device that plays can set its rate: a group's rate stays
// 1.0 unless every one can honour it (watch-together.md, Sync).
func (current *session) everyPlayerSetsRate() bool {
	for _, each := range current.members {
		for _, attached := range each.devices {
			if attached.role.plays() && !attached.canSetRate {
				return false
			}
		}
	}
	return true
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

// roster is every member and their devices.
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

// sendRoster tells every device the roster, and the host how many joins the cap refused.
func (current *session) sendRoster() {
	current.broadcast(Outbound{
		V: ProtocolVersion, Type: MessageRoster, Roster: current.roster(), RefusedJoins: current.refusedJoins,
	})
}

// broadcast sends message to every device of every member.
func (current *session) broadcast(message Outbound) {
	for _, each := range current.members {
		for _, attached := range each.devices {
			attached.send(message)
		}
	}
}

// sessionByJoinSecret is the session whose join link is secret, or nil.
func (hub *Hub) sessionByJoinSecret(secret string) *session {
	hash := sha256.Sum256([]byte(secret))
	for _, each := range hub.sessions {
		if subtle.ConstantTimeCompare(each.joinHash[:], hash[:]) == 1 {
			return each
		}
	}
	return nil
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
