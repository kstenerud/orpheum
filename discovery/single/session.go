// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// instancePingWait is how long a live instance has to answer a ping when another instance of its
// core opens a session: a crashed one never answers, and a busy NAS answers well within it (D293 §1).
const instancePingWait = 10 * time.Second

// HolderWindowDefault is how long an installation other than the one that last held the core's realm
// waits for that one to come back while it is away: longer than the core's longest wait between asks,
// discovery_retry_most_seconds, 300 s by default (D437 §5; the owner, 2026-11-06).
const HolderWindowDefault = 6 * time.Minute

// session is what a session's token stands for: the core, by its id and key, and the instance that
// opened it, until it expires. A token is checked against the record's core on every call, so one a
// stranger opened while its key was the pending core is worth nothing once the core changes (the
// security audit's A3, K13).
type session struct {
	coreID     string
	publicKey  string
	instanceID string
	expires    time.Time
}

// session opens a session for the core that proves its key, for the instance asking: at once for the
// installation that last held the core's realm; for another, once the last holder's window has passed
// while it is away, and then when no other instance of the core still answers on the signalling
// connection (D293 §1, D437 §5).
func (discovery *Discovery) session(writer http.ResponseWriter, request *http.Request) {
	var opening discoveryproto.SessionRequest
	if !read(writer, request, &opening) {
		return
	}
	if opening.InstanceID == "" {
		http.Error(writer, "say which instance of the core is asking: instanceId", http.StatusBadRequest)
		return
	}
	record, err := discovery.parts.Store.Read()
	if err != nil {
		discovery.fail(writer, request, err)
		return
	}
	held := record.Core
	if held == nil || held.ID != opening.CoreID {
		refuse(writer, http.StatusForbidden, discoveryproto.RefusalEnrolAgain,
			"not this discovery's core: enrol again")
		return
	}
	if !discovery.isSigned(held.PublicKey, discoveryproto.PurposeSession, held.ID, opening.Nonce, opening.Signature) {
		http.Error(writer, "the signature is not the core's over a nonce this discovery issued", http.StatusForbidden)
		return
	}
	ctx := request.Context()
	if wait := discovery.holderAway(*held, opening.InstallationID); wait > 0 {
		discovery.parts.Logger.InfoContext(ctx,
			"discovery: asked a copy of the core to wait while the installation that last held it is away",
			"coreId", held.ID, "wait", wait)
		refuseFor(writer, discoveryproto.RefusalHolderAway,
			"the copy of this server that last held it is away: this one waits for it to come back first",
			wait)
		return
	}
	isHolder := opening.InstallationID != "" && opening.InstallationID == held.HolderInstallationID
	if !discovery.admit(ctx, opening.InstanceID, isHolder) {
		address := addressOf(request.RemoteAddr)
		discovery.parts.Logger.WarnContext(ctx,
			"discovery: refused a second instance of the core while the first answers", "coreId", held.ID,
			"from", address)
		if told := discovery.signal.CopyRefused(ctx, address, discovery.parts.Now()); told != nil {
			discovery.parts.Logger.WarnContext(ctx, "discovery: the serving core was not told of the copy",
				"err", told)
		}
		refuse(writer, http.StatusConflict, discoveryproto.RefusalAnotherInstance,
			"another copy of this server is connected to the discovery and still answers")
		return
	}
	if held.HolderInstallationID != opening.InstallationID {
		if err = discovery.keepHolder(opening.InstallationID); err != nil {
			discovery.fail(writer, request, err)
			return
		}
	}
	token, err := discovery.secret()
	if err != nil {
		discovery.fail(writer, request, err)
		return
	}
	expires := discovery.parts.Now().Add(sessionLasts)
	discovery.mutex.Lock()
	discovery.tokens[token] = session{
		coreID: held.ID, publicKey: string(held.PublicKey), instanceID: opening.InstanceID, expires: expires,
	}
	discovery.mutex.Unlock()
	answer(writer, discoveryproto.SessionResponse{Token: token, ExpiresIn: int64(sessionLasts / time.Second)})
}

// holderAway is how long an installation other than the one that last held the core's realm waits
// before it is heard: the rest of the window from when the last holder was last connected, or from
// the discovery's start when that was later, while the last holder holds no signalling connection;
// zero for the last holder itself, while it is connected, once the window has passed, and when no
// installation holds the realm (D437 §5). A request naming no installation is never the last holder.
func (discovery *Discovery) holderAway(held Core, installationID string) time.Duration {
	holder := held.HolderInstallationID
	if holder == "" || installationID == holder {
		return 0
	}
	isOnline, lastClosed := discovery.signal.Standing()
	if isOnline {
		return 0
	}
	discovery.mutex.Lock()
	connected := discovery.started
	if discovery.holderAdmitted.After(connected) {
		connected = discovery.holderAdmitted
	}
	discovery.mutex.Unlock()
	if lastClosed.After(connected) {
		connected = lastClosed
	}
	return max(0, connected.Add(discovery.holderWindow).Sub(discovery.parts.Now()))
}

