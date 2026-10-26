// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"cmp"
	"slices"
	"time"

	discoverylink "github.com/kstenerud/orpheum/discovery/link"
)

// What a discovery finds the core by for Watch Together (discovery-core.md K7; the protocol's §3.1):
// each unused link's locate hash, and each membership device's locator. A used link locates nothing:
// its person's device comes back by the locator it was handed (A12). Neither hash gives back what it
// hashes, and ending a session, a membership or a link drops its own from the next look.

// locateDay is the unit of an unused link's life at the discovery. A link lives with its session,
// which has no end the core knows of in advance, so the core sends each a life to the end of the day
// after today and sends the full set again when the day turns: a discovery forgets the links of a core
// that went away within two days, and the core sends its set once a day for it [I].
const locateDay = 24 * time.Hour

// LiveLink is an unused link a discovery may find the core by: its secret's hash
// (link.LocateHash), and when the discovery forgets it unless told again.
type LiveLink struct {
	LocateHash string
	ExpiresAt  time.Time
}

// LiveLinks are every unused link's locate hash, in the hashes' order so that an unchanged set
// compares equal.
func (hub *Hub) LiveLinks() []LiveLink {
	now := hub.clock.Now()
	expiresAt := now.Truncate(locateDay).Add(2 * locateDay)
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	live := []LiveLink{}
	for _, each := range hub.sessions {
		for _, candidate := range each.links {
			if candidate.waits(now) {
				live = append(live, LiveLink{LocateHash: candidate.locateHash, ExpiresAt: expiresAt})
			}
		}
	}
	slices.SortFunc(live, func(first, second LiveLink) int { return cmp.Compare(first.LocateHash, second.LocateHash) })
	return live
}

// LiveLocators are every membership device's locator, as their hashes (link.LocatorHash), sorted.
func (hub *Hub) LiveLocators() []string {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hashes := []string{}
	for _, each := range hub.sessions {
		for _, candidate := range each.members {
			for _, held := range candidate.credentials {
				if held.locatorHash != "" {
					hashes = append(hashes, held.locatorHash)
				}
			}
		}
	}
	slices.Sort(hashes)
	return hashes
}

// OfferKey is what an offer a discovery passed on under hash is proven under (the protocol's §2.5
// item 3): for a membership device's locator, its credential's hash, proof.SessionOfferKey of the
// credential; for an unused link's locate hash, link.OfferKey of its secret. False when neither has
// it.
func (hub *Hub) OfferKey(hash string) ([]byte, bool) {
	now := hub.clock.Now()
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	for _, each := range hub.sessions {
		for _, candidate := range each.links {
			if candidate.waits(now) && candidate.locateHash == hash {
				return discoverylink.OfferKey(candidate.secret), true
			}
		}
		for _, candidate := range each.members {
			for _, held := range candidate.credentials {
				if held.locatorHash != "" && held.locatorHash == hash {
					return slices.Clone(held.hash[:]), true
				}
			}
		}
	}
	return nil, false
}

// OnLinksChanged has changed called whenever a link or a locator is made, so the discovery learns it
// at once; what ends one is found by the discovery's next look. Called before the hub is used; changed
// must never block, since the hub calls it holding its lock.
func (hub *Hub) OnLinksChanged(changed func()) {
	hub.onLinksChanged = changed
}

// linksChanged tells whoever registers the core's links with its discovery that they changed.
func (hub *Hub) linksChanged() {
	if hub.onLinksChanged != nil {
		hub.onLinksChanged()
	}
}
