// SPDX-License-Identifier: AGPL-3.0-or-later

package disc

import "encoding/binary"

// SoundBreak is where a DVD title's sound does not carry on across a clock shift, restamped: the
// last AC-3 frame before it is cut short, or the first after it starts more than soundSlackTicks
// before or after the last before it ended. FFmpeg's dvdvideo demuxer drops such a frame; the
// restamp keeps it, so the join may click (D405 §4). The node reports one, to find a disc that has
// it.
type SoundBreak struct {
	// FromByte is the clock shift's, in the title's stream.
	FromByte int64
	// AtMs is when the shift plays, from the title's start.
	AtMs int64
	// IsCutShort is whether the last frame before the shift is cut short.
	IsCutShort bool
	// GapTicks is how long after the last whole frame before the shift ends the first after it
	// starts, 90 kHz; negative when it starts before.
	GapTicks int64
}

// Bounds on reading a title's sound at its clock shifts: what is read either side of one, and how
// many are read, so a title of many shifts reads a few megabytes for them at most.
const (
	soundSectors    = 128
	maxSoundJoins   = 16
	soundSlackTicks = clockTicksPerSecond / 1000 // a millisecond
)

// The private stream 1 substreams of a DVD's AC-3 tracks, and an AC-3 frame's: its sync word, its
// samples (each 90 kHz frame of them 2880 ticks at 48 kHz, the one rate a DVD's AC-3 has).
const (
	firstAC3Substream, lastAC3Substream = 0x80, 0x87
	ac3Sync                             = 0x0B77
	ac3FrameTicks                       = 2880
)

// vobuSpan is a VOBU's sectors of its set's title VOBs.
type vobuSpan struct{ sector, sectors int64 }

// soundJoin is a clock shift as its sound is read: which of the title's shifts it is, the VOBUs
// either side of it, the ticks the one before is moved by, the shift, and when it plays from the
// title's start. Only the first maxSoundJoins are read.
type soundJoin struct {
	index         int
	before, after vobuSpan
	ticksBefore   int64
	shift         ClockShift
	atMs          int64
}

// appendSoundBreak reads the sound either side of one of a title's first maxSoundJoins clock
// shifts, from the last sectors of the VOBU before it to the first of the VOBU it starts, and
// appends its break to breaks if the sound does not carry on across it once restamped. A join whose
// sound cannot be read, for want of AC-3 on either side or of a read, is taken as carrying on: the
// check only reports.
func (disc *volume) appendSoundBreak(breaks []SoundBreak, set *titleSet, join soundJoin) []SoundBreak {
	if join.index >= maxSoundJoins {
		return breaks
	}
	tailSectors := min(join.before.sectors, soundSectors)
	tail, err := disc.vobSectors(set, join.before.sector+join.before.sectors-tailSectors, tailSectors)
	if err != nil {
		return breaks
	}
	head, err := disc.vobSectors(set, join.after.sector, min(join.after.sectors, soundSectors))
	if err != nil {
		return breaks
	}
	found, has := soundAcross(tail, head, join.ticksBefore, join.shift.Ticks)
	if !has {
		return breaks
	}
	found.FromByte, found.AtMs = join.shift.FromByte, join.atMs
	return append(breaks, found)
}

// vobSectors reads count sectors from sector of the set's title VOBs.
func (disc *volume) vobSectors(set *titleSet, sector, count int64) ([]byte, error) {
	pieces, err := slice(set.vobs, sector*sectorBytes, count*sectorBytes)
	if err != nil {
		return nil, err
	}
	data := make([]byte, 0, count*sectorBytes)
	for _, piece := range pieces {
		read, err := disc.read(piece.Offset, piece.Length)
		if err != nil {
			return nil, err
		}
		data = append(data, read...)
	}
	return data, nil
}

// soundAcross is the break of the first AC-3 track's sound between tail, the packs before a clock
// shift, and head, the packs after it, if it has one.
func soundAcross(tail, head []byte, ticksBefore, ticksAfter int64) (SoundBreak, bool) {
	substream, found := firstAC3(tail)
	if !found {
		return SoundBreak{}, false
	}
	ending, ok := ac3Ending(ac3Of(tail, substream))
	if !ok {
		return SoundBreak{}, false
	}
	next := ac3Of(head, substream)
	if len(next.starts) == 0 {
		return SoundBreak{}, false
	}
	start := next.starts[0]
	left := ending.left + start.offset // the bytes of the frame cut by the shift, either side of it
	isCutShort := left != 0
	endTicks := ending.endTicks
	if ending.left > 0 && left == ending.leftFrameBytes {
		isCutShort, endTicks = false, endTicks+ac3FrameTicks // a whole frame split across the shift
	}
	gap := signedTicks(start.ticks + ticksAfter - endTicks - ticksBefore)
	if !isCutShort && gap > -soundSlackTicks && gap < soundSlackTicks {
		return SoundBreak{}, false
	}
	return SoundBreak{IsCutShort: isCutShort, GapTicks: gap}, true
}

// signedTicks is a difference of two 33-bit timestamps, taken modulo 2^33, as the nearer way round.
func signedTicks(difference int64) int64 {
	difference &= clockMask
	if difference >= 1<<32 {
		return difference - (1 << 33)
	}
	return difference
}

