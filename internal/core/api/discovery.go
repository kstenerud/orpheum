// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/portmap"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
)

// DiscoveryProbeBody is what the discovery's outside probe found of one of the core's addresses, or
// of "webrtc".
type DiscoveryProbeBody struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Kind    string `doc:"What the probe tried there" enum:"https,turn" json:"kind"`
	Result  string `doc:"reached, or the hop a failure stopped at" json:"result"`
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
	ReachableFrom   string               `doc:"As the probe found; absent before it ran" enum:"public_ipv4,public_ipv4_other_port,ipv6,nowhere" json:"reachableFrom,omitempty"` //nolint:lll // one struct tag
	Published       []string             `doc:"The addresses the discovery publishes" json:"published"`
	Probe           []DiscoveryProbeBody `json:"probe"`
	LastHeartbeatAt int64                `doc:"Unix milliseconds; 0 before the first" json:"lastHeartbeatAt"`
	Problem         string               `doc:"Why the last call to the discovery failed; empty when it did not" json:"problem,omitempty"`                             //nolint:lll // one struct tag
	DeprecatedUntil string               `doc:"When the discovery stops speaking this core's protocol, when it said so" json:"deprecatedUntil,omitempty"`              //nolint:lll // one struct tag
	ClientOrigin    string               `doc:"Where the discovery serves its client, which the server's links lead to" json:"clientOrigin,omitempty"`                 //nolint:lll // one struct tag
	Fingerprint     string               `doc:"The identity key's fingerprint as a person reads it, in safe32 groups; links carry the same digest" json:"fingerprint"` //nolint:lll // one struct tag
	PortMapping     PortMappingBody      `json:"portMapping"`
	// WebRTCPaths are how many WebRTC connections took each path since the core started (D274 §3).
	WebRTCPaths []WebRTCPathBody `json:"webrtcPaths"`
}

// PortMappingBody is what the home router opened for the core while its discovery publishes it
// (D273 §7): each opening, and why each way that failed failed.
type PortMappingBody struct {
	IsWanted  bool                  `doc:"Whether the core is asking: should_map_ports on and a discovery publishing it" json:"isWanted"` //nolint:lll // one struct tag
	CheckedAt int64                 `doc:"Unix milliseconds; 0 before the first" json:"checkedAt"`
	Mappings  []PortMappingItemBody `json:"mappings"`
	Problems  []string              `json:"problems"`
}

// PortMappingItemBody is one opening: a public IPv4 address and port forwarded to the core, or an
// IPv6 address of the core the router's firewall lets through.
type PortMappingItemBody struct {
	Method    string `enum:"pcp,natpmp,upnp" json:"method"`
	Internal  string `doc:"The core's address and port" json:"internal"`
	External  string `doc:"Where the internet reaches it" json:"external"`
	ExpiresAt int64  `doc:"Unix milliseconds; 0 for one the router made for good" json:"expiresAt"`
}

// WebRTCPathBody is how many connections took one path.
type WebRTCPathBody struct {
	Path  string `enum:"direct_ipv6,direct_ipv4,punched_ipv6,punched_ipv4,relayed,failed" json:"path"`
	Count int    `json:"count"`
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
	body := discoveryStatusBody(dependencies.Discovery.Status())
	body.PortMapping = portMappingBody(dependencies.PortMapping.Report())
	body.WebRTCPaths = server.webrtcPathsBody()
	return &DiscoveryStatusOutput{Body: body}, nil
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
	body := discoveryStatusBody(status)
	body.PortMapping = portMappingBody(dependencies.PortMapping.Report())
	body.WebRTCPaths = server.webrtcPathsBody()
	return &DiscoveryStatusOutput{Body: body}, nil
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
		ClientOrigin: status.ClientOrigin, Fingerprint: status.FingerprintText,
	}
	if !status.ClaimExpiresAt.IsZero() {
		body.ClaimExpiresAt = status.ClaimExpiresAt.UnixMilli()
	}
	if !status.LastHeartbeatAt.IsZero() {
		body.LastHeartbeatAt = status.LastHeartbeatAt.UnixMilli()
	}
	for _, probe := range status.Probe {
		body.Probe = append(body.Probe, DiscoveryProbeBody{
			Address: probe.Address, Port: probe.Port, Kind: probe.Kind, Result: probe.Result,
		})
	}
	return body
}

// portMappingBody is a port mapping report as the API sends it.
func portMappingBody(report portmap.Report) PortMappingBody {
	body := PortMappingBody{
		IsWanted: report.IsWanted, Mappings: []PortMappingItemBody{}, Problems: append([]string{}, report.Problems...),
	}
	if !report.CheckedAt.IsZero() {
		body.CheckedAt = report.CheckedAt.UnixMilli()
	}
	for _, mapping := range report.Mappings {
		item := PortMappingItemBody{
			Method: mapping.Method, Internal: mapping.Internal.String(), External: mapping.External.String(),
		}
		if !mapping.ExpiresAt.IsZero() {
			item.ExpiresAt = mapping.ExpiresAt.UnixMilli()
		}
		body.Mappings = append(body.Mappings, item)
	}
	return body
}

// webrtcPathsBody are the paths WebRTC connections took, each with its count, every path listed.
func (server *Server) webrtcPathsBody() []WebRTCPathBody {
	counts := map[rtcapi.Path]int{}
	if server.rtc != nil {
		counts = server.rtc.PathCounts()
	}
	paths := make([]WebRTCPathBody, 0, len(rtcapi.Paths()))
	for _, path := range rtcapi.Paths() {
		paths = append(paths, WebRTCPathBody{Path: string(path), Count: counts[path]})
	}
	return paths
}
