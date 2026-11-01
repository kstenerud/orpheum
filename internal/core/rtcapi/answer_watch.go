// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"log/slog"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// answerLingersAfter is how long a channel may stay open after the core's last byte for its response
// before its close is logged as a response the client may never have had: a justified constant, far
// past any round trip, in which a client that had the response's end closes the channel, and short of
// the page's silentAtMostMs (40 s), at which it gives the request up. A Play page's playback was
// answered, its head reaching the page and its body never, and nothing in the core's log said where
// its bytes stopped (the owner's Mac Chrome, staging, 2026-10-30).
const answerLingersAfter = 10 * time.Second

// answerWatch is what the log says of a response whose channel its client held open long after the
// core's last byte for it: handed over whole to SCTP, or waiting for SCTP's buffer to drain. Its
// SCTP stream and label name it as the page's give-up report does, its buffered amounts say whether
// the client's SCTP acknowledged every byte (pion counts a byte buffered until it is acknowledged),
// and the time since another channel of the same stream closed says whether its stream was reused
// at once.
type answerWatch struct {
	channel *webrtc.DataChannel
	served  servedChannel
	logger  *slog.Logger
	// streamClosedBefore is how long before this channel opened another on its stream closed;
	// hasStreamClosedBefore says whether one did on its connection.
	streamClosedBefore    time.Duration
	hasStreamClosedBefore bool

	mutex            sync.Mutex
	path             string
	sentBytes        int
	finishedAt       time.Time
	bufferedAtFinish uint64
	drainWaitSince   time.Time
}

// newAnswerWatch watches the response on channel, served as served says.
func newAnswerWatch(channel *webrtc.DataChannel, served servedChannel, logger *slog.Logger) *answerWatch {
	watch := &answerWatch{channel: channel, served: served, logger: logger}
	if id := channel.ID(); id != nil && served.held != nil && served.now != nil {
		watch.streamClosedBefore, watch.hasStreamClosedBefore = served.held.sinceStreamClosed(*id, served.now())
	}
	return watch
}

// asked notes the path the channel's request asked for.
func (watch *answerWatch) asked(path string) {
	watch.mutex.Lock()
	defer watch.mutex.Unlock()
	watch.path = path
}

// sent notes bytes of the response handed to SCTP.
func (watch *answerWatch) sent(bytes int) {
	watch.mutex.Lock()
	defer watch.mutex.Unlock()
	watch.sentBytes += bytes
}

// draining notes whether the response waits for its channel's SCTP buffer to drain.
func (watch *answerWatch) draining(isWaiting bool) {
	watch.mutex.Lock()
	defer watch.mutex.Unlock()
	watch.drainWaitSince = time.Time{}
	if isWaiting && watch.served.now != nil {
		watch.drainWaitSince = watch.served.now()
	}
}

// finished notes whether the response is handed to SCTP whole, its end message too: true as the end
// is handed over, false should that fail.
func (watch *answerWatch) finished(isWhole bool) {
	if watch.served.now == nil {
		return
	}
	watch.mutex.Lock()
	defer watch.mutex.Unlock()
	watch.finishedAt = time.Time{}
	if isWhole {
		watch.finishedAt = watch.served.now()
		watch.bufferedAtFinish = watch.channel.BufferedAmount()
	}
}

// closed notes the channel closed: its stream for the next channel to reuse it, and, when it stayed
// open answerLingers or longer after the core's last byte for it, a warning.
func (watch *answerWatch) closed() {
	if watch.served.now == nil {
		return
	}
	now := watch.served.now()
	id := watch.channel.ID()
	if id != nil && watch.served.held != nil {
		watch.served.held.streamClosed(*id, now)
	}
	watch.mutex.Lock()
	defer watch.mutex.Unlock()
	isHandedOver := !watch.finishedAt.IsZero()
	since := watch.finishedAt
	if !isHandedOver {
		since = watch.drainWaitSince
	}
	if since.IsZero() || now.Sub(since) < watch.served.answerLingers {
		return
	}
	watch.logger.Warn("rtcapi: a response's channel stayed open long after the core's last byte for it",
		watch.logged(now, isHandedOver)...)
}

// logged is the warning's fields. The caller holds the mutex.
func (watch *answerWatch) logged(now time.Time, isHandedOver bool) []any {
	fields := []any{
		"connection", watch.connectionName(), "stream", streamOf(watch.channel), "label", watch.channel.Label(),
		pathField, watch.path, "isHandedOverWhole", isHandedOver, "sentBytes", watch.sentBytes,
		"bufferedAtClose", watch.channel.BufferedAmount(),
	}
	if isHandedOver {
		fields = append(fields, "bufferedAtFinish", watch.bufferedAtFinish,
			"openAfterFinishMs", now.Sub(watch.finishedAt).Milliseconds())
	} else {
		fields = append(fields, "drainWaitMs", now.Sub(watch.drainWaitSince).Milliseconds())
	}
	if watch.hasStreamClosedBefore {
		fields = append(fields, "streamClosedBeforeMs", watch.streamClosedBefore.Milliseconds())
	}
	return fields
}

// connectionName is how the log names the channel's connection, or "" without a record of it.
func (watch *answerWatch) connectionName() string {
	if watch.served.held == nil {
		return ""
	}
	return watch.served.held.name()
}

// streamOf is a channel's SCTP stream id, or -1 before it has one.
func streamOf(channel *webrtc.DataChannel) int {
	if id := channel.ID(); id != nil {
		return int(*id)
	}
	return -1
}
