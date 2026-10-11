// SPDX-License-Identifier: AGPL-3.0-or-later

package containertest

import (
	"bytes"
	"encoding/binary"
	"slices"

	"github.com/kstenerud/orpheum/internal/node/container"
)

// element writes one Matroska element with an 8-byte size.
func element(id uint64, children ...[]byte) []byte {
	out := bytes.TrimLeft(binary.BigEndian.AppendUint64(nil, id), "\x00")
	size := 0
	for _, part := range children {
		size += len(part)
	}
	sizeAt := len(out)
	out = binary.BigEndian.AppendUint64(out, uint64(size))
	out[sizeAt] = 0x01 // the 8-byte length marker
	for _, part := range children {
		out = append(out, part...)
	}
	return out
}

// uintElement is an element holding an 8-byte unsigned integer.
func uintElement(id, value uint64) []byte {
	return element(id, binary.BigEndian.AppendUint64(nil, value))
}

// MatroskaWithCues is a Matroska file of one track, a picture, stream 0, in millisecond ticks,
// whose cues, before its one cluster as some muxers write them, are keyframesMs.
func MatroskaWithCues(keyframesMs ...uint64) []byte {
	tracks := element(container.IDTracks, element(container.IDTrackEntry,
		uintElement(container.IDTrackNumber, 1), uintElement(container.IDTrackType, 1)))
	cluster := element(container.IDCluster, bytes.Repeat([]byte{0x42}, 64))
	points := make([][]byte, 0, len(keyframesMs))
	for _, time := range keyframesMs {
		points = append(points, element(container.IDCuePoint, uintElement(container.IDCueTime, time),
			element(container.IDCueTrackPositions, uintElement(container.IDCueTrack, 1),
				uintElement(container.IDCueClusterPosition, 0))))
	}
	body := slices.Concat(tracks, element(container.IDCues, points...), cluster)
	return append(element(container.IDEBML), element(container.IDSegment, body)...)
}
