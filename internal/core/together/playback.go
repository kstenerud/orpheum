// SPDX-License-Identifier: AGPL-3.0-or-later

package together

// Playback is how a device's player plays the film, as it says in its state: the delivery, "direct"
// for the file as it is or "converted" for its ladder, the quality it plays in kb/s, and the link it
// estimated as it chose that quality, in kb/s, each 0 while unknown. The owner: "The controller side
// should get information about playback, such as the transcoding and bandwidth choices" (D371 §4).
type Playback struct {
	Delivery    string `json:"delivery"`
	QualityKbps int64  `json:"qualityKbps,omitempty"`
	LinkKbps    int64  `json:"linkKbps,omitempty"`
}

// The deliveries a player says it plays by.
const (
	DeliveryDirect    = "direct"
	DeliveryConverted = "converted"
)

// isKnown is whether a player's playback is one a device may say: a known delivery, and no negative
// quality or link.
func (playback Playback) isKnown() bool {
	return (playback.Delivery == DeliveryDirect || playback.Delivery == DeliveryConverted) &&
		playback.QualityKbps >= 0 && playback.LinkKbps >= 0
}
