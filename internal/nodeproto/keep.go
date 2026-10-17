// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// KeepTier is how strongly a chunk store is kept on its node's disk: the highest any player of the
// core gives it (D309 §8). The node, needing room, removes the lowest tier first, the least lately
// used within one; a store the core gives none is KeepUnlikely.
type KeepTier string

// The tiers, lowest first.
const (
	// KeepUnlikely is a store no one is coming back to: a quality switched from, a film another
	// replaced, a player silent past the last tier. The first removed, but only when room is needed.
	KeepUnlikely KeepTier = ""
	// KeepMaybe is a store a player may come back to: one that left its page, or was silent a while.
	KeepMaybe KeepTier = "maybe"
	// KeepLikely is a store a player is likely to come back to: one paused, or silent a moment.
	KeepLikely KeepTier = "likely"
	// KeepInUse is a store a playing player is reading.
	KeepInUse KeepTier = "inUse"
)

// rank is a tier's place, lowest first; an unknown tier is the lowest.
func (tier KeepTier) rank() int {
	switch tier {
	case KeepMaybe:
		return 1
	case KeepLikely:
		return 2
	case KeepInUse:
		return 3
	case KeepUnlikely:
		return 0
	}
	return 0
}

// IsAbove is whether the tier is kept more strongly than other.
func (tier KeepTier) IsAbove(other KeepTier) bool { return tier.rank() > other.rank() }
