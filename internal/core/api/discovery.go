// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/internal/core/auth"
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
	Issuer          string   `doc:"The discovery's issuer, the origin the server was enrolled by; empty when it is enrolled with none (D391 §3)" json:"issuer"`                                            //nolint:lll // one struct tag
	SuggestedIssuer string   `doc:"The discovery an admin is offered first; empty when this build suggests none (D391 §7)" json:"suggestedIssuer,omitempty"`                                               //nolint:lll // one struct tag
	CoreAPI         string   `doc:"The core API the discovery's address document names, which the server calls" json:"coreApi,omitempty"`                                                                  //nolint:lll // one struct tag
	LoaderOrigin    string   `doc:"Where the discovery serves the loader, as its address document names it" json:"loaderOrigin,omitempty"`                                                                 //nolint:lll // one struct tag
	Homepage        string   `doc:"What the discovery is, as its address document names it" json:"homepage,omitempty"`                                                                                     //nolint:lll // one struct tag
	Identity        string   `doc:"The discovery's identity, its root chain's genesis hash (od1-...), which the server's signatures are bound to (D393 §4)" json:"identity,omitempty"`                     //nolint:lll // one struct tag
	Name            string   `doc:"What the discovery's address document calls it" json:"name,omitempty"`                                                                                                  //nolint:lll // one struct tag
	IsSuggested     bool     `doc:"Whether the discovery is the one this build suggests, the known Orpheum service" json:"isSuggested"`                                                                    //nolint:lll // one struct tag
	RootKeys        []string `doc:"The discovery root keys whose signatures count (suspended keys left out), each its role and fingerprint as ssh-keygen -l shows it (D393 §1)" json:"rootKeys"`           //nolint:lll // one struct tag
	RootVersion     int64    `doc:"The version of the discovery's root chain the server holds" json:"rootVersion"`                                                                                         //nolint:lll // one struct tag
	DocumentAt      int64    `doc:"Unix milliseconds the address document held was issued; 0 when none is" json:"documentAt"`                                                                              //nolint:lll // one struct tag
	DocumentStaleAt int64    `doc:"Unix milliseconds the address document held expired, since when the server cannot confirm its addresses are current; 0 while current" json:"documentStaleAt,omitempty"` //nolint:lll // one struct tag
	DocumentProblem string   `doc:"What the last look at the discovery's places found wrong: a conflict of root statements, equivocation, a document no longer signed" json:"documentProblem,omitempty"`   //nolint:lll // one struct tag
	CoreID          string   `json:"coreId,omitempty"`
	State           string   `doc:"Absent before the discovery's first answer" enum:"pending,claimed,grace,frozen,released,disabled" json:"state,omitempty"` //nolint:lll // one struct tag
	Zone            string   `json:"zone,omitempty"`
	Realm           string   `doc:"What a person signs in to the server by at the discovery's page, beside their name and password: the realm the host's account there gave it; empty at a discovery with none (D297)" json:"realm,omitempty"` //nolint:lll // one struct tag
	// ClaimCode is what the owner types at the discovery to claim the core, while it is pending.
	ClaimCode          string               `json:"claimCode,omitempty"`
	ClaimExpiresAt     int64                `doc:"Unix milliseconds; 0 when there is no code" json:"claimExpiresAt"`
	Account            string               `doc:"The address of the account the server is enrolled with, at a discovery with accounts (D310 §5)" json:"account,omitempty"`                 //nolint:lll // one struct tag
	RemovedAt          int64                `doc:"Unix milliseconds the server was removed from its account, another enrolled in its place (D312 §15); 0 when it was not" json:"removedAt"` //nolint:lll // one struct tag
	ReachableFrom      string               `doc:"As the probe found; absent before it ran" enum:"public_ipv4,public_ipv4_other_port,ipv6,nowhere" json:"reachableFrom,omitempty"`          //nolint:lll // one struct tag
	Published          []string             `doc:"The addresses the discovery publishes" json:"published"`
	DeadEndsLastWeek   int                  `doc:"How many times a client found no path to the server in the last week" json:"deadEndsLastWeek"` //nolint:lll // one struct tag
	Probe              []DiscoveryProbeBody `json:"probe"`
	LastHeartbeatAt    int64                `doc:"Unix milliseconds; 0 before the first" json:"lastHeartbeatAt"`
	Problem            string               `doc:"Why the last call to the discovery failed; empty when it did not" json:"problem,omitempty"`                                                                                         //nolint:lll // one struct tag
	DeprecatedUntil    string               `doc:"When the discovery stops speaking this core's protocol, when it said so" json:"deprecatedUntil,omitempty"`                                                                          //nolint:lll // one struct tag
	ClientOrigin       string               `doc:"Where the discovery serves its client, which the server's links lead to" json:"clientOrigin,omitempty"`                                                                             //nolint:lll // one struct tag
	HomeAddresses      []string             `doc:"The server's addresses at home, which reach it directly in the house (D292 §6); none until the discovery publishes it" json:"homeAddresses"`                                        //nolint:lll // one struct tag
	IsHomeAddressReady bool                 `doc:"Whether the certificate the home addresses need is held: until it is, a browser at home reaches the server the long way, which gives no admin (D297 §6)" json:"isHomeAddressReady"` //nolint:lll // one struct tag
	Fingerprint        string               `doc:"The identity key's fingerprint as a person reads it, in safe32 groups; links carry the same digest" json:"fingerprint"`                                                             //nolint:lll // one struct tag
	PortMapping        PortMappingBody      `json:"portMapping"`
	// WebRTCPaths are how many WebRTC connections took each path since the core started (D274 §3).
	WebRTCPaths []WebRTCPathBody `json:"webrtcPaths"`
	// HouseNetworks are the house's public IPv6 networks the discovery recognises devices at home by
	// (D356 §10).
	HouseNetworks []HouseNetworkBody `json:"houseNetworks"`
}

