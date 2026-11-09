// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// Verdict is what the core tells a client on its NegotiationLabel channel once the connection is up
// and the channel open: that the connection is held, or why it is turned away. The core knows a
// punched or relayed client's address only once ICE has connected, so it cannot refuse the offer;
// closing the connection alone left the page to guess, and it said "can't be reached from this
// network", which read as the device's network failing (the owner's Fire TV, 2026-10-25).
type Verdict struct {
	IsHeld bool `json:"isHeld,omitempty"`
	// Refused is why the connection is turned away: RefusedNetworkFull.
	Refused string `json:"refused,omitempty"`
}

// RefusedNetworkFull is a connection turned away because its client's network holds its share of
// the core's connections already (perSourceLimit).
const RefusedNetworkFull = "network-full"

// turnedAwayClosesWithin is how long a connection turned away is kept for its verdict to arrive
// before the core closes it, when its client has not closed it first, as a client that read the
// verdict does. Closed at once, the verdict sent just before it never reached the page.
const turnedAwayClosesWithin = 2 * time.Second

// verdictTeller sends a connection's verdict on its negotiation channel once both are there: the
// verdict, decided as the connection connects, and the channel, open once SCTP is.
type verdictTeller struct {
	mutex   sync.Mutex
	channel *webrtc.DataChannel
	isOpen  bool
	decided *Verdict
	isSent  bool
}

// heard takes the client's negotiation channel, and tells it the verdict once it opens.
func (teller *verdictTeller) heard(channel *webrtc.DataChannel) {
	teller.mutex.Lock()
	teller.channel = channel
	teller.mutex.Unlock()
	channel.OnOpen(func() {
		teller.mutex.Lock()
		teller.isOpen = true
		teller.mutex.Unlock()
		teller.tell()
	})
}

// decide is the connection's verdict, told once the channel is open.
func (teller *verdictTeller) decide(verdict Verdict) {
	teller.mutex.Lock()
	teller.decided = &verdict
	teller.mutex.Unlock()
	teller.tell()
}

// isRefused is whether the connection was turned away, so it serves no request.
func (teller *verdictTeller) isRefused() bool {
	teller.mutex.Lock()
	defer teller.mutex.Unlock()
	return teller.decided != nil && teller.decided.Refused != ""
}

// tell sends the verdict, once, when it is decided and the channel open.
func (teller *verdictTeller) tell() {
	teller.mutex.Lock()
	if teller.isSent || !teller.isOpen || teller.decided == nil || teller.channel == nil {
		teller.mutex.Unlock()
		return
	}
	teller.isSent = true
	channel, verdict := teller.channel, *teller.decided
	teller.mutex.Unlock()
	text, err := json.Marshal(verdict)
	if err == nil {
		_ = channel.SendText(string(text)) //nolint:errcheck // a connection gone first needs no verdict
	}
}
