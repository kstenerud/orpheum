// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
)

// labThroughputPieceBytes is how much of the body is written at a time: a data channel message's
// most (rtcapi.MaxMessageBytes), so a write over WebRTC is one message, as a chunk's relay is.
const labThroughputPieceBytes = 64 << 10

// labThroughputOverHeader is the header the body says the path it came over in.
const labThroughputOverHeader = "Orpheum-Over"

// LabThroughputInput asks for the throughput test's body, of a size (webrtc-throughput-2026-10-28.md):
// 32 MiB unless it names another, from 1 to 256 MiB. Justified bounds, not settings: enough to time a
// link of a few hundred megabits for a second or more, and no more than a signed-in tester may have
// the core send at a request.
type LabThroughputInput struct {
	AuthorizationHeader
	Bytes int64 `default:"33554432" doc:"How many bytes the body holds" maximum:"268435456" minimum:"1048576" query:"bytes"` //nolint:lll // one struct tag
	// Leg is which of the page's ways of loading asked, for the log: its requests' transport, the
	// service worker's relay, or the core's LAN name.
	Leg string `doc:"Which way the page loads it, for the log" enum:"transport,relay,lan" query:"leg" required:"false"` //nolint:lll // one struct tag
	ClientFacts
}

func (server *Server) registerLabThroughput() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getLabThroughput", Method: http.MethodGet, Path: "/lab/throughput",
		Summary: "The throughput test's body: bytes of no meaning, for a lab page to time its loads by " +
			"(webrtc-throughput-2026-10-28.md)",
		Tags: []string{tagRunningIt},
		Responses: map[string]*huma.Response{
			"200": {Description: "The body", Content: map[string]*huma.MediaType{"application/octet-stream": {}}},
		},
		Extensions: callableBy(capabilityDeviceTests),
	}, server.getLabThroughput)
}

// getLabThroughput answers GET /lab-throughput: Bytes bytes that no compression shrinks, written a
// message at a time, behind the device tests' gate. Each is logged as it ends, with how it came and
// how long the core took to send it, which over WebRTC, its flow bounded by the page's credit, is near
// how long it took to arrive.
func (server *Server) getLabThroughput(ctx context.Context, input *LabThroughputInput) (*huma.StreamResponse, error) {
	dependencies, principal, err := server.deviceTestsCaller(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return &huma.StreamResponse{Body: func(stream huma.Context) {
		stream.SetHeader("Content-Type", "application/octet-stream")
		stream.SetHeader("Content-Length", strconv.FormatInt(input.Bytes, 10))
		stream.SetHeader("Cache-Control", "no-store")
		// The path it came over, for the page to show beside its rate: "webrtc" or "http".
		stream.SetHeader(labThroughputOverHeader, input.over)
		stream.SetStatus(http.StatusOK)
		startedAt := dependencies.Clock.Now()
		sent, writeErr := writeThroughputBody(stream.BodyWriter(), input.Bytes)
		elapsed := dependencies.Clock.Now().Sub(startedAt)
		server.logger.InfoContext(ctx, "a throughput test's body was sent", "leg", input.Leg,
			"over", input.over, "host", input.host, "bytes", sent, "isWhole", writeErr == nil,
			"elapsedMs", elapsed.Milliseconds(), "megabitsPerSecond", megabitsPerSecond(sent, elapsed.Seconds()),
			logKeyUserID, principal.UserID, logKeyDeviceID, principal.DeviceID, logKeyUserAgent, input.userAgent)
	}}, nil
}

// writeThroughputBody writes size bytes to writer a piece at a time, and answers how many it wrote,
// stopping at the first failure: a page that stops reading ends it.
func writeThroughputBody(writer io.Writer, size int64) (int64, error) {
	piece := throughputPiece()
	var sent int64
	for sent < size {
		next := piece[:min(int64(len(piece)), size-sent)]
		written, err := writer.Write(next)
		sent += int64(written)
		if err != nil {
			return sent, fmt.Errorf("write the throughput test's body: %w", err)
		}
	}
	return sent, nil
}

// throughputPiece is one piece of the body: random bytes, so no gzip on the way shrinks it, made
// anew for each body so no two are alike, which matters to nothing but costs nothing.
func throughputPiece() []byte {
	piece := make([]byte, labThroughputPieceBytes)
	for index := range piece {
		piece[index] = byte(rand.Uint32()) //nolint:gosec // bytes of no meaning, not a secret
	}
	return piece
}

// megabitsPerSecond is bytes over seconds in Mbit/s, rounded to one decimal; 0 for no time.
func megabitsPerSecond(bytes int64, seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	const bitsPerByte, bitsPerMegabit, tenths = 8, 1e6, 10
	return float64(int64(float64(bytes)*bitsPerByte/bitsPerMegabit/seconds*tenths)) / tenths
}
