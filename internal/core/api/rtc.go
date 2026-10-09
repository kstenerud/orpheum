// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
)

// RTCAnswerer answers a client's WebRTC offer, after which the client sends its requests over data
// channels (webrtc-transport.md): rtcapi.Server.
type RTCAnswerer interface {
	Answer(ctx context.Context, offer rtcapi.Description) (rtcapi.Description, error)
}

// SetRTC names what answers WebRTC offers; with none, the offer endpoint says the server takes none.
func (server *Server) SetRTC(answerer RTCAnswerer) {
	server.rtc = answerer
}

// SessionDescriptionBody is a WebRTC session description: an offer or an answer, and its SDP.
type SessionDescriptionBody struct {
	Type string `enum:"offer,answer" json:"type"`
	SDP  string `json:"sdp"           maxLength:"65536" minLength:"1"`
}

// RTCOfferInput is a signed-in client's offer.
type RTCOfferInput struct {
	AuthorizationHeader
	Body SessionDescriptionBody
}

// RTCAnswerOutput is the core's answer, with every candidate it gathered.
type RTCAnswerOutput struct {
	Body SessionDescriptionBody
}

func (server *Server) registerRTC() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "postRTCOffer", Method: http.MethodPost, Path: "/rtc/offer", Tags: []string{tagRunningIt},
		Summary: "Answer a WebRTC offer, after which the client sends its requests over data channels (D273 §8)",
	}, server.postRTCOffer)
}

func (server *Server) postRTCOffer(ctx context.Context, input *RTCOfferInput) (*RTCAnswerOutput, error) {
	if _, _, err := server.requireSignedIn(ctx, input.Authorization); err != nil {
		return nil, err
	}
	if server.rtc == nil {
		return nil, newError(http.StatusNotFound, CodeNotFound, "this server takes no WebRTC connections")
	}
	answer, err := server.rtc.Answer(ctx, rtcapi.Description{Type: input.Body.Type, SDP: input.Body.SDP})
	switch {
	case errors.Is(err, rtcapi.ErrTooManyConnections):
		return nil, newError(http.StatusTooManyRequests, CodeRateLimited, err.Error())
	case err != nil:
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "body.sdp", err.Error())
	}
	output := &RTCAnswerOutput{}
	output.Body.Type, output.Body.SDP = answer.Type, answer.SDP
	return output, nil
}
