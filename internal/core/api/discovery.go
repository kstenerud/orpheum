// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/discovery"
)

// DiscoveryProbeBody is what the discovery's outside probe found of one of the core's addresses, or
// of "webrtc".
type DiscoveryProbeBody struct {
	Candidate string `json:"candidate"`
	Result    string `json:"result"`
}

// DiscoveryStatusBody is where the core stands with its discovery (D273): none, waiting to be
// claimed with the code its owner types, or claimed, with what the discovery published and probed.
type DiscoveryStatusBody struct {
	BaseURL string `doc:"The discovery's address; empty when the core is enrolled with none" json:"baseUrl"`
	CoreID  string `json:"coreId,omitempty"`
	State   string `doc:"Absent before the discovery's first answer" enum:"pending,claimed,grace,frozen,released,disabled" json:"state,omitempty"` //nolint:lll // one struct tag
	Zone    string `json:"zone,omitempty"`
	// ClaimCode is what the owner types at the discovery to claim the core, while it is pending.
	ClaimCode       string               `json:"claimCode,omitempty"`
	ClaimExpiresAt  int64                `doc:"Unix milliseconds; 0 when there is no code" json:"claimExpiresAt"`
	ReachableFrom   string               `doc:"As the probe found; absent before it ran" enum:"everywhere,ipv6_or_webrtc,nowhere" json:"reachableFrom,omitempty"` //nolint:lll // one struct tag
	Published       []string             `doc:"The addresses the discovery publishes" json:"published"`
	Probe           []DiscoveryProbeBody `json:"probe"`
	LastHeartbeatAt int64                `doc:"Unix milliseconds; 0 before the first" json:"lastHeartbeatAt"`
	Problem         string               `doc:"Why the last call to the discovery failed; empty when it did not" json:"problem,omitempty"`                //nolint:lll // one struct tag
	DeprecatedUntil string               `doc:"When the discovery stops speaking this core's protocol, when it said so" json:"deprecatedUntil,omitempty"` //nolint:lll // one struct tag
}

// DiscoveryStatusOutput is the core's standing with its discovery.
type DiscoveryStatusOutput struct {
	Body DiscoveryStatusBody
}

// ConnectDiscoveryInput is an admin connecting the core to a discovery.
type ConnectDiscoveryInput struct {
	AuthorizationHeader
	Body struct {
		BaseURL string `doc:"The discovery's address: the service's, or the owner's own" json:"baseUrl" maxLength:"2048" minLength:"1"` //nolint:lll // one struct tag
	}
}

// discoveryPath is the core's discovery, which an admin reads, connects and leaves.
const discoveryPath = "/system/discovery"

func (server *Server) registerDiscovery() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getDiscovery", Method: http.MethodGet, Path: discoveryPath,
		Tags: []string{tagRunningIt}, Summary: "Where the core stands with its discovery (admin; D273)",
	}, server.getDiscovery)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "connectDiscovery", Method: http.MethodPost, Path: discoveryPath,
		Tags:    []string{tagRunningIt},
		Summary: "Enrol the core with a discovery, which answers with the code its owner claims it by (admin; D273)",
	}, server.connectDiscovery)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "leaveDiscovery", Method: http.MethodDelete, Path: discoveryPath,
		Tags: []string{tagRunningIt}, DefaultStatus: http.StatusNoContent,
		Summary: "Take the core out of its discovery (admin; D273)",
	}, server.leaveDiscovery)
}

func (server *Server) getDiscovery(ctx context.Context, input *AuthorizationHeader) (*DiscoveryStatusOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return &DiscoveryStatusOutput{Body: discoveryStatusBody(dependencies.Discovery.Status())}, nil
}

// connectDiscovery enrols the core: an address that is not a discovery's is refused, as is a core
// already enrolled, and a discovery that does not answer is the failing leg.
func (server *Server) connectDiscovery(
	ctx context.Context, input *ConnectDiscoveryInput,
) (*DiscoveryStatusOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	status, err := dependencies.Discovery.Connect(ctx, input.Body.BaseURL)
	switch {
	case errors.Is(err, discovery.ErrNotADiscoveryURL):
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "body.baseUrl", err.Error())
	case errors.Is(err, discovery.ErrAlreadyEnrolled):
		return nil, newError(http.StatusConflict, CodeConflict, err.Error())
	case err != nil:
		failure := newDetailedError(http.StatusBadGateway, err.Error(),
			ProviderRefusalDetailBody{Reason: ReasonUnavailable})
		failure.ErrorBody.Error.FailedHop = HopProvider
		return nil, failure
	}
	return &DiscoveryStatusOutput{Body: discoveryStatusBody(status)}, nil
}

func (server *Server) leaveDiscovery(ctx context.Context, input *AuthorizationHeader) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if leaveErr := dependencies.Discovery.Leave(ctx); leaveErr != nil {
		return nil, fmt.Errorf("leave the discovery: %w", leaveErr)
	}
	return &struct{}{}, nil
}

// discoveryStatusBody is a status as the API sends it.
func discoveryStatusBody(status discovery.Status) DiscoveryStatusBody {
	body := DiscoveryStatusBody{
		BaseURL: status.BaseURL, CoreID: status.CoreID, State: string(status.State), Zone: status.Zone,
		ClaimCode: status.ClaimCode, ReachableFrom: status.ReachableFrom,
		Published: append([]string{}, status.Published...), Probe: []DiscoveryProbeBody{},
		Problem: status.Problem, DeprecatedUntil: status.DeprecatedUntil,
	}
	if !status.ClaimExpiresAt.IsZero() {
		body.ClaimExpiresAt = status.ClaimExpiresAt.UnixMilli()
	}
	if !status.LastHeartbeatAt.IsZero() {
		body.LastHeartbeatAt = status.LastHeartbeatAt.UnixMilli()
	}
	for _, probe := range status.Probe {
		body.Probe = append(body.Probe, DiscoveryProbeBody{Candidate: probe.Candidate, Result: probe.Result})
	}
	return body
}
