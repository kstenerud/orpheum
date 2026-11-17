// SPDX-License-Identifier: AGPL-3.0-or-later

package mp4test

import (
	"bytes"
	"encoding/binary"
)

// MP4 is an MP4 to build: its movie timescale and tracks, its movie box after its samples, as a file
// written by a recorder has it. Its samples are bytes, not media: what an index is read and cut down
// from (D512), not what FFmpeg plays.
type MP4 struct {
	Timescale uint32
	Tracks    []MP4Track
	// IsFragmented adds a movie extends box, as a fragmented MP4 has.
	IsFragmented bool
}

// MP4Track is one track of an MP4: its samples' durations, composition offsets, sync samples and
// sizes, grouped SamplesPerChunk to a chunk, its chunks interleaved with the other tracks'.
type MP4Track struct {
	// Description is its sample description's kind, avc1 or mp4a; Descriptions how many it has, one
	// when zero.
	Description  string
	Descriptions int
	Timescale    uint32
	// Durations are its samples' durations, one a sample.
	Durations []uint32
	// Offsets, when set, are its samples' composition offsets, one a sample, in a version 1 ctts.
	Offsets []int32
	// Syncs, when set, are its sync samples, from 1; a track without them is all sync samples.
	Syncs []uint32
	// Sizes are its samples' sizes, one a sample; all alike, they are stored as one.
	Sizes           []uint32
	SamplesPerChunk int
	// IsWide stores its chunk offsets in a co64.
	IsWide bool
	// Edits, when set, are its edit list: each a duration in the movie's timescale, a media time, -1
	// for an empty edit, and a rate in 16.16.
	Edits [][3]int64
	// Extra are boxes added to its sample table, whole.
	Extra [][]byte
}

// sampleFill is what a sample's bytes are drawn from.
const sampleFill = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// sampleData is a sample of size bytes, its pattern telling its track and number apart from its
// neighbours'.
func sampleData(track, sample, size int) []byte {
	pattern := []byte{sampleFill[track%len(sampleFill)], sampleFill[sample%len(sampleFill)]}
	return bytes.Repeat(pattern, (size+1)/2)[:size]
}

// Box is an MP4 box of kind holding payload.
func Box(kind string, payload ...[]byte) []byte {
	joined := bytes.Join(payload, nil)
	size := uint32(8 + len(joined)) //nolint:gosec // a test's small box
	return append(binary.BigEndian.AppendUint32([]byte(nil), size), append([]byte(kind), joined...)...)
}

// FullBox is an MP4 full box of kind, its version and flags zero but version, holding payload.
func FullBox(kind string, version byte, payload ...[]byte) []byte {
	return Box(kind, append([]byte{version, 0, 0, 0}, bytes.Join(payload, nil)...))
}

// u32 is value as four bytes, big endian.
func u32(values ...uint32) []byte {
	var out []byte
	for _, value := range values {
		out = binary.BigEndian.AppendUint32(out, value)
	}
	return out
}

// MP4File is the MP4's bytes: an ftyp, an mdat of every sample, each sample's bytes its track's and
// sample's numbers, and the movie box last.
func MP4File(movie MP4) []byte {
	file := Box("ftyp", []byte("isom"), u32(0x200), []byte("isomiso2"))
	mdatAt := len(file)
	var data []byte
	offsets := make([][]uint64, len(movie.Tracks))
	next := make([]int, len(movie.Tracks))
	for remaining := true; remaining; {
		remaining = false
		for number, track := range movie.Tracks {
			first := next[number]
			if first >= len(track.Sizes) {
				continue
			}
			remaining = true
			last := min(first+track.SamplesPerChunk, len(track.Sizes))
			offsets[number] = append(offsets[number], uint64(mdatAt+8+len(data)))
			for sample := first; sample < last; sample++ {
				data = append(data, sampleData(number, sample, int(track.Sizes[sample]))...)
			}
			next[number] = last
		}
	}
	file = append(file, Box("mdat", data)...)
	traks := [][]byte{FullBox("mvhd", 0, u32(0, 0, movie.Timescale, 0, 1<<16), make([]byte, 80))}
	if movie.IsFragmented {
		traks = append(traks, Box("mvex", FullBox("trex", 0, u32(1, 1, 0, 0, 0))))
	}
	for number, track := range movie.Tracks {
		traks = append(traks, trakOf(track, offsets[number]))
	}
	return append(file, Box("moov", traks...)...)
}

