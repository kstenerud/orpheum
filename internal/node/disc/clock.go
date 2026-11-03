// SPDX-License-Identifier: AGPL-3.0-or-later

package disc

import (
	"encoding/binary"
	"sort"
)

// A DVD program stream's clock: 90 kHz ticks, 33 bits that wrap.
const (
	clockTicksPerSecond = 90_000
	clockMask           = 1<<33 - 1
	// Up to clockGapTicks either side of where the VOBU before it ended, a VOBU's clock carries on
	// from it: an NTSC frame (3003 ticks), more than a seamless join's rounding leaves, as FFmpeg's
	// dvdvideo demuxer (which allows none) treats any other gap as the clock started again.
	clockGapTicks = 3003
)

// Where a DVD's navigation pack, the first pack of each VOBU, says when its VOBU plays (PCI's
// VOBU_S_PTM and VOBU_E_PTM, as libdvdread names them): after the pack header (14 bytes), the system
// header (24) and the PCI packet's header and substream number (7).
const (
	navStartTicksAt = 0x39
	navEndTicksAt   = 0x3D
	// The DSI packet's VOBU_EA, the sector of the VOBU's last pack counted from its navigation pack:
	// after the PCI packet (to 0x400), the DSI packet's header and substream number (7), its pack's
	// SCR (4) and its own sector (4).
	navLastPackAt = 0x40F
)

// ClockShift is where a title's clock starts again: from FromByte of its stream, every timestamp
// is moved on by Ticks (90 kHz, taken modulo 2^33), so the title plays on one clock.
type ClockShift struct {
	FromByte int64
	Ticks    int64
}

// Restamp moves the timestamps of stream, the title's from offset, by the title's clock shifts, so
// FFmpeg reads one clock however its cells' clocks were set: each pack's system clock reference and
// each packet's presentation and decoding times. The offset is a sector's start; what is not a
// program stream pack is left as it is.
func (title Title) Restamp(stream []byte, offset int64) {
	if len(title.ClockShifts) == 0 {
		return
	}
	for start := 0; start < len(stream); start += sectorBytes {
		ticks := title.shiftAt(offset + int64(start))
		if ticks != 0 {
			restampPack(stream[start:min(start+sectorBytes, len(stream))], ticks)
		}
	}
}

// shiftAt is the ticks the title's clock is moved by at byte offset of its stream.
func (title Title) shiftAt(offset int64) int64 {
	after := sort.Search(len(title.ClockShifts), func(index int) bool {
		return title.ClockShifts[index].FromByte > offset
	})
	if after == 0 {
		return 0
	}
	return title.ClockShifts[after-1].Ticks
}

// restampPack moves one MPEG-2 pack's timestamps by ticks: its SCR, and the PTS and DTS of its
// audio, video and private stream 1 (AC-3, DTS, LPCM, subpicture) packets. A pack it cannot read
// as one is left from the first byte it cannot.
func restampPack(pack []byte, ticks int64) {
	if len(pack) < 14 || pack[0] != 0 || pack[1] != 0 || pack[2] != 1 || pack[3] != 0xBA || pack[4]>>6 != 1 {
		return
	}
	restampSCR(pack[4:10], ticks)
	packet := 14 + int(pack[13]&0x07)
	for packet+6 <= len(pack) && pack[packet] == 0 && pack[packet+1] == 0 && pack[packet+2] == 1 {
		id := pack[packet+3]
		end := packet + 6 + int(binary.BigEndian.Uint16(pack[packet+4:packet+6]))
		if end > len(pack) {
			return
		}
		if id == 0xBD || id >= 0xC0 && id <= 0xEF {
			restampPacket(pack[packet:end], ticks)
		}
		packet = end
	}
}

// restampPacket moves an MPEG-2 PES packet's PTS, and its DTS when it has one, by ticks.
func restampPacket(packet []byte, ticks int64) {
	if len(packet) < 9 || packet[6]>>6 != 2 {
		return
	}
	flags := packet[7] >> 6
	if flags&2 != 0 && len(packet) >= 14 {
		restampTimestamp(packet[9:14], ticks)
	}
	if flags == 3 && len(packet) >= 19 {
		restampTimestamp(packet[14:19], ticks)
	}
}

// The bits of a pack header's six SCR bytes, read as one 48-bit word, that hold the SCR base: 3
// bits, a marker, 15 bits, a marker, 15 bits, then a marker and the extension, which stay.
const (
	scrHigh, scrMiddle, scrLow = 0x3800_0000_0000, 0x03FF_F800_0000, 0x0000_03FF_F800
	// The bits of a PTS's or DTS's five bytes, read as one 40-bit word, that hold it: a four-bit
	// prefix, 3 bits, a marker, 15 bits, a marker, 15 bits and a marker.
	timestampHigh, timestampMiddle, timestampLow = 0x0E_0000_0000, 0x00_FFFE_0000, 0x00_0000_FFFE
)

// restampSCR moves the SCR base of a pack header's six SCR bytes by ticks.
func restampSCR(field []byte, ticks int64) {
	word := readWord(field)
	scr := (word&scrHigh)>>13 | (word&scrMiddle)>>12 | (word&scrLow)>>11
	scr = moved(scr, ticks)
	word = word&^(scrHigh|scrMiddle|scrLow) | scr<<13&scrHigh | scr<<12&scrMiddle | scr<<11&scrLow
	writeWord(field, word)
}