// ac3Stream is one AC-3 substream's bytes as a run of packs carries them, joined, and where frames
// start that its packets stamp: each at an offset of the bytes, with its PTS.
type ac3Stream struct {
	bytes  []byte
	starts []ac3Start
}

// ac3Start is a stamped frame's start.
type ac3Start struct {
	offset int
	ticks  int64
}

// firstAC3 is the lowest AC-3 substream the packs carry.
func firstAC3(packs []byte) (byte, bool) {
	lowest, found := byte(lastAC3Substream), false
	eachAC3(packs, func(substream byte, _ []byte, _ int, _ int64, _ bool) {
		if substream <= lowest {
			lowest, found = substream, true
		}
	})
	return lowest, found
}

// ac3Of is substream's bytes in the packs, and its stamped frames' starts.
func ac3Of(packs []byte, substream byte) ac3Stream {
	var stream ac3Stream
	eachAC3(packs, func(each byte, data []byte, firstFrame int, ticks int64, isStamped bool) {
		if each != substream {
			return
		}
		if isStamped && firstFrame >= 0 && firstFrame < len(data) {
			stream.starts = append(stream.starts, ac3Start{offset: len(stream.bytes) + firstFrame, ticks: ticks})
		}
		stream.bytes = append(stream.bytes, data...)
	})
	return stream
}

// ac3End is how an AC-3 substream's bytes end: when its last whole frame ends, and the bytes past it
// (left), the start of a frame of leftFrameBytes.
type ac3End struct {
	endTicks       int64
	left           int
	leftFrameBytes int
}

// ac3Ending reads the stream's frames on from its last stamped start to its end.
func ac3Ending(stream ac3Stream) (ac3End, bool) {
	if len(stream.starts) == 0 {
		return ac3End{}, false
	}
	last := stream.starts[len(stream.starts)-1]
	offset, ticks := last.offset, last.ticks
	for offset < len(stream.bytes) {
		frameBytes, ok := ac3FrameBytes(stream.bytes[offset:])
		if !ok {
			return ac3End{}, false
		}
		if offset+frameBytes > len(stream.bytes) {
			return ac3End{endTicks: ticks, left: len(stream.bytes) - offset, leftFrameBytes: frameBytes}, true
		}
		offset, ticks = offset+frameBytes, ticks+ac3FrameTicks
	}
	return ac3End{endTicks: ticks}, true
}

// ac3FrameBytes is the size of the 48 kHz AC-3 frame that frame starts with, by its header.
func ac3FrameBytes(frame []byte) (int, bool) {
	if len(frame) < 5 || binary.BigEndian.Uint16(frame) != ac3Sync || frame[4]>>6 != 0 {
		return 0, false
	}
	// The frame's bit rate by half its frmsizecod (ATSC A/52 Table 5.18); at 48 kHz a frame is four
	// bytes per kb/s.
	kbpsByCode := []int{32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 448, 512, 576, 640}
	code := int(frame[4]&0x3F) >> 1
	if code >= len(kbpsByCode) {
		return 0, false
	}
	return kbpsByCode[code] * 4, true //nolint:mnd // four bytes per kb/s at 48 kHz
}

// eachAC3 calls each with every AC-3 packet of the packs: its substream, its frames' bytes, the
// offset in them of the first frame that starts there (-1 for none), and its PTS when it has one.
// A sector it cannot read as a pack is passed over.
func eachAC3(packs []byte, each func(substream byte, data []byte, firstFrame int, ticks int64, isStamped bool)) {
	for start := 0; start+sectorBytes <= len(packs); start += sectorBytes {
		pack := packs[start : start+sectorBytes]
		if pack[0] != 0 || pack[1] != 0 || pack[2] != 1 || pack[3] != 0xBA || pack[4]>>6 != 1 {
			continue
		}
		packet := 14 + int(pack[13]&0x07)
		for packet+6 <= len(pack) && pack[packet] == 0 && pack[packet+1] == 0 && pack[packet+2] == 1 {
			end := packet + 6 + int(binary.BigEndian.Uint16(pack[packet+4:packet+6]))
			if end > len(pack) {
				break
			}
			if pack[packet+3] == 0xBD {
				ac3Packet(pack[packet:end], each)
			}
			packet = end
		}
	}
}

// ac3Packet calls each with a private stream 1 packet's AC-3, if it carries AC-3: after its PES
// header, a substream number, a frame count and a 16-bit pointer to the first frame's first byte,
// counted from 1 after it (0 for none).
func ac3Packet(packet []byte, each func(substream byte, data []byte, firstFrame int, ticks int64, isStamped bool)) {
	if len(packet) < 9 || packet[6]>>6 != 2 {
		return
	}
	payload := 9 + int(packet[8])
	if payload+4 > len(packet) {
		return
	}
	substream := packet[payload]
	if substream < firstAC3Substream || substream > lastAC3Substream {
		return
	}
	var ticks int64
	isStamped := packet[7]&0x80 != 0 && payload >= 14
	if isStamped {
		field := packet[9:14]
		ticks = int64(field[0]>>1&7)<<30 | int64(field[1])<<22 | int64(field[2]>>1)<<15 | int64(field[3])<<7 |
			int64(field[4]>>1)
	}
	firstFrame := int(binary.BigEndian.Uint16(packet[payload+2:payload+4])) - 1
	each(substream, packet[payload+4:], firstFrame, ticks, isStamped)
}
