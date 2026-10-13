// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto

// The headers every call carries and every answer may: the protocol's version, and the date after
// which a deprecated version is no longer answered (D273 §6, §7).
const (
	ProtocolHeader   = "Orpheum-Discovery-Protocol"
	DeprecatedHeader = "Orpheum-Discovery-Deprecated"
)

// The protocol this build speaks. Changes are additive wherever they can be and raise the minor;
// a breaking one raises the major, which the paths carry too (/core/v1; the protocol's §5, D273 §6).
const (
	ProtocolMajor = 1
	ProtocolMinor = 0
)

// ProtocolVersion is what every call says in ProtocolHeader, "major.minor": the one version signal,
// which no body repeats (the protocol's §5; C13).
const ProtocolVersion = "1.0"

// The paths under a discovery's base URL.
const (
	PathChallenge = "/challenge"
	PathEnrol     = "/enrol"
	PathSession   = "/session"
	PathStatus    = "/status"
	PathAddresses = "/addresses"
	PathSignal    = "/signal"
	PathCore      = "/core"
	// PathACMEChallenge is where the core sets and clears its certificate's DNS-01 record, which
	// the discovery holds for it (the spec's §3.7).
	PathACMEChallenge = "/acme-challenge"
)

// The purposes a signature is made for: the protocol, the purpose and the nonce are what is signed,
// so a signature made for one purpose cannot serve another.
const (
	PurposeEnrol   = "enrol"
	PurposeSession = "session"
)

// ChallengeRequest asks for a nonce to sign.
type ChallengeRequest struct {
	PublicKey []byte `json:"publicKey"`
}

// ChallengeResponse is the nonce to sign.
type ChallengeResponse struct {
	Nonce string `json:"nonce"`
}

// EnrolRequest makes the core a pending one: its public key, and its signature over the nonce for
// PurposeEnrol, proving it holds the key.
type EnrolRequest struct {
	PublicKey []byte `json:"publicKey"`
	Nonce     string `json:"nonce"`
	Signature []byte `json:"signature"`
}

// EnrolResponse is the pending core's id and the code its owner claims it by, until ClaimExpiresAt
// (Unix milliseconds).
type EnrolResponse struct {
	CoreID         string `json:"coreId"`
	ClaimCode      string `json:"claimCode"`
	ClaimExpiresAt int64  `json:"claimExpiresAt"`
}

// SessionRequest opens a session: the core's id, the instance asking, and its signature over the
// nonce for PurposeSession. InstanceID is random, made at each start of the core and never kept, so
// a discovery tells a restarted core, or a copy of it, from the one it holds (D293 §1).
type SessionRequest struct {
	CoreID     string `json:"coreId"`
	InstanceID string `json:"instanceId"`
	Nonce      string `json:"nonce"`
	Signature  []byte `json:"signature"`
}

// SessionResponse is the bearer token the core's other calls carry, for ExpiresIn seconds: a
// duration, which the core counts on its own clock from when it asked, so a core whose clock is wrong
// neither renews on every step nor sends expired tokens (the protocol's §2.3; C11).
type SessionResponse struct {
	Token     string `json:"token"`
	ExpiresIn int64  `json:"expiresIn"`
}

// Refusal is the body of a refusal the core acts on rather than retrying as it would any failure:
// Code says which, Message says it for the admin (the protocol's §2.2, §2.3; C4, C12).
type Refusal struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// The refusals a core acts on.
const (
	// RefusalEnrolAgain is a discovery that holds no core of this id and key: the core enrols again
	// and shows the code it is given.
	RefusalEnrolAgain = "enrol_again"
	// RefusalAnotherInstance is another instance of this core, still answering, holding the
	// discovery's connection: this one is a copy, which says so and asks again later (D293 §1).
	RefusalAnotherInstance = "another_instance"
	// RefusalNotClaimed is a call only a claimed core makes, from a core not yet claimed.
	RefusalNotClaimed = "not_claimed"
	// RefusalVersion is a call in a major the discovery does not serve, or with no version at all:
	// its message names the versions it serves (the protocol's §5).
	RefusalVersion = "unsupported_version"
)

