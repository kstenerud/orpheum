// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi_test

import (
	"log/slog"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/rtcapi/rtcapitest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A Play page's playback was answered, its head reaching the page and its body never, and nothing in
// the core's log said where its bytes stopped (the owner's Mac Chrome, staging, 2026-10-30): a channel
// its client holds open long after the core handed its whole answer to SCTP is logged as it closes,
// by its stream and label as the page names it, with what SCTP still held.
func TestChannel_AChannelHeldOpenLongAfterItsAnswerIsLoggedByItsStream(t *testing.T) {
	t.Parallel()
	log := &testutil.Log{}
	server := rtcapi.New(t.Context(), echo, slog.New(slog.NewTextHandler(log, nil)))
	t.Cleanup(func() { _ = server.Close() }) //nolint:errcheck // the test is over
	fake := clocktest.NewFake(time.Unix(1_800_000_000, 0))
	rtcapi.SetClock(server, fake)
	rtcapi.SetBounds(server, rtcapi.Bounds{AnswerLingers: time.Minute})
	link := rtcapitest.Connect(t, server.Answer)
	held := link.Begin(rtcapi.RequestHead{Method: http.MethodPost, Path: "/api/v1/playback"}, true)
	if !held.IsEndedWithin(10 * time.Second) {
		t.Fatal("the core's answer did not end")
	}
	fake.Advance(time.Minute)
	held.Close()
	log.AwaitEvent(t, "channelLingered",
		"stream="+strconv.Itoa(held.Stream()), "label=request", "path=/api/v1/playback", "isHandedOverWhole=true",
		"bufferedAtFinish=", "bufferedAtClose=", "openAfterFinishMs=60000")
}
