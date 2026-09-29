// SPDX-License-Identifier: GPL-3.0-or-later

package hash

import (
	"fmt"
	"io"
)

// Matroska element IDs, with their marker bits, as the specification writes them.
const (
	idEBML               = 0x1A45DFA3
	idSegment            = 0x18538067
	idSeekHead           = 0x114D9B74
	idSeek               = 0x4DBB
	idSeekID             = 0x53AB
	idSeekPosition       = 0x53AC
	idCluster            = 0x1F43B675
	idCues               = 0x1C53BB6B
	idCuePoint           = 0xBB
	idCueTrackPositions  = 0xB7
	idCueClusterPosition = 0xF1
)

// Bounds on what a hostile file can make the node read or walk.
const (
	maxSeekHeadBytes = 64 << 10
	maxCuesBytes     = 32 << 20
	maxElementHops   = 1 << 20
)

// unknownSize is an element whose size field is all ones: it runs to the end of its parent.
const unknownSize = -1

// element is one EBML element header: where it starts, its ID, and where its data lies.
type element struct {
	start, dataStart, dataSize int64
	id                         uint64
}

func (e element) end() int64 { return e.dataStart + e.dataSize }

// matroskaPayload returns the span from the first Cluster's start to the last Cluster's end
// (D179 §3). The first is found by stepping over the few elements before it; the last by
// starting at the last cue's cluster, found through the SeekHead, and stepping to the end of
// the segment. Without Cues it steps over every cluster header instead: more reads, the same
// answer.
func matroskaPayload(file io.ReaderAt, sizeBytes int64) (payloadSpan, error) {
	header, err := readElement(file, sizeBytes, 0)
	if err != nil || header.id != idEBML {
		return payloadSpan{}, fmt.Errorf("%w: no EBML header", errNotThisContainer)
	}
	segment, err := readElement(file, sizeBytes, header.end())
	if err != nil || segment.id != idSegment {
		return payloadSpan{}, fmt.Errorf("%w: no Segment after the EBML header", errNotThisContainer)
	}
	segmentEnd := sizeBytes
	if segment.dataSize != unknownSize {
		segmentEnd = min(segment.end(), sizeBytes)
	}

	firstCluster, cuesPosition, err := walkToFirstCluster(file, sizeBytes, segment.dataStart, segmentEnd)
	if err != nil {
		return payloadSpan{}, err
	}
	from := firstCluster
	if cuesPosition >= 0 {
		// A cue that points anywhere but at a cluster is ignored: the walk from the first
		// cluster gives the same answer.
		if lastCued, found := lastCuedCluster(file, sizeBytes, segment.dataStart, cuesPosition); found {
			from = max(lastCued, firstCluster)
		}
	}
	lastEnd, err := walkToLastClusterEnd(file, sizeBytes, from, segmentEnd)
	if err != nil {
		return payloadSpan{}, err
	}
	return payloadSpan{start: firstCluster, end: lastEnd, anchor: AnchorMatroska}, nil
}

// walkToFirstCluster steps over top-level elements from the segment's data to the first
// Cluster, and notes where the SeekHead says the Cues are (-1 when it does not say).
func walkToFirstCluster(
	file io.ReaderAt, sizeBytes, segmentDataStart, segmentEnd int64,
) (firstCluster, cuesPosition int64, err error) {
	cuesPosition = -1
	position := segmentDataStart
	for range maxElementHops {
		if position >= segmentEnd {
			return 0, 0, fmt.Errorf("%w: the segment has no cluster", errNotThisContainer)
		}
		found, readErr := readElement(file, sizeBytes, position)
		if readErr != nil {
			return 0, 0, readErr
		}
		switch {
		case found.id == idCluster:
			return found.start, cuesPosition, nil
		case found.dataSize == unknownSize:
			return 0, 0, fmt.Errorf("%w: a header element of unknown size", errNotThisContainer)
		case found.id == idSeekHead && cuesPosition < 0:
			cuesPosition = seekPositionOfCues(file, sizeBytes, found, segmentDataStart)
		}
		position = found.end()
	}
	return 0, 0, fmt.Errorf("%w: too many elements before the first cluster", errNotThisContainer)
}

