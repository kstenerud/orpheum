// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/reachability"
)

// ReachabilityResult is one hop's check: reached, not checked, or where it failed, with the failing
// layer's words.
type ReachabilityResult struct {
	Outcome reachability.Outcome `enum:"reached,notChecked,nameUnresolved,notAnswering,certificateRefused,anotherServer" json:"outcome"` //nolint:lll // one struct tag
	Detail  string               `json:"detail,omitempty"`
}

// ReachabilityOutput is a check of the server's path from outside in (D243 §7): its public name
// and where it points, HTTPS through it to this server, and the voice relay's port over UDP and
// TCP.
type ReachabilityOutput struct {
	Body struct {
		CheckedAt int64              `doc:"Unix milliseconds" json:"checkedAt"`
		Name      string             `doc:"The public name; empty when the server has none" json:"name"`
		Addresses []string           `doc:"Where the name points" json:"addresses"`
		HTTPS     ReachabilityResult `json:"https"`
		RelayUDP  ReachabilityResult `json:"relayUdp"`
		RelayTCP  ReachabilityResult `json:"relayTcp"`
	}
}

// ReachabilityEchoInput is a reachability check's own request coming back from outside.
type ReachabilityEchoInput struct {
	Nonce string `maxLength:"64" minLength:"1" path:"nonce"`
}

func (server *Server) registerReachability() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "checkReachability", Method: http.MethodPost, Path: "/system/reachability",
		Tags:       []string{tagRunningIt},
		Summary:    "Check the public name and voice relay from outside in, naming a failing hop (D243 §7)",
		Extensions: callableBy(capabilityAdmin),
	}, server.checkReachability)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "echoReachability", Method: http.MethodGet, Path: reachability.EchoPath + "{nonce}",
		Tags: []string{tagRunningIt}, DefaultStatus: http.StatusNoContent,
		Summary:    "Answer a reachability check's own request, which came back through the public name",
		Extensions: callableBy(capabilityNone),
	}, server.echoReachability)
}

// checkReachability runs the check now. It is a POST: it sends requests out and waits for them.
func (server *Server) checkReachability(ctx context.Context, input *AuthorizationHeader) (*ReachabilityOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	report := dependencies.Reachability.Check(ctx)
	output := &ReachabilityOutput{}
	output.Body.CheckedAt, output.Body.Name = report.CheckedAt.UnixMilli(), report.Name
	output.Body.Addresses = append([]string{}, report.Addresses...)
	output.Body.HTTPS = ReachabilityResult(report.HTTPS)
	output.Body.RelayUDP = ReachabilityResult(report.RelayUDP)
	output.Body.RelayTCP = ReachabilityResult(report.RelayTCP)
	return output, nil
}

// echoReachability answers 204 for a nonce this server's check made, and as for nothing otherwise:
// it says only "this is the server that asked", to whoever asks.
func (server *Server) echoReachability(_ context.Context, input *ReachabilityEchoInput) (*struct{}, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	if !dependencies.Reachability.Recognises(input.Nonce) {
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such check")
	}
	return &struct{}{}, nil
}
