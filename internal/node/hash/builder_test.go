// SPDX-License-Identifier: GPL-3.0-or-later

package hash

import (
	"bytes"
	"encoding/binary"
	"math/rand/v2"
)

// Element IDs the builder writes that the hash never reads.
const (
	idInfo        = 0x1549A966
	idTitle       = 0x7BA9
	idTracks      = 0x1654AE6B
	idVoid        = 0xEC
	idTags        = 0x1254C367
	idAttachments = 0x1941A469
	idTimecode    = 0xE7
	idSimpleBlock = 0xA3
	idCueTime     = 0xB3
	idCueTrack    = 0xF7
)

// ebml writes one element with an ID as the specification writes it and an 8-byte size, so an
// element's length never depends on its contents' length class.
func ebml(id uint64, children ...[]byte) []byte {
	out := bytes.TrimLeft(binary.BigEndian.AppendUint64(nil, id), "\x00")
	size := 0
	for _, child := range children {
		size += len(child)
	}
	sizeAt := len(out)
	out = binary.BigEndian.AppendUint64(out, uint64(size))
	out[sizeAt] = 0x01 // the 8-byte length marker
	for _, child := range children {
		out = append(out, child...)
	}
	return out
}

// ebmlUint is an element holding an 8-byte unsigned integer.
func ebmlUint(id, value uint64) []byte {
	return ebml(id, binary.BigEndian.AppendUint64(nil, value))
}

// withUnknownSize rewrites an element written by ebml to declare an unknown size.
func withUnknownSize(element []byte, idLength int) []byte {
	out := bytes.Clone(element)
	copy(out[idLength:idLength+8], []byte{0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	return out
}

// matroska describes a synthetic file: what mkvmerge would write, and what mkvpropedit leaves
// behind after an edit (D179 §3).
type matroska struct {
	title string
	// titleMovedToEnd writes a Void where Info was and appends the Info after everything else,
	// as mkvpropedit does when a new title outgrows its slot.
	titleMovedToEnd bool
	blocks          [][]byte // one SimpleBlock payload per cluster
	cues            bool
	// uncuedAtEnd leaves the last clusters without a cue, as a file whose final clusters hold
	// no keyframe does, so finding the last cluster takes a walk past the last cue.
	uncuedAtEnd int
	trailing    [][]byte // top-level elements after the clusters: tags, attachments
}

// payloadBlocks returns deterministic pseudo-random block payloads totalling about total
// bytes, split into clusters of clusterSize.
func payloadBlocks(total, clusterSize int) [][]byte {
	source := rand.New(rand.NewChaCha8([32]byte{1, 2, 3})) //nolint:gosec // test data, not a secret
	data := make([]byte, total)
	for index := range data {
		data[index] = byte(source.Uint32() & 0xFF)
	}
	var blocks [][]byte
	for len(data) > 0 {
		size := min(clusterSize, len(data))
		blocks = append(blocks, data[:size])
		data = data[size:]
	}
	return blocks
}

// build writes the file. Positions in the SeekHead and Cues are relative to the segment's data,
// as the specification says; the SeekHead is written with a fixed size so they can be computed
// before it is.
func (m matroska) build() []byte { //nolint:varnamelen // m is the file being described
	info := ebml(idInfo, ebml(idTitle, []byte(m.title)))
	tracks := ebml(idTracks, bytes.Repeat([]byte{0x42}, 64))
	seekHead := func(cuesAt uint64) []byte {
		return ebml(idSeekHead, ebml(idSeek, ebmlUint(idSeekID, idCues), ebmlUint(idSeekPosition, cuesAt)))
	}
	var header []byte
	header = append(header, seekHead(0)...)
	if m.titleMovedToEnd {
		// The Void keeps Info's old slot, which is the length of a short title's Info.
		old := ebml(idInfo, ebml(idTitle, []byte("Short")))
		header = append(header, ebml(idVoid, make([]byte, len(old)-9))...)
	} else {
		header = append(header, info...)
	}
	header = append(header, tracks...)

	var clusters []byte
	var clusterPositions []uint64
	for index, block := range m.blocks {
		clusterPositions = append(clusterPositions, uint64(len(header)+len(clusters)))
		cluster := ebml(idCluster, ebmlUint(idTimecode, uint64(index)), ebml(idSimpleBlock, block))
		clusters = append(clusters, cluster...)
	}
	var cues []byte
	if m.cues {
		var points [][]byte
		for index, position := range clusterPositions[:len(clusterPositions)-m.uncuedAtEnd] {
			points = append(points, ebml(idCuePoint, ebmlUint(idCueTime, uint64(index)),
				ebml(idCueTrackPositions, ebmlUint(idCueTrack, 1), ebmlUint(idCueClusterPosition, position))))
		}
		cues = ebml(idCues, points...)
		cuesAt := uint64(len(header) + len(clusters))
		header = append(seekHead(cuesAt), header[len(seekHead(0)):]...)
	}
	body := [][]byte{header, clusters, cues}
	body = append(body, m.trailing...)
	if m.titleMovedToEnd {
		body = append(body, info)
	}
	return append(ebml(idEBML, []byte("webm-ish header")), ebml(idSegment, body...)...)
}
