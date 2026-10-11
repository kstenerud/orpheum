// SPDX-License-Identifier: AGPL-3.0-or-later

package container

import (
	"fmt"
	"io"
)

// Matroska element IDs, with their marker bits, as the specification writes them.
const (
	IDEBML               = 0x1A45DFA3
	IDSegment            = 0x18538067
	IDSeekHead           = 0x114D9B74
	IDSeek               = 0x4DBB
	IDSeekID             = 0x53AB
	IDSeekPosition       = 0x53AC
	IDInfo               = 0x1549A966
	IDTimecodeScale      = 0x2AD7B1
	IDTracks             = 0x1654AE6B
	IDTrackEntry         = 0xAE
	IDTrackNumber        = 0xD7
	IDTrackType          = 0x83
	IDCluster            = 0x1F43B675
	IDCues               = 0x1C53BB6B
	IDCuePoint           = 0xBB
	IDCueTime            = 0xB3
	IDCueTrackPositions  = 0xB7
	IDCueTrack           = 0xF7
	IDCueClusterPosition = 0xF1
)

// UnknownSize is an element whose size field is all ones: it runs to the end of its parent.
const UnknownSize = -1

// Element is one EBML element header: where it starts, its ID, and where its data lies.
type Element struct {
	Start, DataStart, DataSize int64
	ID                         uint64
}

// End is where the element's data ends.
func (e Element) End() int64 { return e.DataStart + e.DataSize }

// ReadElement reads the element header at position.
func ReadElement(file io.ReaderAt, sizeBytes, position int64) (Element, error) {
	var buffer [12]byte // an ID of at most 4 bytes and a size of at most 8
	available := min(int64(len(buffer)), sizeBytes-position)
	if available < 2 {
		return Element{}, fmt.Errorf("%w: no element header at %d", ErrNotThisContainer, position)
	}
	if err := ReadAt(file, sizeBytes, position, buffer[:available]); err != nil {
		return Element{}, err
	}
	id, idLength, ok := vint(buffer[:available], true)
	if !ok || idLength > 4 {
		return Element{}, fmt.Errorf("%w: no element ID at %d", ErrNotThisContainer, position)
	}
	dataSize, sizeLength, ok := vint(buffer[idLength:available], false)
	if !ok {
		return Element{}, fmt.Errorf("%w: no element size at %d", ErrNotThisContainer, position)
	}
	found := Element{Start: position, DataStart: position + int64(idLength+sizeLength), ID: id, DataSize: UnknownSize}
	if dataSize != allOnes(sizeLength) {
		if dataSize > uint64(sizeBytes) { //nolint:gosec // sizeBytes >= 0
			return Element{}, fmt.Errorf("%w: an element at %d larger than the file", ErrNotThisContainer, position)
		}
		found.DataSize = int64(dataSize) //nolint:gosec // bounded by sizeBytes above
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

// Node is an element read with its data, and its children when it is a master element.
type Node struct {
	ID       uint64
	Data     []byte
	Children []Node
}

// Integer is the node's data as a big-endian unsigned integer.
func (n Node) Integer() uint64 {
	var value uint64
	for _, next := range n.Data {
		value = value<<8 | uint64(next)
	}
	return value
}

// ReadChildren reads a master element of at most maxBytes and parses depth levels of it: three
// for SeekHead → Seek → fields and Cues → CuePoint → CueTrackPositions → fields.
func ReadChildren(file io.ReaderAt, sizeBytes int64, parent Element, maxBytes int64, depth int) ([]Node, error) {
	if parent.DataSize == UnknownSize || parent.DataSize > maxBytes {
		return nil, fmt.Errorf("%w: an element of %d bytes, more than %d", ErrNotThisContainer, parent.DataSize,
			maxBytes)
	}
	data := make([]byte, parent.DataSize)
	if err := ReadAt(file, sizeBytes, parent.DataStart, data); err != nil {
		return nil, err
	}
	return parseNodes(data, depth), nil
}

// parseNodes parses data as a run of elements, descending depth levels. It stops quietly at the
// first thing that is not an element: what it returns is what it could follow.
func parseNodes(data []byte, depth int) []Node {
	var nodes []Node
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
		parsed := Node{ID: id, Data: body}
		if depth > 1 {
			parsed.Children = parseNodes(body, depth-1)
		}
		nodes = append(nodes, parsed)
		data = data[headerLength+int(dataSize):] //nolint:gosec // bounded above
	}
	return nodes
}