// admit makes instanceID the core's live instance, unless another is live and still answers a ping
// on its signalling connection: a crashed or restarted one does not, and is replaced at once, its
// connection closed and its tokens dropped; one with no connection is taken as gone. A real copy
// keeps the place it holds, so two copies never take turns, and a stolen key cannot push a serving
// core off (D293 §1). The last holder's installation, isHolder, replaces its live instance without a
// ping, as a deploy's restart does (D437 §5).
func (discovery *Discovery) admit(ctx context.Context, instanceID string, isHolder bool) bool {
	discovery.mutex.Lock()
	live := discovery.instance
	discovery.mutex.Unlock()
	if live != "" && live != instanceID {
		if !isHolder && discovery.signal.Answers(ctx, live, discovery.pingWait) {
			return false
		}
		discovery.signal.DropSilent(live)
		discovery.parts.Logger.InfoContext(ctx,
			"discovery: a new instance of the core took the place of the one before", "lastHolder", isHolder)
	}
	discovery.mutex.Lock()
	defer discovery.mutex.Unlock()
	if discovery.instance != instanceID {
		for token, each := range discovery.tokens {
			if each.instanceID != instanceID {
				delete(discovery.tokens, token)
			}
		}
		discovery.instance = instanceID
	}
	discovery.holderAdmitted = discovery.parts.Now()
	return true
}

// keepHolder records installationID as the installation that last held the core's realm, kept across
// the discovery's restarts; empty, a core before D437, holds it as no installation (D437 §5).
func (discovery *Discovery) keepHolder(installationID string) error {
	return discovery.parts.Store.Update(func(record *Record) error {
		if record.Core != nil {
			record.Core.HolderInstallationID = installationID
		}
		return nil
	})
}

// addressOf is the IP address of a request's remote address, with no port.
func addressOf(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		return remoteAddress
	}
	return host
}

// forgetSessions drops every session and closes the signalling connection: the record's core changed,
// or left.
func (discovery *Discovery) forgetSessions(reason string) {
	discovery.mutex.Lock()
	discovery.tokens, discovery.instance = map[string]session{}, ""
	discovery.mutex.Unlock()
	discovery.signal.CloseAll(reason)
}

// signedIn is the record's core and the session of the token request carries, when the token is live
// and was opened for that core, by its id and key; otherwise it answers 401, and the core opens a new
// session, which says whether it must enrol again. Expired tokens are pruned on the way.
func (discovery *Discovery) signedIn(writer http.ResponseWriter, request *http.Request) (Core, session, bool) {
	token, isBearer := strings.CutPrefix(request.Header.Get("Authorization"), "Bearer ")
	now := discovery.parts.Now()
	discovery.mutex.Lock()
	for each, opened := range discovery.tokens {
		if !now.Before(opened.expires) {
			delete(discovery.tokens, each)
		}
	}
	opened, found := discovery.tokens[token]
	discovery.mutex.Unlock()
	if !isBearer || !found {
		http.Error(writer, "no session: open one first", http.StatusUnauthorized)
		return Core{}, session{}, false
	}
	record, err := discovery.parts.Store.Read()
	if err != nil {
		discovery.fail(writer, request, err)
		return Core{}, session{}, false
	}
	held := record.Core
	if held == nil || held.ID != opened.coreID || string(held.PublicKey) != opened.publicKey {
		http.Error(writer, "the session is not the core's this discovery holds: open one again",
			http.StatusUnauthorized)
		return Core{}, session{}, false
	}
	return *held, opened, true
}

// refuse answers a refusal the core acts on: its code, and what it says to the admin.
func refuse(writer http.ResponseWriter, status int, code, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	refusal := discoveryproto.Refusal{Code: code, Message: message}
	_ = json.NewEncoder(writer).Encode(refusal) //nolint:errcheck,errchkjson // the status says it
}

// refuseFor answers a 409 refusal that names how long the core waits before it asks again, in whole
// seconds, rounded up.
func refuseFor(writer http.ResponseWriter, code, message string, wait time.Duration) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusConflict)
	refusal := discoveryproto.Refusal{
		Code: code, Message: message, RetryAfterSeconds: int64((wait + time.Second - 1) / time.Second),
	}
	_ = json.NewEncoder(writer).Encode(refusal) //nolint:errcheck,errchkjson // the status says it
}
