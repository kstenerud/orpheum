// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// settableLinks are the core's links and locators as a test sets them.
type settableLinks struct {
	mutex    sync.Mutex
	links    []Link
	locators []string
}

func (settable *settableLinks) read(context.Context) ([]Link, []string, error) {
	settable.mutex.Lock()
	defer settable.mutex.Unlock()
	return slices.Clone(settable.links), slices.Clone(settable.locators), nil
}

func (settable *settableLinks) set(links []Link, locators []string) {
	settable.mutex.Lock()
	defer settable.mutex.Unlock()
	settable.links, settable.locators = links, locators
}

func TestStep_AClaimedCoreTellsItsDiscoveryItsLinksWhenTheyChangeAndUnderEachNewSession(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	links := &settableLinks{}
	built.service.parts.Links = links.read
	expires := built.clock.Now().Add(7 * 24 * time.Hour)
	links.set([]Link{{LocateHash: "hash-of-a-link", ExpiresAt: expires}}, []string{"hash-of-a-locator"})
	if _, err := built.service.Connect(ctx, built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.service.step(ctx)
	sent := func() ([]discoveryproto.LocateHashesRequest, []discoveryproto.LocatorsRequest) {
		built.fake.mutex.Lock()
		defer built.fake.mutex.Unlock()
		return slices.Clone(built.fake.locateHashes), slices.Clone(built.fake.locators)
	}
	if hashes, _ := sent(); len(hashes) != 0 {
		t.Errorf("a pending core sent its links %v; want none until it is claimed", hashes)
	}
	built.fake.claim()
	built.service.step(ctx)
	built.service.step(ctx)
	hashes, locators := sent()
	if len(hashes) != 1 || len(locators) != 1 || !slices.Equal(locators[0].Hashes, []string{"hash-of-a-locator"}) ||
		len(hashes[0].Hashes) != 1 || hashes[0].Hashes[0] != (discoveryproto.LocateHash{
		Hash: "hash-of-a-link", ExpiresIn: int64((7 * 24 * time.Hour) / time.Second),
	}) {
		t.Fatalf("once claimed the core sent links %+v and locators %+v; want its one link, with its life in "+
			"seconds, and its one locator, once", hashes, locators)
	}
	built.clock.Advance(heartbeatEvery)
	built.service.step(ctx)
	if hashes, _ = sent(); len(hashes) != 1 {
		t.Errorf("unchanged, the links were sent %d times; want once", len(hashes))
	}
	links.set(nil, []string{"hash-of-a-locator", "hash-of-another"})
	built.service.step(ctx)
	if hashes, locators = sent(); len(hashes) != 2 || len(hashes[1].Hashes) != 0 ||
		!slices.Equal(locators[1].Hashes, []string{"hash-of-a-locator", "hash-of-another"}) {
		t.Errorf("after a change the core sent links %+v and locators %+v; want the new full sets", hashes, locators)
	}
	built.fake.mutex.Lock()
	built.fake.tokens = map[string]bool{} // the discovery restarted, forgetting its sessions and the sets
	built.fake.mutex.Unlock()
	built.clock.Advance(heartbeatEvery)
	built.service.step(ctx) // the heartbeat's session is refused, a new one opened, and the links sent under it
	if hashes, _ = sent(); len(hashes) != 3 {
		t.Errorf("under a new session the links were sent %d times in all; want them sent again", len(hashes))
	}
}
