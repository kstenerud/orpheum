// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
)

// sentRelayFailures are the relay failures the last heartbeat carried.
func sentRelayFailures(t *testing.T, built *world) []discoveryproto.RelayFailure {
	t.Helper()
	built.fake.mutex.Lock()
	defer built.fake.mutex.Unlock()
	if len(built.fake.heartbeats) == 0 {
		t.Fatal("no heartbeat was sent")
	}
	return built.fake.heartbeats[len(built.fake.heartbeats)-1].RelayFailures
}

// A relay the WebRTC server reports failed wakes the loop, whose next step sends a heartbeat at once
// carrying it, in Unix milliseconds; the heartbeat after carries it no more. A relay reported again
// before it was sent keeps its latest failure alone, and at most MaxRelayFailures are kept, the
// latest (D567 §3).
func TestReportRelayFailure_TheNextHeartbeatCarriesItAtOnceAndOnce(t *testing.T) {
	t.Parallel()
	built := claimedWorld(t)
	before := heartbeatCount(built)
	select {
	case <-built.service.wake: // the enrolment's own wake, which no loop took
	default:
	}
	failedAt := built.clock.Now()
	for _, at := range []time.Time{failedAt.Add(-time.Second), failedAt} {
		built.service.ReportRelayFailure(t.Context(), rtcapi.RelayFailure{Name: "relay.example", At: at})
	}
	select {
	case <-built.service.wake:
	default:
		t.Fatal("a relay's failure did not wake the loop")
	}
	built.service.step(t.Context())
	if count := heartbeatCount(built); count != before+1 {
		t.Fatalf("%d heartbeats after the failure; want one more than %d, at once", count, before)
	}
	want := []discoveryproto.RelayFailure{{Name: "relay.example", FailedAt: failedAt.UnixMilli()}}
	if sent := sentRelayFailures(t, built); !slices.Equal(sent, want) {
		t.Errorf("the heartbeat carried %+v; want %+v", sent, want)
	}
	built.clock.Advance(heartbeatEvery)
	built.service.step(t.Context())
	if sent := sentRelayFailures(t, built); len(sent) != 0 {
		t.Errorf("the heartbeat after carried %+v; want the failure sent once", sent)
	}
	for each := range discoveryproto.MaxRelayFailures + 2 {
		name := fmt.Sprintf("relay-%d.example", each)
		built.service.ReportRelayFailure(t.Context(), rtcapi.RelayFailure{Name: name, At: failedAt})
	}
	built.clock.Advance(heartbeatEvery)
	built.service.step(t.Context())
	sent := sentRelayFailures(t, built)
	if len(sent) != discoveryproto.MaxRelayFailures || sent[0].Name != "relay-2.example" {
		t.Errorf("the heartbeat carried %+v; want the latest %d", sent, discoveryproto.MaxRelayFailures)
	}
}
