// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"log/slog"
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// aDeviceAtHome is a device in the house, proven at home over a direct pair to the core's LAN port.
var aDeviceAtHome = rtcapi.ProvenAtHome{Path: rtcapi.PathDirectIPv4, Device: netip.MustParseAddr("192.168.1.30")}

// offerFrom is an offer the discovery saw come from network, naming address, the device's own IPv6
// one, among its candidates; none when it is empty.
func offerFrom(network, address string) discoveryproto.SignalMessage {
	sdp := "an offer\r\na=fingerprint:sha-256 AB:CD"
	if address != "" {
		sdp += "\r\na=candidate:2 1 udp 1686052607 " + address + " 50001 typ srflx raddr :: rport 0"
	}
	offer := provenOffer("1", sdp)
	offer.ClientNetwork = network
	return offer
}

// sentHouseNetworks are the house networks the last heartbeat carried, as "network source".
func sentHouseNetworks(t *testing.T, built *world) []string {
	t.Helper()
	built.fake.mutex.Lock()
	defer built.fake.mutex.Unlock()
	if len(built.fake.heartbeats) == 0 {
		t.Fatal("no heartbeat was sent")
	}
	var sent []string
	for _, each := range built.fake.heartbeats[len(built.fake.heartbeats)-1].HouseNetworks {
		sent = append(sent, each.Network+" "+each.Source)
	}
	return sent
}

// heartbeatCount is how many heartbeats the discovery took.
func heartbeatCount(built *world) int {
	built.fake.mutex.Lock()
	defer built.fake.mutex.Unlock()
	return len(built.fake.heartbeats)
}

// An offer whose connection proves itself from home teaches the network the discovery saw it come
// from, by the device that witnessed it and when; the log says whether the device's own IPv6 address
// in its offer agrees (D356 §1, §2, Q5).
func TestHouseNetworks_ADirectConnectionFromTheHouseTeachesItsOffersNetwork(t *testing.T) {
	t.Parallel()
	log := &testutil.Log{}
	answerer := &answering{proven: &aDeviceAtHome}
	built := newWorld(t)
	built.service.parts.Logger = slog.New(slog.NewTextHandler(log, nil))
	built.service.parts.Answerer, built.service.parts.OfferKey = answerer, offerKeys
	if _, err := built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.fake.claim()
	built.service.step(t.Context())
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		built.service.signal(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		testutil.Receive(t, done, "the signalling loop to end")
	})
	built.fake.signals <- offerFrom("2a0f:1000:1:2::/64", "2a0f:1000:1:2::77")
	reply := testutil.Receive(t, built.fake.replies, "the core's answer")
	if reply.Type != discoveryproto.SignalAnswer {
		t.Fatalf("the offer was answered %+v", reply)
	}
	log.AwaitLine(t, "a device proven at home witnessed the house's network", "network=2a0f:1000:1:2::/64",
		"isNew=true", "device=192.168.1.30", "path=direct_ipv4", "offerAgrees=yes")
	networks := built.service.Status().HouseNetworks
	if len(networks) != 1 || networks[0].Network.String() != "2a0f:1000:1:2::/64" ||
		networks[0].Source != discoveryproto.HouseNetworkLearnt || networks[0].Device != aDeviceAtHome.Device ||
		!networks[0].WitnessedAt.Equal(built.clock.Now()) {
		t.Errorf("the house networks are %+v; want the offer's, learnt from 192.168.1.30 now", networks)
	}
	// An offer whose own address is in another network is learnt all the same, and the log says so.
	built.service.learnHouseNetwork(t.Context(), offerFrom("2a0f:1000:1:2::/64", "2a0f:3000:5:6::1"), aDeviceAtHome)
	log.AwaitLine(t, "network=2a0f:1000:1:2::/64", "isNew=false", "offerAgrees=no", "offerNetworks=[2a0f:3000:5:6::/64]")
	built.service.learnHouseNetwork(t.Context(), offerFrom("2a0f:1000:1:2::/64", ""), aDeviceAtHome)
	log.AwaitLine(t, "network=2a0f:1000:1:2::/64", "isNew=false", "offerAgrees=unknown")
}

// An offer that came over IPv4, or with anything but a public IPv6 /64, teaches nothing, however its
// connection proved itself: a carrier's shared address names no household (D356 §2, §5).
func TestHouseNetworks_AnOfferFromNoIPv6NetworkTeachesNothing(t *testing.T) {
	t.Parallel()
	built := claimedWorld(t)
	for _, network := range []string{"", "11.22.33.0/24", "2a0f:1000::/32", "fd77:1:2:3::/64", "not a network"} {
		built.service.learnHouseNetwork(t.Context(), offerFrom(network, ""), aDeviceAtHome)
	}
	if networks := built.service.Status().HouseNetworks; len(networks) != 0 {
		t.Errorf("the house networks are %+v; want none learnt", networks)
	}
}

