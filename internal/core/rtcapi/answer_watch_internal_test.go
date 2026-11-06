// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"log/slog"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

const lingeringWarning = "channelLingered"

// watchFor is a watch on a channel with no stream yet, timed by fake, logging to log.
func watchFor(fake *clocktest.Fake, log *testutil.Log) *answerWatch {
	served := servedChannel{now: fake.Now, answerLingers: time.Minute}
	return newAnswerWatch(&webrtc.DataChannel{}, served, slog.New(slog.NewTextHandler(log, nil)))
}

// A client closes a channel within a round trip of its answer's end: only one held open
// answerLingers after the core's last byte for it, handed over whole or waiting for SCTP's buffer,
// is logged.
func TestAnswerWatch_LogsAChannelOnlyOnceItLingeredPastTheCoresLastByte(t *testing.T) {
	t.Parallel()
	fake := clocktest.NewFake(time.Unix(1_800_000_000, 0))
	log := &testutil.Log{}
	soon := watchFor(fake, log)
	soon.asked("/api/v1/items")
	soon.finished(true)
	fake.Advance(time.Minute - time.Millisecond)
	soon.closed()
	failed := watchFor(fake, log)
	failed.finished(true)
	failed.finished(false) // its end could not be handed over: the cut short warning says so
	fake.Advance(time.Minute)
	failed.closed()
	if len(testutil.LinesWith(log.String(), lingeringWarning)) != 0 {
		t.Fatalf("logged a channel closed in time, or one whose end was not handed over: %s", log.String())
	}
	late := watchFor(fake, log)
	late.asked("/api/v1/playback")
	late.finished(true)
	stuck := watchFor(fake, log)
	stuck.asked("/api/v1/chunk")
	stuck.draining(true)
	fake.Advance(time.Minute)
	late.closed()
	stuck.closed()
	log.AwaitEvent(t, lingeringWarning, "path=/api/v1/playback", "isHandedOverWhole=true", "openAfterFinishMs=60000")
	log.AwaitEvent(t, lingeringWarning, "path=/api/v1/chunk", "isHandedOverWhole=false", "drainWaitMs=60000")
}

// A channel opened on a stream another channel of its connection closed says how soon after, so a
// stream reused while its reset was still under way shows in the log.
func TestHeld_SaysHowSoonAStreamWasUsedAgain(t *testing.T) {
	t.Parallel()
	held := newHeldConnection(1, time.Unix(0, 0))
	closedAt := time.Unix(100, 0)
	if _, wasClosed := held.sinceStreamClosed(4, closedAt); wasClosed {
		t.Error("a stream no channel closed on says one did")
	}
	held.streamClosed(4, closedAt)
	since, wasClosed := held.sinceStreamClosed(4, closedAt.Add(9*time.Millisecond))
	if !wasClosed || since != 9*time.Millisecond {
		t.Errorf("stream 4 was closed %v before (%t); want 9ms", since, wasClosed)
	}
	if _, wasClosed := held.sinceStreamClosed(6, closedAt); wasClosed {
		t.Error("another stream says it was closed")
	}
}