// HouseNetworkBody is one of the house's public IPv6 /64s, as the core tells its discovery: how it
// was had, and for a learnt one the device in the house that last witnessed it, and when (D356 §10).
type HouseNetworkBody struct {
	Network     string `doc:"A public IPv6 /64, \"2a0f:1000:1:2::/64\"" json:"network"`
	Source      string `doc:"Learnt from a device that reached the server directly at home, or named in the config" enum:"learnt,configured" json:"source"` //nolint:lll // one struct tag
	Device      string `doc:"A learnt network's: the address in the house of the device that last witnessed it" json:"device,omitempty"`                    //nolint:lll // one struct tag
	WitnessedAt int64  `doc:"Unix milliseconds a device last witnessed a learnt network; 0 for a configured one" json:"witnessedAt"`                        //nolint:lll // one struct tag
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
	Path  string `enum:"direct_ipv6,direct_ipv4,punched_ipv6,punched_ipv4,hairpinned_ipv4,relayed,failed" json:"path"`
	Count int    `json:"count"`
}

// DiscoveryStatusOutput is the core's standing with its discovery.
type DiscoveryStatusOutput struct {
	Body DiscoveryStatusBody
}

// IdentifyDiscoveryInput is an admin asking what a discovery's issuer shows, before enrolling.
type IdentifyDiscoveryInput struct {
	AuthorizationHeader
	Body struct {
		Issuer string `doc:"The discovery's issuer, the name typed" json:"issuer" maxLength:"2048" minLength:"1"`
	}
}

// IdentifiedDiscoveryOutput is a discovery as its issuer shows it, for the admin to compare its
// identity with what its operator shows (the root key audit's §3.8).
type IdentifiedDiscoveryOutput struct {
	Body struct {
		Issuer      string `json:"issuer"`
		Identity    string `doc:"The discovery's identity, od1-..., to compare with what its operator shows" json:"identity"` //nolint:lll // one struct tag
		Name        string `json:"name"`
		IsSuggested bool   `doc:"Whether it is the discovery this build suggests, trusted from the root this build holds" json:"isSuggested"` //nolint:lll // one struct tag
	}
}

// ConnectDiscoveryInput is an admin connecting the core to a discovery by its issuer, with their
// password: which discovery a core trusts is as weighty as a claim (D391 §3). A discovery not
// suggested is given with its identity, compared with what its operator shows (D393 §4).
type ConnectDiscoveryInput struct {
	AuthorizationHeader
	Body struct {
		Identity string `doc:"The discovery's identity, od1-...; needed for any but the suggested one" json:"identity,omitempty" maxLength:"128"`              //nolint:lll // one struct tag
		Issuer   string `doc:"The discovery's issuer, the name typed: \"discovery.example\" or its https origin" json:"issuer" maxLength:"2048" minLength:"1"` //nolint:lll // one struct tag
		Password string `doc:"The admin's own password" json:"password" maxLength:"1024" minLength:"1"`                                                        //nolint:lll // one struct tag
	}
}

