// SPDX-License-Identifier: GPL-3.0-or-later

package rtcapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pion/webrtc/v4"
)

// channelWriter is an http.ResponseWriter that sends a response over its request's channel, waiting
// whenever the channel holds bufferedHighBytes until it drains to bufferedLowBytes.
type channelWriter struct {
	ctx        context.Context //nolint:containedctx // one response's context, ended with its channel
	channel    *webrtc.DataChannel
	headers    http.Header
	status     int
	isHeadSent bool
	drained    chan struct{}
	failed     error
}

func newChannelWriter(ctx context.Context, channel *webrtc.DataChannel) *channelWriter {
	writer := &channelWriter{
		ctx: ctx, channel: channel, headers: http.Header{}, drained: make(chan struct{}, 1),
	}
	channel.SetBufferedAmountLowThreshold(bufferedLowBytes)
	channel.OnBufferedAmountLow(func() {
		select {
		case writer.drained <- struct{}{}:
		default:
		}
	})
	return writer
}

// Header is the response's headers, sent with its head.
func (writer *channelWriter) Header() http.Header { return writer.headers }

// WriteHeader sets the response's status, sent with its head at the first write.
func (writer *channelWriter) WriteHeader(status int) {
	if writer.status == 0 {
		writer.status = status
	}
}

// Write sends body, in messages of at most MaxMessageBytes, after the head.
func (writer *channelWriter) Write(body []byte) (int, error) {
	if err := writer.sendHead(); err != nil {
		return 0, err
	}
	written := 0
	for len(body) > 0 {
		size := min(len(body), MaxMessageBytes)
		if err := writer.send(body[:size]); err != nil {
			return written, err
		}
		written += size
		body = body[size:]
	}
	return written, nil
}

// Flush has nothing to do: every write is sent as it is made.
func (writer *channelWriter) Flush() {}

// sendHead sends the response's head, once.
func (writer *channelWriter) sendHead() error {
	if writer.isHeadSent {
		return writer.failed
	}
	writer.isHeadSent = true
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	head, err := json.Marshal(ResponseHead{Status: writer.status, Headers: writer.headers})
	if err != nil {
		writer.failed = fmt.Errorf("encode the response's head: %w", err)
		return writer.failed
	}
	if err = writer.channel.SendText(string(head)); err != nil {
		writer.failed = fmt.Errorf("send the response's head: %w", err)
	}
	return writer.failed
}

// send sends one message of the body, waiting first while the channel holds too much.
func (writer *channelWriter) send(message []byte) error {
	if writer.failed != nil {
		return writer.failed
	}
	for writer.channel.BufferedAmount() > bufferedHighBytes {
		select {
		case <-writer.drained:
		case <-writer.ctx.Done():
			writer.failed = writer.ctx.Err()
			return writer.failed
		}
	}
	if err := writer.channel.Send(message); err != nil {
		writer.failed = fmt.Errorf("send the response: %w", err)
	}
	return writer.failed
}

// finish sends the head if nothing was written, and the empty message that ends the response.
func (writer *channelWriter) finish() error {
	if err := writer.sendHead(); err != nil {
		return err
	}
	return writer.send([]byte{})
}
