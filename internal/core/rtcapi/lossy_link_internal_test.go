// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"io"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/v5/vnet"
	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/internal/testutil"
)

// pageSilentAtMost is the page's silentAtMostMs (web/packages/sdk's rtc-request.ts): a request that
// hears nothing for it is given up.
const pageSilentAtMost = 40 * time.Second

// lossFor is how long the lossy link, the owner's train (2026-11-13), loses every full-size packet
// the core sends: past pion's own fifth resend (1+2+4+8+16 = 31 s), whose sixth then waits 32 s
// more, past pageSilentAtMost.
const lossFor = 32 * time.Second

// fullSizeBytes is the least a datagram the lossy link loses holds: an SCTP packet of data is about
// sctpPacketBytes, and a STUN check, a SACK or a heartbeat is a few dozen bytes.
const fullSizeBytes = 600

// lossyResponseBytes is the response the core sends as the loss begins: a few packets.
const lossyResponseBytes = 16 << 10

// On a link losing every full-size packet the core sends for lossFor, a response sent as the loss
// begins still reaches the page before it gives the request up, and the core's send window stays
// sctpLeastWindowBytes wide through the loss: its resends wait at most sctpResendCeiling.
// Not parallel: vnet counts its MAC addresses in a package variable it does not lock
// (transport v5.1.1's net.go, newMACAddress), so two virtual networks made at once race.
func TestSettings_AResponseOverALossyLinkArrivesBeforeThePageGivesUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		arrived, windowBytes := sendOverLossyLink(t, settings(nil))
		if !arrived {
			t.Errorf("the response did not arrive within the page's %v", pageSilentAtMost)
		}
		if windowBytes < sctpLeastWindowBytes {
			t.Errorf("the core's send window through the loss = %d bytes; want at least %d",
				windowBytes, sctpLeastWindowBytes)
		}
	})
}

// The same link with pion's own settings goes quiet past the page's limit, its window one packet: the
// harness shows the loss the core's settings answer.
// Not parallel, as the test above.
func TestSettings_PionsOwnGoQuietOverALossyLinkPastThePagesLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		arrived, windowBytes := sendOverLossyLink(t, webrtc.SettingEngine{})
		if arrived {
			t.Errorf("the response arrived within the page's %v with pion's own settings", pageSilentAtMost)
		}
		if windowBytes != sctpPacketBytes {
			t.Errorf("pion's own send window through the loss = %d bytes; want one packet, %d",
				windowBytes, sctpPacketBytes)
		}
	})
}