// A network newly learnt is sent at once, not at the next interval: the loop is woken, and its next
// step sends a heartbeat carrying it. One witnessed again sends nothing new (D356 §3).
func TestHouseNetworks_ANewNetworkSendsAHeartbeatAtOnce(t *testing.T) {
	t.Parallel()
	built := claimedWorld(t)
	before := heartbeatCount(built)
	select {
	case <-built.service.wake: // the enrolment's own wake, which no loop took
	default:
	}
	built.service.learnHouseNetwork(t.Context(), offerFrom("2a0f:1000:1:2::/64", ""), aDeviceAtHome)
	select {
	case <-built.service.wake:
	default:
		t.Fatal("learning a new network did not wake the loop")
	}
	built.service.step(t.Context())
	if count := heartbeatCount(built); count != before+1 {
		t.Fatalf("%d heartbeats after learning; want one more than %d, at once", count, before)
	}
	if sent := sentHouseNetworks(t, built); !slices.Equal(sent, []string{"2a0f:1000:1:2::/64 learnt"}) {
		t.Errorf("the heartbeat carried %v; want the learnt network", sent)
	}
	built.service.learnHouseNetwork(t.Context(), offerFrom("2a0f:1000:1:2::/64", ""), aDeviceAtHome)
	select {
	case <-built.service.wake:
		t.Error("a network witnessed again woke the loop")
	default:
	}
	built.service.step(t.Context())
	if heartbeatCount(built) != before+1 {
		t.Error("a network witnessed again sent a heartbeat before the interval")
	}
}

// The heartbeat carries the learnt networks, the most recently witnessed first, then those the admin
// named that are not among them, at most four (D356 §3, §6).
func TestHeartbeat_CarriesTheHouseNetworksMostRecentFirst(t *testing.T) {
	t.Parallel()
	built := claimedWorld(t)
	built.service.parts.ConfiguredHouseNetworks = []netip.Prefix{
		netip.MustParsePrefix("2a0f:3000:9:8::/64"), netip.MustParsePrefix("2a0f:1000:1:1::/64"),
		netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("2a0f:1000::/32"),
	}
	for _, network := range []string{"2a0f:1000:1:1::/64", "2a0f:1000:1:2::/64", "2a0f:1000:1:1::/64"} {
		built.service.learnHouseNetwork(t.Context(), offerFrom(network, ""), aDeviceAtHome)
		built.clock.Advance(time.Hour)
	}
	built.service.step(t.Context())
	want := []string{"2a0f:1000:1:1::/64 learnt", "2a0f:1000:1:2::/64 learnt", "2a0f:3000:9:8::/64 configured"}
	if sent := sentHouseNetworks(t, built); !slices.Equal(sent, want) {
		t.Errorf("the heartbeat carried %v; want %v", sent, want)
	}
}

// A learnt network no device has witnessed for thirty days is no longer sent: the provider has likely
// moved the prefix, and may give it to another household (D356 §4).
func TestHouseNetworks_ANetworkUnwitnessedForThirtyDaysIsDropped(t *testing.T) {
	t.Parallel()
	built := claimedWorld(t)
	built.service.learnHouseNetwork(t.Context(), offerFrom("2a0f:1000:1:1::/64", ""), aDeviceAtHome)
	built.clock.Advance(HouseNetworkKeptFor / 2)
	built.service.learnHouseNetwork(t.Context(), offerFrom("2a0f:1000:1:2::/64", ""), aDeviceAtHome)
	built.clock.Advance(HouseNetworkKeptFor / 2)
	if networks := built.service.Status().HouseNetworks; len(networks) != 2 {
		t.Fatalf("thirty days after the first was witnessed, the house networks are %+v; want both", networks)
	}
	built.clock.Advance(time.Minute)
	built.service.step(t.Context())
	if sent := sentHouseNetworks(t, built); !slices.Equal(sent, []string{"2a0f:1000:1:2::/64 learnt"}) {
		t.Errorf("past thirty days the heartbeat carried %v; want the first dropped", sent)
	}
}

