// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto

import "strings"

// The client API's paths, under a discovery's <base>/client/v1: what a device holds (a link's hash,
// a locator or a bootstrap token) finds its core, and its offers reach it (the protocol's §3, §4).
const (
	PathLocate  = "/locate"
	PathPaths   = "/paths"
	PathDeadEnd = "/dead-end"
	// PathRealmSignIn finds a core by its realm, for a person who signs in there (ClientRealmRequest).
	PathRealmSignIn = "/realm"
	// PathCores is the start of a core's offer path: OfferPath.
	PathCores = "/cores/"
	// PathClientFloor is read with GET: the client versions the discovery refuses, which its loader
	// runs none of (D308 §6; ClientFloorResponse).
	PathClientFloor = "/client-floor"
)

// ClientFloorResponse is the discovery's floor of client versions: each one named is refused, as one
// with a hole its operator knows of. None for a discovery that refuses none.
type ClientFloorResponse struct {
	RefusedVersions []string `json:"refusedVersions"`
}

// OfferPath is where a client posts its offers for the core coreID: /cores/<coreId>/offer.
func OfferPath(coreID string) string {
	return PathCores + coreID + "/offer"
}

// LocateRequest is a link's hash (link.LocateHash of its secret), which the discovery finds the core
// by and spends by its use policy; the secret itself never leaves the device but to the core.
type LocateRequest struct {
	Hash string `json:"hash"`
}

// LocateResponse is the core's paths, and a bootstrap token: good for /paths and offers for a few
// minutes, never renewed, so the device signs in or registers over a path in that time (the
// protocol's §3.1; X10).
type LocateResponse struct {
	PathsResponse
	BootstrapToken string `json:"bootstrapToken"`
}

// ClientRealmRequest is a person's realm sign-in: the realm, who it is for (proof.RealmPerson of
// the realm and their name) and their proof (proof.RealmAsk under the key their password makes). The
// discovery cannot check the proof; the realm's core does (D297 §2, §3).
type ClientRealmRequest struct {
	Realm  string `json:"realm"`
	Person string `json:"person"`
	Ask    string `json:"ask"`
}

// ClientRealmResponse is what a realm sign-in the core took gives: what a link's locate gives, the
// core's identity key, when it was claimed, and the core's tag (proof.RealmTag) over its id and that
// key, which the page checks under the person's key before it believes the key is the core's. Any
// part of the sign-in wrong, the realm included, answers RefusalNotFound alone, after the same wait
// (D297 §2).
type ClientRealmResponse struct {
	LocateResponse
	PublicKey []byte `json:"publicKey"`
	Tag       string `json:"tag"`
	// ClaimedAt is when the realm's core was claimed for its account, Unix ms, 0 when the discovery
	// does not say: a page that remembers another core for the realm says since when it changed
	// (D312 §2). The tag does not cover it; it only dates the warning.
	ClaimedAt int64 `json:"claimedAt,omitempty"`
}

// PathsRequest names the core by what the device holds: its locator, which the core handed it, or the
// bootstrap token /locate gave it.
type PathsRequest struct {
	Locator        string `json:"locator,omitempty"`
	BootstrapToken string `json:"bootstrapToken,omitempty"`
}

// CoreState is how a core stands as its discovery knows it, so a client tells a household member the
// cause when it is the core's (the protocol's §3.2; D293 §6).
type CoreState string

// The states a client is told.
const (
	CoreOnline    CoreState = "online"
	CoreOffline   CoreState = "offline"
	CorePaused    CoreState = "paused"
	CoreRestoring CoreState = "restoring"
)

// The kinds of path, ranked so by a client: the LAN, IPv6, IPv4, WebRTC (the protocol's §3.3).
const (
	PathKindLAN    = "lan"
	PathKindIPv6   = "ipv6"
	PathKindIPv4   = "ipv4"
	PathKindWebRTC = "webrtc"
)

// Path is one way to the core: a URL for a direct path, or for WebRTC the path to post offers to.
type Path struct {
	Kind  string `json:"kind"`
	URL   string `json:"url,omitempty"`
	Offer string `json:"offer,omitempty"`
}

// PathsResponse is how to reach the core: its id, its API's major, its state, the client's own
// network prefix as the discovery sees it (the key under which the client remembers what worked,
// D293 §8), the paths, and the ICE servers for WebRTC.
type PathsResponse struct {
	CoreID   string    `json:"coreId"`
	APIMajor int       `json:"apiMajor"`
	State    CoreState `json:"state"`
	// OfflineSince is when the core went offline, Unix ms; 0 while it is online.
	OfflineSince int64       `json:"offlineSince,omitempty"`
	ClientPrefix string      `json:"clientPrefix"`
	Paths        []Path      `json:"paths"`
	ICEServers   []ICEServer `json:"iceServers"`
}