// State is where a core stands with its discovery.
type State string

// The states a core is in.
const (
	StatePending  State = "pending"
	StateClaimed  State = "claimed"
	StateGrace    State = "grace"
	StateFrozen   State = "frozen"
	StateReleased State = "released"
	StateDisabled State = "disabled"
)

// StatusResponse is the core's state, its zone once claimed, and its claim code while pending.
type StatusResponse struct {
	State          State  `json:"state"`
	Zone           string `json:"zone,omitempty"`
	ClaimCode      string `json:"claimCode,omitempty"`
	ClaimExpiresAt int64  `json:"claimExpiresAt,omitempty"`
	// ICEServers are the STUN servers the core gathers its WebRTC candidates through, so its
	// answers carry the public address its NAT gives it (D274): the discovery's own.
	ICEServers []ICEServer `json:"iceServers,omitempty"`
	// ClientOrigin is where the discovery serves its client, "https://app.example": the origin the
	// core's links lead to, and its CORS allows (the protocol's §3.4; D293 §4).
	ClientOrigin string `json:"clientOrigin,omitempty"`
}

// The sources of a candidate address.
const (
	SourceInterface  = "interface"
	SourceMapped     = "mapped"
	SourceConfigured = "configured"
)

// ScopeLAN marks a candidate only a client at home can reach, published unprobed.
const ScopeLAN = "lan"

// Candidate is an address and port the core may be reached at.
type Candidate struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Source  string `json:"source"`
	// Method is how a mapped candidate was obtained: upnp, natpmp or pcp.
	Method string `json:"method,omitempty"`
	Scope  string `json:"scope,omitempty"`
}

// ICEServer is a TURN server the owner runs, passed to the core's clients: never the discovery's.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// AddressesRequest is the heartbeat: the core's version, its API's major, its candidates and its
// owner's TURN servers.
type AddressesRequest struct {
	OrpheumVersion string      `json:"orpheumVersion"`
	APIMajor       int         `json:"apiMajor"`
	Candidates     []Candidate `json:"candidates"`
	TURN           []ICEServer `json:"turn,omitempty"`
}

// ProbeResult is what the discovery's outside probe found of one candidate, or of "webrtc".
type ProbeResult struct {
	Candidate string `json:"candidate"`
	Result    string `json:"result"`
}

// Where a core can be reached from, as the probe found.
const (
	ReachableEverywhere   = "everywhere"
	ReachableIPv6OrWebRTC = "ipv6_or_webrtc"
	ReachableNowhere      = "nowhere"
)

// AddressesResponse is the heartbeat's answer: the names published, the probe's findings, and where
// the core can be reached from.
type AddressesResponse struct {
	Published     []string      `json:"published"`
	Probe         []ProbeResult `json:"probe"`
	ReachableFrom string        `json:"reachableFrom"`
}

// The kinds of message on the signalling connection.
const (
	SignalOffer   = "offer"
	SignalAnswer  = "answer"
	SignalRefusal = "refusal"
)

// SignalMessage is one message on the signalling connection: an offer the discovery passes on, and
// the core's answer to it or refusal of it. An answer's Signature is the core's identity key over
// proof.Answer: the core, the offer, the offer's fingerprint and the answer's exact SDP, which the
// client checks before it takes the answer (the protocol's §2.5 item 4).
type SignalMessage struct {
	Type      string `json:"type"`
	OfferID   string `json:"offerId"`
	SDP       string `json:"sdp,omitempty"`
	Signature []byte `json:"signature,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// ACMEChallengeRequest sets the TXT record of name, which is _acme-challenge.<coreId>.<zone>, to
// value, the 43-character base64url digest DNS-01 asks for (RFC 8555 §8.4; S18). The record is
// cleared by DELETE PathACMEChallenge/<name>, the name in the path, since a DELETE defines no body
// (the protocol's §2.7; C16).
type ACMEChallengeRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