// ClaimDiscoveryInput is an admin entering the key the household's account made.
type ClaimDiscoveryInput struct {
	AuthorizationHeader
	Body struct {
		Key string `doc:"The key from the household's account: 16 letters and digits, as typed" json:"key" maxLength:"64" minLength:"1"` //nolint:lll // one struct tag
		//nolint:lll // one struct tag
		Password string `doc:"The admin's own password, from which the core makes their realm key once it has the account's realm, so they sign in from away (D297 §3)" json:"password" maxLength:"1024" minLength:"1"`
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
		OperationID: "identifyDiscovery", Method: http.MethodPost, Path: discoveryPath + "/identify",
		Tags:    []string{tagRunningIt},
		Summary: "What a discovery's issuer shows: its identity and name, to compare before enrolling (admin; D393 §4)",
	}, server.identifyDiscovery)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "connectDiscovery", Method: http.MethodPost, Path: discoveryPath,
		Tags:    []string{tagRunningIt},
		Summary: "Enrol the core with a discovery, which answers with the code its owner claims it by (admin; D273)",
	}, server.connectDiscovery)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "claimDiscovery", Method: http.MethodPost, Path: discoveryPath + "/claim",
		Tags:    []string{tagRunningIt},
		Summary: "Enrol the core with its household's account by the key the account made (admin; D310 §5)",
	}, server.claimDiscovery)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "leaveDiscovery", Method: http.MethodDelete, Path: discoveryPath,
		Tags: []string{tagRunningIt}, DefaultStatus: http.StatusNoContent,
		Summary: "Take the core out of its discovery (admin; D273)",
	}, server.leaveDiscovery)
	server.registerUpdate()
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

// identifyDiscovery is what the issuer the admin typed shows: its identity and name, its chain and
// document checked; a name that is no issuer, or the suggested one in a build holding none of its
// root, is refused naming the field, and a discovery that does not answer is the failing leg.
func (server *Server) identifyDiscovery(
	ctx context.Context, input *IdentifyDiscoveryInput,
) (*IdentifiedDiscoveryOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	identified, err := dependencies.Discovery.Identify(ctx, input.Body.Issuer)
	if refused := issuerRefusal(err); refused != nil {
		return nil, refused
	}
	if err != nil {
		return nil, failingLeg(err)
	}
	output := &IdentifiedDiscoveryOutput{}
	output.Body.Issuer, output.Body.Identity = identified.Issuer, identified.Identity
	output.Body.Name, output.Body.IsSuggested = identified.Name, identified.IsSuggested
	return output, nil
}

// issuerRefusal is err refused naming the field it is about, nil for any other error.
func issuerRefusal(err error) error {
	var confirm discovery.IdentityToConfirmError
	switch {
	case errors.Is(err, addressdoc.ErrNotAnIssuer), errors.Is(err, discovery.ErrSuggestedHasNoGenesis):
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.issuer", err.Error())
	case errors.As(err, &confirm), errors.Is(err, discovery.ErrNotTheIdentity):
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.identity", err.Error())
	}
	return nil
}

// connectDiscovery enrols the core with the discovery the admin named by its issuer, once their
// password is checked: a wrong password, a name that is no issuer, the suggested issuer in a build
// that holds none of its root, and a discovery whose identity was not given or is another are refused
// naming their fields; a core already enrolled is a conflict, and a discovery that does not answer, or
// whose chain or document does not hold, is the failing leg.
func (server *Server) connectDiscovery(
	ctx context.Context, input *ConnectDiscoveryInput,
) (*DiscoveryStatusOutput, error) {
	dependencies, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if checkErr := dependencies.Auth.CheckPassword(ctx, principal.UserID, input.Body.Password); checkErr != nil {
		if errors.Is(checkErr, auth.ErrWrongPassword) {
			return nil, invalidRequestAt(http.StatusUnprocessableEntity, "body.password", "that is not your password")
		}
		return nil, fmt.Errorf("check the admin's password: %w", checkErr)
	}
	status, err := dependencies.Discovery.Connect(ctx, input.Body.Issuer, input.Body.Identity)
	if refused := issuerRefusal(err); refused != nil {
		return nil, refused
	}
	return server.discoveryAnswer(dependencies, status, conflictingAs(err, discovery.ErrAlreadyEnrolled))
}

// claimDiscovery sends the key the household's account made, once the admin's password is checked,
// and then makes the admin's realm key from it: a wrong password or key is refused naming its field,
// a core not waiting to be claimed is a conflict, and a discovery that does not answer is the failing
// leg.
func (server *Server) claimDiscovery(ctx context.Context, input *ClaimDiscoveryInput) (*DiscoveryStatusOutput, error) {
	dependencies, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	// The password first, so a mistyped one spends no key.
	if checkErr := dependencies.Auth.CheckPassword(ctx, principal.UserID, input.Body.Password); checkErr != nil {
		if errors.Is(checkErr, auth.ErrWrongPassword) {
			return nil, invalidRequestAt(http.StatusUnprocessableEntity, "body.password", "that is not your password")
		}
		return nil, fmt.Errorf("check the admin's password: %w", checkErr)
	}
	status, err := dependencies.Discovery.Claim(ctx, input.Body.Key)
	if errors.Is(err, discovery.ErrWrongKey) {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "body.key", err.Error())
	}
	if err == nil {
		// The claim gave the core its account's realm: the admin's key for it, from the password.
		err = dependencies.Auth.MakeRealmKey(ctx, principal.UserID, input.Body.Password)
	}
	return server.discoveryAnswer(dependencies, status, conflictingAs(err, discovery.ErrNotPending))
}

