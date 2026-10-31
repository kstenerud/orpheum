// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"net/netip"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
)

// HouseNetworkKeptFor is how long a learnt house network is kept after a device last witnessed it
// (D356 §4): a provider's prefix moves, nightly at some, and an old one may be another household's
// once it is reused. A constant, not a setting (the owner, 2026-10-27, D356 Q3).
const HouseNetworkKeptFor = 30 * 24 * time.Hour

// HouseNetwork is one of the house's public IPv6 /64s, as the core tells its discovery and its admin
// sees it: how it was had (discoveryproto.HouseNetworkLearnt or HouseNetworkConfigured), and for a
// learnt one the device in the house that last witnessed it, by its address there, and when (D356 §3,
// §10).
type HouseNetwork struct {
	Network     netip.Prefix
	Source      string
	Device      netip.Addr
	WitnessedAt time.Time
}

// keptHouseNetwork is a learnt house network as discovery.json keeps it, so a restart keeps it (D356
// §4): WitnessedAt in Unix milliseconds.
type keptHouseNetwork struct {
	Network     string `json:"network"`
	Device      string `json:"device,omitempty"`
	WitnessedAt int64  `json:"witnessedAt"`
}

// keptHouseNetworksOf are learnt as discovery.json keeps them.
func keptHouseNetworksOf(learnt []HouseNetwork) []keptHouseNetwork {
	kept := make([]keptHouseNetwork, 0, len(learnt))
	for _, each := range learnt {
		device := ""
		if each.Device.IsValid() {
			device = each.Device.String()
		}
		kept = append(kept, keptHouseNetwork{
			Network: each.Network.String(), Device: device, WitnessedAt: each.WitnessedAt.UnixMilli(),
		})
	}
	return kept
}

// learntOf are the learnt house networks discovery.json kept, those that are still house networks,
// the most recently witnessed first.
func learntOf(kept []keptHouseNetwork) []HouseNetwork {
	var learnt []HouseNetwork
	for _, each := range kept {
		network, err := netip.ParsePrefix(each.Network)
		if err != nil || !discoveryproto.IsHouseNetwork(network) {
			continue
		}
		device, _ := netip.ParseAddr(each.Device) //nolint:errcheck // none kept is none
		learnt = append(learnt, HouseNetwork{
			Network: network, Source: discoveryproto.HouseNetworkLearnt, Device: device,
			WitnessedAt: time.UnixMilli(each.WitnessedAt),
		})
	}
	sortMostRecentFirst(learnt)
	return learnt
}

// sortMostRecentFirst orders learnt house networks the most recently witnessed first.
func sortMostRecentFirst(learnt []HouseNetwork) {
	slices.SortStableFunc(learnt, func(a, b HouseNetwork) int { return b.WitnessedAt.Compare(a.WitnessedAt) })
}

// houseNetworksAt are the house's networks at now, as the heartbeat sends them: the learnt ones
// witnessed within HouseNetworkKeptFor, the most recent first, then those the admin named that are not
// among them, at most discoveryproto.MaxHouseNetworks in all (D356 §3, §4, §6). It forgets the learnt
// ones that aged out. The caller holds the mutex.
func (service *Service) houseNetworksAt(now time.Time) []HouseNetwork {
	service.learnt = slices.DeleteFunc(service.learnt, func(each HouseNetwork) bool {
		return now.Sub(each.WitnessedAt) > HouseNetworkKeptFor
	})
	networks := slices.Clone(service.learnt)
	for _, configured := range service.parts.ConfiguredHouseNetworks {
		isNamed := slices.ContainsFunc(networks, func(each HouseNetwork) bool { return each.Network == configured })
		if discoveryproto.IsHouseNetwork(configured) && !isNamed {
			networks = append(networks,
				HouseNetwork{Network: configured, Source: discoveryproto.HouseNetworkConfigured})
		}
	}
	return networks[:min(len(networks), discoveryproto.MaxHouseNetworks)]
}