// restampTimestamp moves a PES packet's five-byte PTS or DTS by ticks.
func restampTimestamp(field []byte, ticks int64) {
	word := readWord(field)
	timestamp := (word&timestampHigh)>>3 | (word&timestampMiddle)>>2 | (word&timestampLow)>>1
	timestamp = moved(timestamp, ticks)
	word = word&^(timestampHigh|timestampMiddle|timestampLow) |
		timestamp<<3&timestampHigh | timestamp<<2&timestampMiddle | timestamp<<1&timestampLow
	writeWord(field, word)
}

// moved is a 33-bit timestamp moved by ticks, modulo 2^33.
func moved(timestamp uint64, ticks int64) uint64 {
	return (timestamp + uint64(ticks&clockMask)) & clockMask
}

// readWord reads field, at most eight bytes, as a big-endian number.
func readWord(field []byte) uint64 {
	var word [8]byte
	copy(word[8-len(field):], field)
	return binary.BigEndian.Uint64(word[:])
}

// writeWord writes word's last len(field) bytes into field, big-endian.
func writeWord(field []byte, word uint64) {
	var bytes [8]byte
	binary.BigEndian.PutUint64(bytes[:], word)
	copy(field, bytes[8-len(field):])
}

// vobuClock is when a VOBU plays, by its navigation pack: from start to end, when known, and how
// many sectors it spans.
type vobuClock struct {
	start, end int64
	sectors    int64
	known      bool
}

// navClock is when the VOBU a navigation pack starts plays, unknown for a sector that is no
// navigation pack.
func navClock(sector []byte) vobuClock {
	if len(sector) < navLastPackAt+4 || sector[3] != 0xBA || sector[17] != 0xBB ||
		sector[41] != 0xBF || sector[44] != 0 || sector[0x403] != 0xBF || sector[0x406] != 1 {
		return vobuClock{}
	}
	return vobuClock{
		start:   int64(binary.BigEndian.Uint32(sector[navStartTicksAt:])),
		end:     int64(binary.BigEndian.Uint32(sector[navEndTicksAt:])),
		sectors: int64(binary.BigEndian.Uint32(sector[navLastPackAt:])) + 1, known: true,
	}
}

// clockCell is a cell of a DVD title as its clock is read: where it starts in the title's stream,
// and its first and last sectors of its set's title VOBs.
type clockCell struct {
	titleByte               int64
	firstSector, lastSector int64
}

// newClockCell is cell of a program chain, starting at titleByte of its title's stream.
func newClockCell(cell dvdCell, titleByte int64) clockCell {
	return clockCell{titleByte: titleByte, firstSector: cell.firstSector, lastSector: cell.lastSector}
}

// dvdClockShifts are where a title's clock starts again: a VOBU, in a cell or at a cell's start,
// that plays from more than a frame before or after the VOBU before it ended is moved to carry on
// from it, as FFmpeg's dvdvideo demuxer offsets it. The VOBUs are found from each cell's first by
// their navigation packs; where one cannot be read, the cell's rest keeps its clock as it is and
// the next VOBU read starts the comparison afresh. At the first maxSoundJoins shifts it reads the
// sound either side, and gives where it does not carry on (D405 §4).
func (disc *volume) dvdClockShifts(set *titleSet, cells []clockCell) ([]ClockShift, []SoundBreak, error) {
	var shifts []ClockShift
	var breaks []SoundBreak
	var ticks, previousEnd, titleStart int64
	var previous vobuSpan
	carriesOn, started := false, false
	for _, cell := range cells {
		for sector := cell.firstSector; sector <= cell.lastSector; {
			vobu, err := disc.vobuClock(set, sector)
			if err != nil {
				return nil, nil, err
			}
			if !vobu.known {
				carriesOn = false
				break
			}
			if !started {
				titleStart, started = vobu.start, true
			}
			if gap := vobu.start + ticks - previousEnd; carriesOn && (gap < -clockGapTicks || gap > clockGapTicks) {
				shift := ClockShift{
					FromByte: cell.titleByte + (sector-cell.firstSector)*sectorBytes, Ticks: previousEnd - vobu.start,
				}
				breaks = disc.appendSoundBreak(breaks, set, soundJoin{
					index: len(shifts), before: previous, after: vobuSpan{sector, vobu.sectors}, ticksBefore: ticks,
					shift: shift, atMs: (previousEnd - titleStart) * 1000 / clockTicksPerSecond,
				})
				ticks = shift.Ticks
				shifts = append(shifts, shift)
			}
			previousEnd, carriesOn, previous = vobu.end+ticks, true, vobuSpan{sector, vobu.sectors}
			sector += vobu.sectors
		}
	}
	return shifts, breaks, nil
}

// vobuClock reads the navigation pack at sector of the set's title VOBs for when its VOBU plays.
func (disc *volume) vobuClock(set *titleSet, sector int64) (vobuClock, error) {
	pieces, err := slice(set.vobs, sector*sectorBytes, sectorBytes)
	if err != nil {
		return vobuClock{}, err
	}
	data := make([]byte, 0, sectorBytes)
	for _, piece := range pieces {
		read, err := disc.read(piece.Offset, piece.Length)
		if err != nil {
			return vobuClock{}, err
		}
		data = append(data, read...)
	}
	return navClock(data), nil
}