// trakOf is the track's trak, its chunks at offsets.
func trakOf(track MP4Track, offsets []uint64) []byte {
	stbl := [][]byte{descriptionsOf(track), FullBox("stts", 0, runsOf(len(track.Durations), func(n int) uint32 {
		return track.Durations[n]
	}))}
	if track.Offsets != nil {
		stbl = append(stbl, FullBox("ctts", 1, runsOf(len(track.Offsets), func(n int) uint32 {
			return uint32(track.Offsets[n]) //nolint:gosec // signed, as version 1 has it
		})))
	}
	if track.Syncs != nil {
		syncs := uint32(len(track.Syncs)) //nolint:gosec // a test's
		stbl = append(stbl, FullBox("stss", 0, u32(syncs), u32(track.Syncs...)))
	}
	stbl = append(stbl, FullBox("stsc", 0, chunksOf(len(track.Sizes), track.SamplesPerChunk)), sizesOf(track.Sizes))
	if track.IsWide {
		wide := u32(uint32(len(offsets))) //nolint:gosec // a test's
		for _, offset := range offsets {
			wide = binary.BigEndian.AppendUint64(wide, offset)
		}
		stbl = append(stbl, FullBox("co64", 0, wide))
	} else {
		narrow := u32(uint32(len(offsets))) //nolint:gosec // a test's
		for _, offset := range offsets {
			narrow = append(narrow, u32(uint32(offset))...) //nolint:gosec // a test's small file
		}
		stbl = append(stbl, FullBox("stco", 0, narrow))
	}
	stbl = append(stbl, track.Extra...)
	var parts [][]byte
	if track.Edits != nil {
		list := u32(uint32(len(track.Edits))) //nolint:gosec // a test's
		for _, edit := range track.Edits {
			for _, value := range edit {
				list = append(list, u32(uint32(value))...) //nolint:gosec // signed, as the list has it
			}
		}
		parts = append(parts, Box("edts", FullBox("elst", 0, list)))
	}
	media := FullBox("mdhd", 0, u32(0, 0, track.Timescale, 0, 0x55c40000))
	return Box("trak", append(parts, Box("mdia", media, Box("minf", Box("stbl", stbl...))))...)
}

// descriptionsOf is the track's stsd, of as many descriptions as it has.
func descriptionsOf(track MP4Track) []byte {
	count := max(track.Descriptions, 1)
	entries := u32(uint32(count)) //nolint:gosec // a test's
	for range count {
		entries = append(entries, Box(track.Description, make([]byte, 6), []byte{0, 1})...)
	}
	return FullBox("stsd", 0, entries)
}

// runsOf is a count of samples' values run-length coded, as stts and ctts have them.
func runsOf(count int, value func(n int) uint32) []byte {
	var runs []uint32
	for n := range count {
		if len(runs) > 0 && runs[len(runs)-1] == value(n) {
			runs[len(runs)-2]++
			continue
		}
		runs = append(runs, 1, value(n))
	}
	return append(u32(uint32(len(runs)/2)), u32(runs...)...) //nolint:gosec // a test's
}

// chunksOf is an stsc of samples grouped perChunk to a chunk, the last chunk holding the rest.
func chunksOf(samples, perChunk int) []byte {
	full, rest := samples/perChunk, samples%perChunk
	entries := []uint32{1, uint32(perChunk), 1} //nolint:gosec // a test's
	count := uint32(1)
	if rest > 0 {
		entries, count = append(entries, uint32(full+1), uint32(rest), 1), 2 //nolint:gosec // a test's
	}
	return append(u32(count), u32(entries...)...)
}

// sizesOf is an stsz of sizes, stored as one when all are alike.
func sizesOf(sizes []uint32) []byte {
	isAlike := true
	for _, size := range sizes {
		isAlike = isAlike && size == sizes[0]
	}
	if isAlike {
		return FullBox("stsz", 0, u32(sizes[0], uint32(len(sizes)))) //nolint:gosec // a test's
	}
	return FullBox("stsz", 0, u32(0, uint32(len(sizes))), u32(sizes...)) //nolint:gosec // a test's
}