// heartbeatHouseNetworks are networks as the heartbeat carries them.
func heartbeatHouseNetworks(networks []HouseNetwork) []discoveryproto.HouseNetwork {
	sent := make([]discoveryproto.HouseNetwork, 0, len(networks))
	for _, each := range networks {
		sent = append(sent, discoveryproto.HouseNetwork{Network: each.Network.String(), Source: each.Source})
	}
	return sent
}

// learnHouseNetwork keeps the network an offer came from, as its discovery saw it, as a house
// network, its connection having proven itself from home (D356 §1): witnessed now, by proven's
// device. A network already kept is witnessed again; a new one pushes out the least recently
// witnessed past discoveryproto.MaxHouseNetworks, is kept in discovery.json, and sends a heartbeat at
// once. An offer that came over IPv4, or with anything but a house network, teaches nothing. The log
// says whether the device's own IPv6 addresses in its offer agree with the network, without acting on
// it (the owner, 2026-10-27, D356 Q5).
func (service *Service) learnHouseNetwork(
	ctx context.Context, offer discoveryproto.SignalMessage, proven rtcapi.ProvenAtHome,
) {
	network, err := netip.ParsePrefix(offer.ClientNetwork)
	if err != nil || !discoveryproto.IsHouseNetwork(network) {
		service.parts.Logger.DebugContext(ctx, "discovery: a device proven at home asked from no IPv6 network; "+
			"nothing learnt", "offerId", offer.OfferID, "device", proven.Device.String(), "path", string(proven.Path))
		return
	}
	now := service.parts.Clock.Now()
	service.mutex.Lock()
	index := slices.IndexFunc(service.learnt, func(each HouseNetwork) bool { return each.Network == network })
	isNew := index < 0
	if isNew {
		service.learnt = append(service.learnt,
			HouseNetwork{Network: network, Source: discoveryproto.HouseNetworkLearnt})
		index = len(service.learnt) - 1
	}
	service.learnt[index].Device, service.learnt[index].WitnessedAt = proven.Device, now
	sortMostRecentFirst(service.learnt)
	service.learnt = service.learnt[:min(len(service.learnt), discoveryproto.MaxHouseNetworks)]
	service.mutex.Unlock()
	agrees, offered := offerAgreesWith(offer.SDP, network)
	service.parts.Logger.InfoContext(ctx, "discovery: a device proven at home witnessed the house's network (D356)",
		"network", network.String(), "isNew", isNew, "device", proven.Device.String(), "path", string(proven.Path),
		"offerAgrees", agrees, "offerNetworks", offered)
	if keepErr := service.keepHouseNetworks(); keepErr != nil {
		service.parts.Logger.WarnContext(ctx, "discovery: could not keep the house's networks", "err", keepErr)
	}
	if isNew {
		service.nudge()
	}
}

// offerAgreesWith is whether the public IPv6 addresses an offer names, its device's own, are in
// network: "yes" when one is, "no" when some are and none is in it, a VPN's or a relay's sign, and
// "unknown" when the offer names none, which a browser may hide; and the /64s it names (D356 Q5).
func offerAgreesWith(sdp string, network netip.Prefix) (string, []string) {
	var offered []string
	agrees := "unknown"
	for _, address := range rtcapi.OfferedIPv6Addresses(sdp) {
		if each, isNetwork := discoveryproto.HouseNetworkOf(address); isNetwork &&
			!slices.Contains(offered, each.String()) {
			offered = append(offered, each.String())
		}
		switch {
		case network.Contains(address):
			agrees = "yes"
		case agrees == "unknown":
			agrees = "no"
		}
	}
	return agrees, offered
}

// keepHouseNetworks writes discovery.json with the house networks learnt now, beside what it held.
func (service *Service) keepHouseNetworks() error {
	status := service.Status()
	if status.BaseURL == "" {
		return nil
	}
	return service.keep()
}