// conflictingAs is err as the API refuses it: conflicting is a conflict; any other error is the
// discovery failing to answer, left as it is.
func conflictingAs(err, conflicting error) error {
	if errors.Is(err, conflicting) {
		return newError(http.StatusConflict, CodeConflict, err.Error())
	}
	return err
}

// discoveryAnswer answers an admin's call to the core's discovery: a refusal conflictingAs made as it
// is, a discovery that does not answer as the failing leg, and otherwise the core's standing.
func (server *Server) discoveryAnswer(
	dependencies *Dependencies, status discovery.Status, err error,
) (*DiscoveryStatusOutput, error) {
	if refusal, isRefusal := errors.AsType[*Error](err); isRefusal {
		return nil, refusal
	}
	if err != nil {
		return nil, failingLeg(err)
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
	// Its people's realm keys stay: a key serves only its master realm, and a core that comes back
	// under it is found by them again with no sign-in at home first (D299).
	return &struct{}{}, nil
}

// failingLeg is a discovery that did not answer, or answered what does not hold, as the API refuses it.
func failingLeg(err error) error {
	failure := newDetailedError(http.StatusBadGateway, err.Error(),
		ProviderRefusalDetailBody{Reason: ReasonUnavailable})
	failure.ErrorBody.Error.FailedHop = HopProvider
	return failure
}

// discoveryStatusBody is a status as the API sends it.
func discoveryStatusBody(status discovery.Status) DiscoveryStatusBody {
	body := DiscoveryStatusBody{
		Issuer: status.Issuer, SuggestedIssuer: status.SuggestedIssuer, CoreAPI: status.BaseURL,
		LoaderOrigin: status.LoaderOrigin, Homepage: status.Homepage, RootKeys: append([]string{}, status.RootKeys...),
		RootVersion: status.RootVersion, DocumentProblem: status.DocumentProblem, Identity: status.Identity,
		Name: status.Name, IsSuggested: status.IsSuggested,
		CoreID: status.CoreID, State: string(status.State), Zone: status.Zone,
		Realm:     status.Realm,
		ClaimCode: status.ClaimCode, ReachableFrom: status.ReachableFrom, DeadEndsLastWeek: status.DeadEndsLastWeek,
		Published: append([]string{}, status.Published...), Probe: []DiscoveryProbeBody{},
		Problem: status.Problem, DeprecatedUntil: status.DeprecatedUntil,
		ClientOrigin: status.ClientOrigin, Fingerprint: status.FingerprintText, Account: status.Account,
		HomeAddresses: append([]string{}, status.HomeAddresses...), IsHomeAddressReady: status.IsHomeAddressReady,
	}
	if !status.RemovedAt.IsZero() {
		body.RemovedAt = status.RemovedAt.UnixMilli()
	}
	if !status.ClaimExpiresAt.IsZero() {
		body.ClaimExpiresAt = status.ClaimExpiresAt.UnixMilli()
	}
	if !status.LastHeartbeatAt.IsZero() {
		body.LastHeartbeatAt = status.LastHeartbeatAt.UnixMilli()
	}
	if !status.DocumentStaleSince.IsZero() {
		body.DocumentStaleAt = status.DocumentStaleSince.UnixMilli()
	}
	if !status.DocumentIssuedAt.IsZero() {
		body.DocumentAt = status.DocumentIssuedAt.UnixMilli()
	}
	for _, probe := range status.Probe {
		body.Probe = append(body.Probe, DiscoveryProbeBody{
			Address: probe.Address, Port: probe.Port, Kind: probe.Kind, Result: probe.Result,
		})
	}
	body.HouseNetworks = houseNetworksBody(status.HouseNetworks)
	return body
}

// houseNetworksBody are the house's networks as the API sends them.
func houseNetworksBody(networks []discovery.HouseNetwork) []HouseNetworkBody {
	body := make([]HouseNetworkBody, 0, len(networks))
	for _, each := range networks {
		item := HouseNetworkBody{Network: each.Network.String(), Source: each.Source}
		if each.Device.IsValid() {
			item.Device = each.Device.String()
		}
		if !each.WitnessedAt.IsZero() {
			item.WitnessedAt = each.WitnessedAt.UnixMilli()
		}
		body = append(body, item)
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