// walkToLastClusterEnd steps over top-level elements from a cluster to the segment's end and
// returns where the last Cluster ends.
func walkToLastClusterEnd(file io.ReaderAt, sizeBytes, from, segmentEnd int64) (int64, error) {
	lastEnd := int64(-1)
	position := from
	for range maxElementHops {
		if position >= segmentEnd {
			if lastEnd < 0 {
				return 0, fmt.Errorf("%w: no cluster at the cued position", errNotThisContainer)
			}
			return lastEnd, nil
		}
		found, err := readElement(file, sizeBytes, position)
		if err != nil {
			return 0, err
		}
		if found.dataSize == unknownSize || found.end() > sizeBytes {
			return 0, fmt.Errorf("%w: an element at %d of unknown size or past the end of the file",
				errNotThisContainer, found.start)
		}
		if found.id == idCluster {
			lastEnd = found.end()
		}
		position = found.end()
	}
	return 0, fmt.Errorf("%w: too many elements after the first cluster", errNotThisContainer)
}

// seekPositionOfCues reads a SeekHead and returns the Cues' absolute position, or -1.
func seekPositionOfCues(file io.ReaderAt, sizeBytes int64, seekHead element, segmentDataStart int64) int64 {
	children, err := readChildren(file, sizeBytes, seekHead, maxSeekHeadBytes)
	if err != nil {
		return -1
	}
	for _, seek := range children {
		if seek.id != idSeek {
			continue
		}
		var target, position uint64
		var hasTarget, hasPosition bool
		for _, field := range seek.children {
			switch field.id {
			case idSeekID:
				target, hasTarget = field.integer(), true
			case idSeekPosition:
				position, hasPosition = field.integer(), true
			}
		}
		isCues := hasTarget && hasPosition && target == idCues
		if isCues && position < uint64(sizeBytes) { //nolint:gosec // sizeBytes >= 0
			return segmentDataStart + int64(position)
		}
	}
	return -1
}

// lastCuedCluster reads the Cues and returns the absolute position of the latest cluster any
// cue points at, when that position holds a Cluster.
func lastCuedCluster(file io.ReaderAt, sizeBytes, segmentDataStart, cuesPosition int64) (int64, bool) {
	cues, err := readElement(file, sizeBytes, cuesPosition)
	if err != nil || cues.id != idCues || cues.dataSize == unknownSize {
		return 0, false
	}
	points, err := readChildren(file, sizeBytes, cues, maxCuesBytes)
	if err != nil {
		return 0, false
	}
	latest := latestClusterPosition(points)
	if latest >= uint64(sizeBytes) { //nolint:gosec // sizeBytes >= 0
		return 0, false
	}
	at := segmentDataStart + int64(latest) //nolint:gosec // bounded by sizeBytes above
	cluster, err := readElement(file, sizeBytes, at)
	if err != nil || cluster.id != idCluster {
		return 0, false
	}
	return at, true
}

// latestClusterPosition is the largest CueClusterPosition among the cue points.
func latestClusterPosition(points []ebmlNode) uint64 {
	var latest uint64
	for _, point := range points {
		if point.id != idCuePoint {
			continue
		}
		for _, positions := range point.children {
			if positions.id != idCueTrackPositions {
				continue
			}
			for _, field := range positions.children {
				if field.id == idCueClusterPosition {
					latest = max(latest, field.integer())
				}
			}
		}
	}
	return latest
}