// sendOverLossyLink connects a client to a peer with the core's settings over a virtual network,
// has the core send a response as the link starts losing every full-size packet it sends, heals the
// link lossFor later, and says whether the response arrived within pageSilentAtMost of being sent,
// and the core's send window just before the link healed.
func sendOverLossyLink(t *testing.T, core webrtc.SettingEngine) (arrived bool, windowBytes uint32) {
	t.Helper()
	quiet := &logging.DefaultLoggerFactory{Writer: io.Discard}
	router, err := vnet.NewRouter(&vnet.RouterConfig{CIDR: "10.0.0.0/24", LoggerFactory: quiet})
	if err != nil {
		t.Fatal(err)
	}
	coreIP := net.ParseIP("10.0.0.2")
	coreNet := addVirtualNet(t, router, coreIP.String())
	clientNet := addVirtualNet(t, router, "10.0.0.3")
	var lossy atomic.Bool
	router.AddChunkFilter(func(chunk vnet.Chunk) bool {
		from, isUDP := chunk.SourceAddr().(*net.UDPAddr)
		return !lossy.Load() || !isUDP || !from.IP.Equal(coreIP) || len(chunk.UserData()) < fullSizeBytes
	})
	if err = router.Start(); err != nil {
		t.Fatal(err)
	}
	defer closeQuietly(t, "the router", router.Stop)

	core.SetNet(coreNet)
	core.LoggerFactory = quiet
	// Each peer closes when the test closes it, never by itself on the other's close_notify (as
	// settings() has it already): see closeOneThenTheOther.
	core.DisableCloseByDTLS(true)
	var client webrtc.SettingEngine
	client.SetNet(clientNet)
	client.LoggerFactory = quiet
	client.DisableCloseByDTLS(true)
	corePeer := newPeer(t, core)
	defer closeQuietly(t, "the core's peer", corePeer.GracefulClose)
	coreHeardTheEnd := make(chan struct{}, 1)
	corePeer.SCTP().OnClose(func(error) {
		select {
		case coreHeardTheEnd <- struct{}{}:
		default:
		}
	})
	clientPeer := newPeer(t, client)
	defer closeOneThenTheOther(t, clientPeer, coreHeardTheEnd)

	opened := make(chan *webrtc.DataChannel, 1)
	corePeer.OnDataChannel(func(channel *webrtc.DataChannel) {
		channel.OnOpen(func() { opened <- channel })
	})
	received := make(chan struct{}, 1)
	channel, err := clientPeer.CreateDataChannel("lossy", nil)
	if err != nil {
		t.Fatal(err)
	}
	channel.OnMessage(func(message webrtc.DataChannelMessage) {
		if len(message.Data) == lossyResponseBytes {
			received <- struct{}{}
		}
	})
	connectPeers(t, clientPeer, corePeer)

	var sender *webrtc.DataChannel
	select {
	case sender = <-opened:
	case <-time.After(10 * time.Second):
		t.Fatal("the channel did not open")
	}
	lossy.Store(true)
	if err = sender.Send(make([]byte, lossyResponseBytes)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(lossFor)
	windowBytes = sendWindowBytes(corePeer)
	lossy.Store(false)
	select {
	case <-received:
		return true, windowBytes
	case <-time.After(pageSilentAtMost - lossFor):
		return false, windowBytes
	}
}

// addVirtualNet adds a network with address ip to router.
func addVirtualNet(t *testing.T, router *vnet.Router, ip string) *vnet.Net {
	t.Helper()
	network, err := vnet.NewNet(&vnet.NetConfig{StaticIPs: []string{ip}})
	if err != nil {
		t.Fatal(err)
	}
	if err = router.AddNet(network); err != nil {
		t.Fatal(err)
	}
	return network
}

// newPeer is a peer connection with settings.
func newPeer(t *testing.T, settings webrtc.SettingEngine) *webrtc.PeerConnection {
	t.Helper()
	peer, err := webrtc.NewAPI(webrtc.WithSettingEngine(settings)).NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	return peer
}

// connectPeers has offerer offer and answerer answer, each with every candidate gathered.
func connectPeers(t *testing.T, offerer, answerer *webrtc.PeerConnection) {
	t.Helper()
	offer, err := offerer.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gathered := webrtc.GatheringCompletePromise(offerer)
	if err = offerer.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	testutil.Receive(t, gathered, "the offerer's candidates")
	if err = answerer.SetRemoteDescription(*offerer.LocalDescription()); err != nil {
		t.Fatal(err)
	}
	answer, err := answerer.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gathered = webrtc.GatheringCompletePromise(answerer)
	if err = answerer.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}
	testutil.Receive(t, gathered, "the answerer's candidates")
	if err = offerer.SetRemoteDescription(*answerer.LocalDescription()); err != nil {
		t.Fatal(err)
	}
}

// sendWindowBytes is peer's SCTP congestion window, in bytes, 0 before its SCTP transport has one.
func sendWindowBytes(peer *webrtc.PeerConnection) uint32 {
	for _, stats := range peer.GetStats() {
		if transport, isSCTP := stats.(webrtc.SCTPTransportStats); isSCTP {
			return transport.CongestionWindow
		}
	}
	return 0
}

// closeOneThenTheOther closes the client's peer and waits, at most closeSettlesWithin, for the core's
// SCTP association to hear its ABORT (heardTheEnd) before the deferred close of the core's peer runs.
// Closed at once, or by pion itself on the client's DTLS close_notify, the core's SCTPTransport.Stop
// can hold its lock while Abort waits 200 ms on a timer for a write loop the ABORT has already ended,
// and the goroutine that accepted data channels blocks on that lock to report the close; a goroutine
// blocked on a mutex is not durably blocked, so the bubble never idles, its clock never moves and the
// timer never fires. GracefulClose waits for the goroutines pion started, as the bubble's clock stops
// once the test's own goroutine returns.
func closeOneThenTheOther(t *testing.T, clientPeer *webrtc.PeerConnection, heardTheEnd <-chan struct{}) {
	t.Helper()
	closeQuietly(t, "the client's peer", clientPeer.GracefulClose)
	select {
	case <-heardTheEnd:
	case <-time.After(closeSettlesWithin):
	}
}

// closeSettlesWithin bounds the wait for one peer to hear the other close: a virtual network with no
// loss carries the ABORT at once, and a test that failed before the association began has none to
// hear.
const closeSettlesWithin = time.Second

// closeQuietly closes what with closeIt, failing the test if it fails.
func closeQuietly(t *testing.T, what string, closeIt func() error) {
	t.Helper()
	if err := closeIt(); err != nil {
		t.Errorf("close %s: %v", what, err)
	}
}