// A fifth network learnt pushes out the one least recently witnessed (D356 §4).
func TestHouseNetworks_AFifthPushesOutTheLeastRecent(t *testing.T) {
	t.Parallel()
	built := claimedWorld(t)
	for _, network := range []string{
		"2a0f:1000:1:1::/64", "2a0f:1000:1:2::/64", "2a0f:1000:1:3::/64", "2a0f:1000:1:4::/64",
		"2a0f:1000:1:1::/64", "2a0f:1000:1:5::/64",
	} {
		built.service.learnHouseNetwork(t.Context(), offerFrom(network, ""), aDeviceAtHome)
		built.clock.Advance(time.Minute)
	}
	var kept []string
	for _, each := range built.service.Status().HouseNetworks {
		kept = append(kept, each.Network.String())
	}
	want := []string{"2a0f:1000:1:5::/64", "2a0f:1000:1:1::/64", "2a0f:1000:1:4::/64", "2a0f:1000:1:3::/64"}
	if !slices.Equal(kept, want) {
		t.Errorf("the house networks are %v; want %v, the second pushed out", kept, want)
	}
}

// The learnt networks are kept in discovery.json, so a restart keeps them, and a Leave forgets them
// with the enrolment (D356 §4).
func TestHouseNetworks_SurviveARestartAndGoWithALeave(t *testing.T) {
	t.Parallel()
	built := claimedWorld(t)
	built.service.learnHouseNetwork(t.Context(), offerFrom("2a0f:1000:1:2::/64", ""), aDeviceAtHome)
	restarted := built.restart(t).Status().HouseNetworks
	if len(restarted) != 1 || restarted[0].Network.String() != "2a0f:1000:1:2::/64" ||
		restarted[0].Device != aDeviceAtHome.Device || !restarted[0].WitnessedAt.Equal(built.clock.Now()) {
		t.Fatalf("after a restart the house networks are %+v; want the one learnt, by its device and when", restarted)
	}
	// A zone the discovery moves the core to is kept beside them.
	built.service.changed(t.Context(), built.service.Status(), discoveryproto.StatusResponse{
		State: discoveryproto.StateClaimed, Zone: "another.example",
	})
	if again := built.restart(t).Status().HouseNetworks; len(again) != 1 {
		t.Errorf("keeping a new zone lost the house networks: %+v", again)
	}
	if err := built.service.Leave(t.Context()); err != nil {
		t.Fatal(err)
	}
	if networks := built.service.Status().HouseNetworks; len(networks) != 0 {
		t.Errorf("after leaving the house networks are %+v; want none", networks)
	}
	if _, err := built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	if networks := built.restart(t).Status().HouseNetworks; len(networks) != 0 {
		t.Errorf("enrolled again after leaving, the house networks are %+v; want none", networks)
	}
}

// A network witnessed again is written to discovery.json only a day or more after the witnessing it
// holds, not per connection from home; the heartbeat's order follows every witnessing at once (D356
// section 4).
func TestHouseNetworks_AWitnessingAgainIsWrittenOnlyADayOn(t *testing.T) {
	t.Parallel()
	built := claimedWorld(t)
	witness := func(network string) {
		built.service.learnHouseNetwork(t.Context(), offerFrom(network, ""), aDeviceAtHome)
	}
	witness("2a0f:1000:1:1::/64")
	witness("2a0f:1000:1:2::/64")
	written := built.files.writeCount(StateFileName)
	built.clock.Advance(time.Hour)
	witness("2a0f:1000:1:1::/64")
	built.clock.Advance(time.Hour)
	witness("2a0f:1000:1:1::/64")
	if count := built.files.writeCount(StateFileName); count != written {
		t.Errorf("two witnessings within a day wrote %d times; want none", count-written)
	}
	if networks := built.service.Status().HouseNetworks; len(networks) != 2 ||
		networks[0].Network.String() != "2a0f:1000:1:1::/64" || !networks[0].WitnessedAt.Equal(built.clock.Now()) {
		t.Errorf("the house networks are %+v; want the one witnessed again first, witnessed now", networks)
	}
	built.clock.Advance(houseNetworkKeepEvery - 2*time.Hour)
	witness("2a0f:1000:1:1::/64")
	if count := built.files.writeCount(StateFileName); count != written+1 {
		t.Errorf("a witnessing a day on wrote %d times; want once", count-written)
	}
	if restarted := built.restart(t).Status().HouseNetworks; len(restarted) != 2 ||
		!restarted[0].WitnessedAt.Equal(built.clock.Now()) {
		t.Errorf("after a restart the house networks are %+v; want the day-on witnessing kept", restarted)
	}
}