// readElement reads the element header at position.
func readElement(file io.ReaderAt, sizeBytes, position int64) (element, error) {
	var buffer [12]byte // an ID of at most 4 bytes and a size of at most 8
	available := min(int64(len(buffer)), sizeBytes-position)
	if available < 2 {
		return element{}, fmt.Errorf("%w: no element header at %d", errNotThisContainer, position)
	}
	if err := readAt(file, sizeBytes, position, buffer[:available]); err != nil {
		return element{}, err
	}
	id, idLength, ok := vint(buffer[:available], true)
	if !ok || idLength > 4 {
		return element{}, fmt.Errorf("%w: no element ID at %d", errNotThisContainer, position)
	}
	dataSize, sizeLength, ok := vint(buffer[idLength:available], false)
	if !ok {
		return element{}, fmt.Errorf("%w: no element size at %d", errNotThisContainer, position)
	}
	found := element{start: position, dataStart: position + int64(idLength+sizeLength), id: id, dataSize: unknownSize}
	if dataSize != allOnes(sizeLength) {
		if dataSize > uint64(sizeBytes) { //nolint:gosec // sizeBytes >= 0
			return element{}, fmt.Errorf("%w: an element at %d larger than the file", errNotThisContainer, position)
		}
		found.dataSize = int64(dataSize) //nolint:gosec // bounded by sizeBytes above
	}
	return found, nil
}

// vint decodes an EBML variable-length integer. An ID keeps its marker bit; a size does not.
func vint(data []byte, keepMarker bool) (uint64, int, bool) {
	if len(data) == 0 || data[0] == 0 {
		return 0, 0, false
	}
	length := 1
	for mask := byte(0x80); data[0]&mask == 0; mask >>= 1 {
		length++
	}
	if length > 8 || length > len(data) {
		return 0, 0, false
	}
	value := uint64(data[0])
	if !keepMarker {
		value &= uint64(0xFF >> length)
	}
	for _, next := range data[1:length] {
		value = value<<8 | uint64(next)
	}
	return value, length, true
}

// allOnes is the value of a size field of length bytes whose data bits are all set: the
// reserved "unknown size".
func allOnes(length int) uint64 {
	return 1<<(7*length) - 1
}

// ebmlNode is an element read with its data, and its children when it is a master element.
type ebmlNode struct {
	id       uint64
	data     []byte
	children []ebmlNode
}

// integer is the node's data as a big-endian unsigned integer.
func (n ebmlNode) integer() uint64 {
	var value uint64
	for _, next := range n.data {
		value = value<<8 | uint64(next)
	}
	return value
}

// readChildren reads a master element of at most limit bytes and parses three levels of it,
// enough for SeekHead → Seek → fields and Cues → CuePoint → CueTrackPositions → fields.
func readChildren(file io.ReaderAt, sizeBytes int64, parent element, maxBytes int64) ([]ebmlNode, error) {
	if parent.dataSize == unknownSize || parent.dataSize > maxBytes {
		return nil, fmt.Errorf("%w: an element of %d bytes, more than %d", errNotThisContainer, parent.dataSize,
			maxBytes)
	}
	data := make([]byte, parent.dataSize)
	if err := readAt(file, sizeBytes, parent.dataStart, data); err != nil {
		return nil, err
	}
	return parseNodes(data, 3), nil
}

// parseNodes parses data as a run of elements, descending depth levels. It stops quietly at
// the first thing that is not an element: what it returns is what it could follow.
func parseNodes(data []byte, depth int) []ebmlNode {
	var nodes []ebmlNode
	for len(data) > 0 {
		id, idLength, ok := vint(data, true)
		if !ok || idLength > 4 {
			return nodes
		}
		dataSize, sizeLength, ok := vint(data[idLength:], false)
		headerLength := idLength + sizeLength
		if !ok || dataSize > uint64(len(data)-headerLength) { //nolint:gosec // len >= headerLength
			return nodes
		}
		body := data[headerLength : headerLength+int(dataSize)] //nolint:gosec // bounded above
		parsed := ebmlNode{id: id, data: body}
		if depth > 1 {
			parsed.children = parseNodes(body, depth-1)
		}
		nodes = append(nodes, parsed)
		data = data[headerLength+int(dataSize):] //nolint:gosec // bounded above
	}
	return nodes
}
