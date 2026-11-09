// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import (
	"encoding/json"
	"net/netip"
)

// MaxLearnedFromLength is the longest device description kept with the address at home: a user
// agent, cut there.
const MaxLearnedFromLength = 200

// addressAtHomeDefinitions are the server's address in the house, which internal/core/home learns
// from the requests that name it or an admin sets, and the device it was learned from (D452 §3, §4).
// A learned value is written as any change is, and marked modified; reset forgets it, and the server
// learns it again from the next request that names it.
func addressAtHomeDefinitions() []Definition {
	return []Definition{
		{
			Key: AddressAtHome, Kind: KindString, StartJSON: json.RawMessage(`""`),
			IsValid: isEmptyOrPrivateIPv4,
			Description: "the server's IPv4 address in the house, which devices at home reach it at directly: " +
				"learned from a request at home that names it, or set by an admin; a configured address " +
				"(ORPHEUM_LAN_ADDRESS) wins over it, and empty is none known yet (D452 §3)",
			StartSource: "D452 §3: none until a request at home names the server by its address",
		},
		{
			Key: AddressAtHomeLearnedFrom, Kind: KindString, StartJSON: json.RawMessage(`""`),
			IsValid: func(text string) bool { return len(text) <= MaxLearnedFromLength },
			Description: "the device the address at home was learned from, its browser's user agent; empty when an " +
				"admin set it, as who set it says (D452 §3)",
			StartSource: "D452 §3: none until the address at home is learned",
		},
	}
}

// isEmptyOrPrivateIPv4 is whether text is no address, or a private IPv4 one: the only addresses a
// device in the house reaches the server at that the server can learn (D452 §3).
func isEmptyOrPrivateIPv4(text string) bool {
	if text == "" {
		return true
	}
	address, err := netip.ParseAddr(text)
	return err == nil && address.Is4() && address.IsPrivate()
}
