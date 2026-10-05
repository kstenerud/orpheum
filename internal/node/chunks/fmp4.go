// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// box is one ISO BMFF box within a byte slice: its four-character type, where it starts, where its
// payload starts and where it ends.
type box struct {
	kind                string
	start, payload, end int
}

var errMalformed = errors.New("malformed MP4 box")

// boxesIn lists the boxes laid end to end in data[from:until].
func boxesIn(data []byte, from, until int) ([]box, error) {
	var boxes []box
	for position := from; position < until; {
		left := until - position
		if left < 8 {
			return nil, fmt.Errorf("%w: %d bytes left at %d", errMalformed, left, position)
		}
		size := uint64(binary.BigEndian.Uint32(data[position:]))
		kind := string(data[position+4 : position+8])
		header := 8
		switch size {
		case 0:
			size = uint64(left)
		case 1:
			if left < 16 {
				return nil, fmt.Errorf("%w: a large %s box cut short", errMalformed, kind)
			}
			size, header = binary.BigEndian.Uint64(data[position+8:]), 16
		}
		if size < uint64(header) || size > uint64(left) {
			return nil, fmt.Errorf("%w: %s of %d bytes at %d", errMalformed, kind, size, position)
		}
		end := position + int(size) //nolint:gosec // within left, an int
		boxes = append(boxes, box{kind: kind, start: position, payload: position + header, end: end})
		position = end
	}
	return boxes, nil
}

// Split divides one file of FFmpeg's segment muxer into its init (ftyp and moov) and its fragments
// (each moof with the mdat after it), dropping the rest, such as mfra.
func Split(file []byte) (init, fragments []byte, err error) {
	boxes, err := boxesIn(file, 0, len(file))
	if err != nil {
		return nil, nil, err
	}
	for _, each := range boxes {
		switch each.kind {
		case "ftyp", "moov":
			init = append(init, file[each.start:each.end]...)
		case "moof", "mdat":
			fragments = append(fragments, file[each.start:each.end]...)
		}
	}
	if len(init) == 0 || len(fragments) == 0 {
		return nil, nil, fmt.Errorf("%w: no init or no fragments", errMalformed)
	}
	return init, fragments, nil
}

// find walks down from the boxes laid in data[from:until] by the path of box types, returning every
// box at its end.
func find(data []byte, from, until int, path ...string) ([]box, error) {
	boxes, err := boxesIn(data, from, until)
	if err != nil {
		return nil, err
	}
	if len(path) == 0 {
		return nil, nil
	}
	kind, rest := path[0], path[1:]
	var found []box
	for _, each := range boxes {
		if each.kind != kind {
			continue
		}
		if len(rest) == 0 {
			found = append(found, each)
			continue
		}
		deeper, deeperErr := find(data, each.payload, each.end, rest...)
		if deeperErr != nil {
			return nil, deeperErr
		}
		found = append(found, deeper...)
	}
	return found, nil
}

// Timescale is the ticks per second of an init's one track.
func Timescale(init []byte) (uint32, error) {
	headers, err := find(init, 0, len(init), "moov", "trak", "mdia", "mdhd")
	if err != nil {
		return 0, err
	}
	if len(headers) != 1 {
		return 0, fmt.Errorf("%w: %d media headers, want one track", errMalformed, len(headers))
	}
	header := init[headers[0].payload:headers[0].end]
	if len(header) == 0 {
		return 0, fmt.Errorf("%w: an empty mdhd", errMalformed)
	}
	// The timescale follows the version and flags and two times, 32-bit in version 0, 64-bit in 1.
	timescaleAt, isKnown := map[byte]int{0: 12, 1: 20}[header[0]]
	if !isKnown || len(header) < timescaleAt+4 {
		return 0, fmt.Errorf("%w: an mdhd of version %d, %d bytes", errMalformed, header[0], len(header))
	}
	return binary.BigEndian.Uint32(header[timescaleAt:]), nil
}

// decodeTimes are every tfdt in fragments, each a 64-bit version 1 box, as FFmpeg writes them.
func decodeTimes(fragments []byte) ([]box, error) {
	tfdts, err := find(fragments, 0, len(fragments), "moof", "traf", "tfdt")
	if err != nil {
		return nil, err
	}
	if len(tfdts) == 0 {
		return nil, fmt.Errorf("%w: no decode time", errMalformed)
	}
	for _, each := range tfdts {
		if each.end-each.payload != 12 || fragments[each.payload] != 1 {
			return nil, fmt.Errorf("%w: a decode time not of version 1", errMalformed)
		}
	}
	return tfdts, nil
}

// Retime sets the fragments' first decode time to start, in the track's ticks, moving every later
// one by as much: FFmpeg starts each file at zero, and a chunk is played at its time in the film.
func Retime(fragments []byte, start uint64) ([]byte, error) {
	tfdts, err := decodeTimes(fragments)
	if err != nil {
		return nil, err
	}
	retimed := append([]byte(nil), fragments...)
	first := binary.BigEndian.Uint64(retimed[tfdts[0].payload+4:])
	for _, each := range tfdts {
		decodeTime := retimed[each.payload+4:]
		binary.BigEndian.PutUint64(decodeTime, binary.BigEndian.Uint64(decodeTime)-first+start)
	}
	return retimed, nil
}

// Samples is how many samples the fragments hold: for sound, its length in the track's frames.
func Samples(fragments []byte) (int64, error) {
	truns, err := find(fragments, 0, len(fragments), "moof", "traf", "trun")
	if err != nil {
		return 0, err
	}
	var count int64
	for _, each := range truns {
		if each.end-each.payload < 8 {
			return 0, fmt.Errorf("%w: a short trun", errMalformed)
		}
		count += int64(binary.BigEndian.Uint32(fragments[each.payload+4:]))
	}
	return count, nil
}

// trun flags this reads: whether each sample says its duration, size, flags and composition offset,
// and whether the run's first sample has flags of its own.
const (
	trunDataOffset       = 0x000001
	trunFirstSampleFlags = 0x000004
	trunSampleDuration   = 0x000100
	trunSampleSize       = 0x000200
	trunSampleFlags      = 0x000400
	trunCompositionTime  = 0x000800
)

// FirstShownAfterDecode is how many ticks after its decode time the fragments' first sample is
// shown: its composition offset, which B-frames make positive; zero when the samples say none.
func FirstShownAfterDecode(fragments []byte) (int64, error) {
	truns, err := find(fragments, 0, len(fragments), "moof", "traf", "trun")
	if err != nil {
		return 0, err
	}
	if len(truns) == 0 {
		return 0, fmt.Errorf("%w: no trun", errMalformed)
	}
	run := fragments[truns[0].payload:truns[0].end]
	if len(run) < 8 {
		return 0, fmt.Errorf("%w: a short trun", errMalformed)
	}
	version, flags := run[0], binary.BigEndian.Uint32(run)&0xffffff
	offset := 8 // version, flags, sample count
	// The fields before the first sample's composition offset, each four bytes when present.
	for _, field := range []uint32{
		trunDataOffset, trunFirstSampleFlags, trunSampleDuration, trunSampleSize, trunSampleFlags,
	} {
		if flags&field != 0 {
			offset += 4
		}
	}
	if flags&trunCompositionTime == 0 {
		return 0, nil
	}
	if len(run) < offset+4 {
		return 0, fmt.Errorf("%w: a short trun", errMalformed)
	}
	raw := binary.BigEndian.Uint32(run[offset:])
	if version == 0 {
		return int64(raw), nil
	}
	return int64(int32(raw)), nil //nolint:gosec // version 1's offset is signed, as the bits say
}
