// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// instancePingWait is how long a live instance has to answer a ping when another instance of its
// core opens a session: a crashed one never answers, and a busy NAS answers well within it (D293 §1).
const instancePingWait = 10 * time.Second

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

// session opens a session for the core that proves its key, and for the instance asking when no
// other instance of the core still answers on the signalling connection (D293 §1).
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
	if !discovery.isSigned(held.PublicKey, discoveryproto.PurposeSession, opening.Nonce, opening.Signature) {
		http.Error(writer, "the signature is not the core's over a nonce this discovery issued", http.StatusForbidden)
		return
	}
	if !discovery.admit(request.Context(), opening.InstanceID) {
		discovery.parts.Logger.WarnContext(request.Context(),
			"discovery: refused a second instance of the core while the first answers", "coreId", held.ID)
		refuse(writer, http.StatusConflict, discoveryproto.RefusalAnotherInstance,
			"another copy of this server is connected to the discovery and still answers")
		return
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
	answer(writer, discoveryproto.SessionResponse{Token: token, ExpiresAt: expires.UnixMilli()})
}

// admit makes instanceID the core's live instance, unless another is live and still answers a ping
// on its signalling connection: a crashed or restarted one does not, and is replaced at once, its
// connection closed and its tokens dropped; one with no connection is taken as gone. A real copy
// keeps the place it holds, so two copies never take turns, and a stolen key cannot push a serving
// core off (D293 §1).
func (discovery *Discovery) admit(ctx context.Context, instanceID string) bool {
	discovery.mutex.Lock()
	live := discovery.instance
	discovery.mutex.Unlock()
	if live != "" && live != instanceID {
		if discovery.signal.answers(ctx, live, discovery.pingWait) {
			return false
		}
		discovery.signal.dropSilent(live)
		discovery.parts.Logger.InfoContext(ctx,
			"discovery: a new instance of the core took the place of one that did not answer")
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
	return true
}

// forgetSessions drops every session and closes the signalling connection: the record's core changed,
// or left.
func (discovery *Discovery) forgetSessions(reason string) {
	discovery.mutex.Lock()
	discovery.tokens, discovery.instance = map[string]session{}, ""
	discovery.mutex.Unlock()
	discovery.signal.closeAll(reason)
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
