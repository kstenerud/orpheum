// SPDX-License-Identifier: GPL-3.0-or-later

package discoveryproto

// The headers every call carries and every answer may: the protocol's version, and the date after
// which a deprecated version is no longer answered (D273 §6, §7).
const (
	ProtocolHeader   = "Orpheum-Discovery-Protocol"
	DeprecatedHeader = "Orpheum-Discovery-Deprecated"
)

// ProtocolVersion is the major version of the protocol this build speaks. Changes are additive
// wherever they can be; a breaking one raises it (D273 §6).
const ProtocolVersion = "1"

// The paths under a discovery's base URL.
const (
	PathChallenge = "/challenge"
	PathEnrol     = "/enrol"
	PathSession   = "/session"
	PathStatus    = "/status"
	PathAddresses = "/addresses"
	PathSignal    = "/signal"
	PathCore      = "/core"
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

// EnrolRequest makes the core a pending one: its public key, its protocol, and its signature over
// the nonce for PurposeEnrol, proving it holds the key.
type EnrolRequest struct {
	PublicKey []byte `json:"publicKey"`
	Protocol  string `json:"protocol"`
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

// SessionRequest opens a session: the core's id and its signature over the nonce for PurposeSession.
type SessionRequest struct {
	CoreID    string `json:"coreId"`
	Nonce     string `json:"nonce"`
	Signature []byte `json:"signature"`
}

// SessionResponse is the bearer token the core's other calls carry, until ExpiresAt (Unix ms).
type SessionResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
}

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
	Protocol       string      `json:"protocol"`
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
// the core's answer to it or refusal of it.
type SignalMessage struct {
	Type    string `json:"type"`
	OfferID string `json:"offerId"`
	SDP     string `json:"sdp,omitempty"`
	Reason  string `json:"reason,omitempty"`
}