// DeadEndRequest is a client telling the discovery that no path reached the core, by what the
// device holds, and why as the page judged it (D398 §2); the discovery counts it for the core's week
// and keeps nothing of the device. The reason's fields are all optional: a page before D398 sends
// none, and a discovery before it ignores them. They carry no address, name or URL.
type DeadEndRequest struct {
	PathsRequest
	// IPv6 is whether the client's network has IPv6, as the page judged it: one of the HasIPv6 values.
	IPv6 string `json:"ipv6,omitempty"`
	// Tried is each way the page tried, by kind, in the order tried; at most MaxDeadEndTries are read.
	Tried []DeadEndTry `json:"tried,omitempty"`
	// Cause is what the page told its person: one of the DeadEndCause values.
	Cause string `json:"cause,omitempty"`
}

// DeadEndTry is one way a page tried at a dead end: its kind (PathKindLAN, PathKindIPv6, PathKindIPv4,
// PathKindWebRTC) and how it ended, as the web client's DirectEnding names it ("noAnswer",
// "unreachable", "refused", "notWaited", "answered"), or "failed" for WebRTC.
type DeadEndTry struct {
	Kind  string `json:"kind"`
	Ended string `json:"ended"`
}

// The HasIPv6 values: the page knows its network has IPv6, knows it has none, or cannot tell (D398 §1).
const (
	HasIPv6Yes     = "yes"
	HasIPv6No      = "no"
	HasIPv6Unknown = "unknown"
)

// The DeadEndCause values: the page said this network has no IPv6 and the core no IPv4 way in, or
// gave its general words (D398 §3).
const (
	DeadEndCauseNoIPv6  = "noIpv6"
	DeadEndCauseUnknown = "unknown"
)

// MaxDeadEndTries is the most ways a dead end's report is read for: a page tries its core's few
// names and WebRTC, so more is not a page's. A justified constant.
const MaxDeadEndTries = 8

// RefusalNotFound is a link's hash, a locator or a bootstrap token the discovery finds no core by:
// never registered, used up, expired or dropped. Which, it does not say.
const RefusalNotFound = "not_found"

// MaxOffersAtOnce is how many offers one request may carry: a client races one connection, then opens
// the rest of its four (the protocol's §2.5; C6).
const MaxOffersAtOnce = 4

// ClientOffer is one WebRTC offer and its proof: proof.Offer over its DTLS fingerprint, under the
// client's session's key or its link's.
type ClientOffer struct {
	SDP   string `json:"sdp"`
	Proof string `json:"proof"`
}

// OfferRequest is a device's offers to the core, by the locator or bootstrap token it holds.
type OfferRequest struct {
	PathsRequest
	Offers []ClientOffer `json:"offers"`
}

// OfferAnswer is the core's answer to one offer, in the offers' order: its exact SDP and its signature
// (proof.Answer), which the client checks before it takes it, or why it refused.
type OfferAnswer struct {
	OfferID   string `json:"offerId"`
	SDP       string `json:"sdp,omitempty"`
	Signature []byte `json:"signature,omitempty"`
	// PublicKey is the core's identity key, which the client checks against the fingerprint it holds.
	PublicKey []byte `json:"publicKey,omitempty"`
	Refusal   string `json:"refusal,omitempty"`
}

// OfferResponse is the core's answers, one per offer.
type OfferResponse struct {
	Answers []OfferAnswer `json:"answers"`
}

// The bounds of a realm's parts: a master realm, a host's account's, 5 to 32 lowercase letters,
// digits and inner hyphens, few enough to guess; a subrealm, one of the account's cores', 1 to 32 of
// the same. A realm is the master alone, or the master, a dot and a subrealm; no part holds a dot, so
// none is mistaken for another's (D297 §1, the owner, 2026-10-15).
const (
	masterRealmShortest = 5
	subrealmShortest    = 1
	realmPartLongest    = 32
)

// IsRealm is whether realm is one as it is kept, in lower case: "karlnas" or "karlnas.test".
func IsRealm(realm string) bool {
	master, sub, hasSub := strings.Cut(realm, ".")
	return isRealmPart(master, masterRealmShortest) && (!hasSub || isRealmPart(sub, subrealmShortest))
}

// isRealmPart is whether part is one realm's part: shortest to realmPartLongest lowercase letters,
// digits and hyphens, neither starting nor ending with a hyphen.
func isRealmPart(part string, shortest int) bool {
	if len(part) < shortest || len(part) > realmPartLongest || part[0] == '-' || part[len(part)-1] == '-' {
		return false
	}
	for _, each := range part {
		if (each < 'a' || each > 'z') && (each < '0' || each > '9') && each != '-' {
			return false
		}
	}
	return true
}
