// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"context"
	"net/http"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
)

// realmAnswerWait is how long a realm sign-in that fails takes, whatever part of it was wrong, long
// enough to cover a core's round trip (D297 §2, D312 §17).
const realmAnswerWait = 5 * time.Second

// realmAskWait is how long a realm sign-in waits for the core's answer: no longer than a failure's
// answer waits, so a core slow to refuse is answered at the same moment as an unknown realm. A wait
// of 10 seconds beside an answer at 1 told a known realm by a refusal later than the second.
const realmAskWait = realmAnswerWait

// realmSignIn finds the core by a person's realm sign-in: passed to the core, which checks it under
// their key, and answered, when it holds, as a link's locate is, with the core's identity key and its
// tag over it. Every way it fails answers the same not-found, no sooner than realmAnswerWait after
// it was asked (D297 §2, §3).
func (discovery *Discovery) realmSignIn(writer http.ResponseWriter, request *http.Request) {
	var asked discoveryproto.ClientRealmRequest
	if !read(writer, request, &asked) {
		return
	}
	ctx := request.Context()
	asking := discovery.parts.Now()
	answered, isTaken := discovery.askRealm(ctx, asked)
	if !isTaken {
		// Until realmAnswerWait after it was asked, so the core's refusal takes no longer than none.
		discovery.wait(ctx, realmAnswerWait-discovery.parts.Now().Sub(asking))
		refuse(writer, http.StatusNotFound, discoveryproto.RefusalNotFound, "no server is found by this")
		return
	}
	token, err := discovery.secret()
	if err != nil {
		discovery.fail(writer, request, err)
		return
	}
	now := discovery.parts.Now()
	discovery.mutex.Lock()
	discovery.pruneBootstraps(now)
	// The token's offers are passed on under the person, whose key the core proves them by.
	discovery.bootstraps[token] = bootstrap{locateHash: asked.Person, expires: now.Add(bootstrapLasts)}
	discovery.mutex.Unlock()
	paths, ok := discovery.pathsFor(writer, request)
	if !ok {
		return
	}
	answer(writer, discoveryproto.ClientRealmResponse{
		LocateResponse: discoveryproto.LocateResponse{PathsResponse: paths, BootstrapToken: token},
		PublicKey:      answered.PublicKey, Tag: answered.Tag,
	})
}

// askRealm is the core's answer to the sign-in, when its realm is the one the discovery's operator
// named and the core took it; the wait for the core is bounded by realmAskWait.
func (discovery *Discovery) askRealm(
	ctx context.Context, asked discoveryproto.ClientRealmRequest,
) (RealmAnswer, bool) {
	record, err := discovery.parts.Store.Read()
	realm, held := proof.RealmName(asked.Realm), proof.RealmName(discovery.parts.Realm)
	if err != nil || record.Core == nil || record.Core.State != discoveryproto.StateClaimed ||
		held == "" || realm != held || asked.Person == "" || asked.Ask == "" {
		return RealmAnswer{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, realmAskWait)
	defer cancel()
	answered, err := discovery.signal.Realm(ctx, asked.Person, asked.Ask)
	return answered, err == nil
}

// wait waits out duration, or until ctx ends, by the parts' Wait or a timer; none past or none at all
// waits nothing.
func (discovery *Discovery) wait(ctx context.Context, duration time.Duration) {
	if duration <= 0 {
		return
	}
	if discovery.parts.Wait != nil {
		discovery.parts.Wait(ctx, duration)
		return
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
