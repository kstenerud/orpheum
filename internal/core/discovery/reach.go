// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

// The step loop's look, while it backs off after reaching no discovery, whether the discovery
// answers again (D437 §5).

import (
	"context"
	"net/http"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// reachProbeEvery is how often the step loop, waiting out a backoff after the discovery could not
// be reached, looks whether it can be again: the last holder of the realm asks at once when its
// network comes back, not up to discoveryRetryMostSeconds later, and so does not miss its window
// (D437 §5). A look is one small call the discovery answers without a session.
const reachProbeEvery = 15 * time.Second

// waitForNextStep waits wait, or until the loop is woken, saying false when ctx ends. While the last
// failure did not reach the discovery, it looks every reachProbeEvery whether the discovery answers
// again, and ends the wait at once when it does.
func (service *Service) waitForNextStep(ctx context.Context, wait time.Duration) bool {
	timer := service.parts.Clock.NewTimer(wait)
	defer timer.Stop()
	for {
		var probe clock.Timer
		var probed <-chan time.Time
		if service.isWaitingForTheNetwork() {
			probe = service.parts.Clock.NewTimer(reachProbeEvery)
			probed = probe.C()
		}
		select {
		case <-ctx.Done():
			return false
		case <-service.wake:
			return true
		case <-timer.C():
			return true
		case <-probed:
			if service.reachesAgain(ctx) {
				return true
			}
		}
		if probe != nil {
			probe.Stop()
		}
	}
}

// isWaitingForTheNetwork is whether the step loop's last failure did not reach the discovery.
func (service *Service) isWaitingForTheNetwork() bool {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	return service.isUnreachable && service.status.BaseURL != ""
}

// reachProbeTimeout bounds one look whether the discovery answers again.
const reachProbeTimeout = 10 * time.Second

// reachesAgain looks whether the discovery answers again: any answer it gives itself, a refusal of
// the unsigned call among them, but not a proxy's saying it cannot reach it (502, 503, 504). When it
// does, the signalling loop is woken too, and it is logged.
func (service *Service) reachesAgain(ctx context.Context) bool {
	looking, cancel := context.WithTimeout(ctx, reachProbeTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(looking, http.MethodGet,
		service.Status().BaseURL+discoveryproto.PathStatus, http.NoBody)
	if err != nil {
		return false
	}
	request.Header.Set(discoveryproto.ProtocolHeader, discoveryproto.ProtocolVersion)
	response, err := service.parts.HTTP.Do(request)
	if err != nil {
		return false
	}
	_ = response.Body.Close() //nolint:errcheck // only its status is read
	if isProxyUnreached(response.StatusCode) {
		return false
	}
	service.mutex.Lock()
	service.isUnreachable = false
	service.mutex.Unlock()
	service.parts.Logger.InfoContext(ctx, "discovery: the discovery answers again; asking at once")
	select {
	case service.reachedAgain <- struct{}{}:
	default:
	}
	return true
}

// isProxyUnreached is a status a proxy in front of the discovery answers when it cannot reach it.
func isProxyUnreached(code int) bool {
	return code == http.StatusBadGateway || code == http.StatusServiceUnavailable ||
		code == http.StatusGatewayTimeout
}
