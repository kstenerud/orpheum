// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
)

// answerRealm is the core's answer to a realm sign-in the discovery passed on: its identity key and
// its tag over it under the person's key (proof.RealmTag), which only the core can make; or its
// refusal, whatever was wrong, which the discovery answers as not-found (D297 §2, §3).
func (service *Service) answerRealm(
	ctx context.Context, asked discoveryproto.SignalMessage,
) discoveryproto.SignalMessage {
	refusal := discoveryproto.SignalMessage{
		Type: discoveryproto.SignalRefusal, OfferID: asked.OfferID, Reason: "the realm sign-in does not hold",
	}
	if service.parts.RealmSignIn == nil || asked.Person == "" || asked.Ask == "" {
		return refusal
	}
	key, err := service.parts.RealmSignIn(ctx, asked.Person, asked.Ask)
	if err != nil {
		service.parts.Logger.InfoContext(ctx, "discovery: a realm sign-in was refused", "err", err)
		return refusal
	}
	coreID := service.Status().CoreID
	return discoveryproto.SignalMessage{
		Type: discoveryproto.SignalRealm, OfferID: asked.OfferID, PublicKey: service.parts.Key.Public(),
		Tag: proof.RealmTag(key, coreID, service.parts.Key.Public()),
	}
}
