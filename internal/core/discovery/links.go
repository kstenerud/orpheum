// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// Link is one link a discovery may find the core by: its secret's hash (link.LocateHash), and when it
// ends.
type Link struct {
	LocateHash string
	ExpiresAt  time.Time
}

// LinksChanged has the core tell its discovery at once what it may find the core by, a link or a
// locator having been made (auth.Service.OnLinksChanged).
func (service *Service) LinksChanged() {
	service.nudge()
}

// registerLinks tells the discovery every link and device locator it may find the core by, when they
// changed or a new session began since it was last told, each the full set (the protocol's §3.1,
// §4). It returns the wait after a failure, and the failure.
func (service *Service) registerLinks(ctx context.Context, status Status) (time.Duration, error) {
	if service.parts.Links == nil {
		return 0, nil
	}
	links, locators, err := service.parts.Links(ctx)
	if err != nil {
		return service.failed(ctx, "read the core's links", err), err
	}
	token, isLive := service.liveToken()
	service.mutex.Lock()
	isSent := isLive && token == service.sentUnder && slices.Equal(links, service.sentLinks) &&
		slices.Equal(locators, service.sentLocators)
	service.mutex.Unlock()
	if isSent {
		return 0, nil
	}
	now := service.parts.Clock.Now()
	hashes := make([]discoveryproto.LocateHash, 0, len(links))
	for _, live := range links {
		hashes = append(hashes, discoveryproto.LocateHash{
			Hash: live.LocateHash, ExpiresIn: int64(max(live.ExpiresAt.Sub(now), 0) / time.Second),
		})
	}
	err = service.signedCall(ctx, callOf{
		base: status.BaseURL, method: http.MethodPut, path: discoveryproto.PathLocateHashes,
		body: discoveryproto.LocateHashesRequest{Hashes: hashes},
	})
	if err == nil {
		err = service.signedCall(ctx, callOf{
			base: status.BaseURL, method: http.MethodPut, path: discoveryproto.PathLocators,
			body: discoveryproto.LocatorsRequest{Hashes: append([]string{}, locators...)},
		})
	}
	if err != nil {
		return service.failed(ctx, "tell the discovery the core's links", err), err
	}
	token, _ = service.liveToken()
	service.mutex.Lock()
	service.sentLinks, service.sentLocators, service.sentUnder = links, locators, token
	service.mutex.Unlock()
	return 0, nil
}
