// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/rtcapi/rtcapitest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// A connected connection's packets go through the pacer: a 5 MiB response takes, on the pacer's
// clock, at least what 300 Mb/s allows, where unpaced it took none (D397).
func TestPacer_PacesWhatAConnectionSends(t *testing.T) {
	t.Parallel()
	server := newServer(t)
	fake := clocktest.NewFake(time.Unix(1_800_000_000, 0))
	rtcapi.SetPacerClock(server, fake)
	link := rtcapitest.Connect(t, server.Answer, rtcapitest.OverIPv4())
	start := fake.Now()
	link.Do(rtcapi.RequestHead{Method: http.MethodGet, Path: "/big"}, nil)
	// 5 MiB at 300 Mb/s is 140 ms; 4 MiB, 112, leaves what went before the pacer followed.
	if took, least := fake.Now().Sub(start), 4<<20*time.Second/(300_000_000/8); took < least {
		t.Errorf("a 5 MiB response took %v on the pacer's clock; paced at most 300 Mb/s, want at least %v",
			took, least)
	}
}
