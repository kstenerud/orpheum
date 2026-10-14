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
	// PathLocateHashes and PathLocators are where the core registers what a discovery finds it by:
	// the hashes of its links' secrets, and of its devices' locators, each the full set (the
	// protocol's §3.1).
	PathLocateHashes = "/locate-hashes"
	PathLocators     = "/locators"
	// PathACMEChallenge is where the core sets and clears its certificate's DNS-01 record, which
	// the discovery holds for it (the spec's §3.7).
	PathACMEChallenge = "/acme-challenge"
	// PathACMEAccount is where the core reports its ACME account, before it orders a certificate, so
	// the discovery's CAA names that account alone (the protocol's §2.7; RFC 8657).
	PathACMEAccount = "/acme-account"
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

// ProbeResult is what the discovery's outside probe found of one candidate, each part a field of its
// own (the protocol's §2.6; C16): the address and port it tried, what it tried there, and how it went.
type ProbeResult struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Kind    string `json:"kind"`
	// Result is ProbeReached, or the hop a failure stopped at, as the core's own reachability check
	// names them (D152): "refused", "timed_out", "tls", "wrong_core".
	Result string `json:"result"`
}

// What a probe tried at a candidate.
const (
	ProbeHTTPS = "https"
	ProbeTURN  = "turn"
)

// ProbeReached is a probe that got through, and found the core.
const ProbeReached = "reached"

// Where a core can be reached from, named by what the probe found to work (the protocol's §2.6; C10).
const (
	// ReachablePublicIPv4 is a public IPv4 address on 443: every network reaches it.
	ReachablePublicIPv4 = "public_ipv4"
	// ReachablePublicIPv4OtherPort is a public IPv4 address on another port, which networks that allow
	// only 80 and 443 do not reach.
	ReachablePublicIPv4OtherPort = "public_ipv4_other_port"
	// ReachableIPv6 is IPv6 alone: a client without it connects over WebRTC, or not at all.
	ReachableIPv6 = "ipv6"
	// ReachableNowhere is no direct path at all: WebRTC alone.
	ReachableNowhere = "nowhere"
)

// AddressesResponse is the heartbeat's answer: the names published, the probe's findings, and where
// the core can be reached from.
type AddressesResponse struct {
	Published     []string      `json:"published"`
	Probe         []ProbeResult `json:"probe"`
	ReachableFrom string        `json:"reachableFrom"`
	// DeadEndsLastWeek is how many times a client found no path to the core in the last seven days,
	// counted with no device identifier, for the core's System page (the protocol's §3.6; D293 §7).
	DeadEndsLastWeek int `json:"deadEndsLastWeek"`
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
//
// An offer also carries the hash the client found the core by, a locator's (link.LocatorHash) or, for a
// link's bootstrap, the link's (link.LocateHash), and its proof (proof.Offer), which the core checks
// under the key it keeps for that hash before it answers: the discovery's check of the hash is only a
// first filter (the protocol's §2.5 item 3; A2, A9).
type SignalMessage struct {
	Type        string `json:"type"`
	OfferID     string `json:"offerId"`
	SDP         string `json:"sdp,omitempty"`
	LocatorHash string `json:"locatorHash,omitempty"`
	Proof       string `json:"proof,omitempty"`
	Signature   []byte `json:"signature,omitempty"`
	// PublicKey is the core's identity key, on an answer: a client holding only a link's fingerprint
	// checks that the key's SHA-256 is it, then the signature with the key, so it takes the key on
	// nobody's word (the protocol's §2.5 item 4, §3.5).
	PublicKey []byte `json:"publicKey,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// ACMEAccountRequest is the core's ACME account, as its CA named it, which the discovery's CAA at
// the core's own name allows alone (accounturi, RFC 8657; the protocol's §2.7). A core that makes a
// new account reports it before it orders, and the record is rewritten (G2).
type ACMEAccountRequest struct {
	AccountURI string `json:"accountUri"`
}

// ACMEChallengeRequest sets the TXT record of name, which is _acme-challenge.<coreId>.<zone>, to
// value, the 43-character base64url digest DNS-01 asks for (RFC 8555 §8.4; S18). The record is
// cleared by DELETE PathACMEChallenge/<name>, the name in the path, since a DELETE defines no body
// (the protocol's §2.7; C16).
type ACMEChallengeRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// LocateHash is one link a discovery may find the core by: its secret's hash (link.LocateHash), how
// many seconds it lives from now, and how many times it may locate, none meaning as often as asked
// until it ends (the protocol's §3.1; C3).
type LocateHash struct {
	Hash      string `json:"hash"`
	ExpiresIn int64  `json:"expiresIn"`
	Uses      int    `json:"uses,omitempty"`
}

// LocateHashesRequest is every link a discovery may find the core by; any it held and this does not
// name, it forgets.
type LocateHashesRequest struct {
	Hashes []LocateHash `json:"hashes"`
}

// LocatorsRequest is every device locator a discovery may find the core by, as their hashes
// (link.LocatorHash); any it held and this does not name, it forgets.
type LocatorsRequest struct {
	Hashes []string `json:"hashes"`
}
