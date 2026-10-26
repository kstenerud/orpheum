// SPDX-License-Identifier: AGPL-3.0-or-later

package disc

import (
	"fmt"
)

// How much of a title's start is read to tell whether it is still encrypted: a Blu-ray's first
// AACS aligned unit (32 packets of 192 bytes), and a DVD's first 64 packs.
const (
	alignedUnitBytes  = 6144
	m2tsPacketBytes   = 192
	dvdPacksInspected = 64
)

// checkNotEncrypted refuses a title whose start is still encrypted (D350 §5): a Blu-ray's first
// aligned unit marked by its copy permission bits, or with its packets' sync bytes scrambled
// (AACS leaves a unit's first 16 bytes clear); a DVD whose program stream packets are flagged
// scrambled (CSS).
func checkNotEncrypted(disc *volume, title Title) error {
	inspected := min(title.SizeBytes(), alignedUnitBytes)
	if title.Kind == DVD {
		inspected = min(title.SizeBytes(), dvdPacksInspected*sectorBytes)
	}
	start, err := disc.readTitle(title, inspected)
	if err != nil {
		return err
	}
	if title.Kind == BluRay && isAACSEncrypted(start) || title.Kind == DVD && isCSSScrambled(start) {
		return fmt.Errorf("%w: title %d", ErrEncrypted, title.Number)
	}
	return nil
}

// readTitle reads the first lengthBytes of the title's stream.
func (disc *volume) readTitle(title Title, lengthBytes int64) ([]byte, error) {
	pieces, err := title.Locate(0, lengthBytes)
	if err != nil {
		return nil, err
	}
	data := make([]byte, 0, lengthBytes)
	for _, piece := range pieces {
		read, err := disc.read(piece.Offset, piece.Length)
		if err != nil {
			return nil, err
		}
		data = append(data, read...)
	}
	return data, nil
}

// isAACSEncrypted is whether an M2TS stream's first aligned unit is encrypted: its first packet's
// copy permission indicator set, or a later packet without its sync byte.
func isAACSEncrypted(unit []byte) bool {
	if len(unit) >= m2tsPacketBytes && unit[0]>>6 != 0 {
		return true
	}
	for offset := 0; offset+m2tsPacketBytes <= len(unit); offset += m2tsPacketBytes {
		if unit[offset+4] != 0x47 {
			return true
		}
	}
	return false
}

// isCSSScrambled is whether any of a program stream's packs carries an audio, video or private
// packet whose PES header says it is scrambled.
func isCSSScrambled(stream []byte) bool {
	for pack := 0; pack+sectorBytes <= len(stream); pack += sectorBytes {
		if isPackScrambled(stream[pack : pack+sectorBytes]) {
			return true
		}
	}
	return false
}

// isPackScrambled reads one 2048-byte pack's PES packets for the scrambling control bits.
func isPackScrambled(pack []byte) bool {
	if len(pack) < 14 || pack[0] != 0 || pack[1] != 0 || pack[2] != 1 || pack[3] != 0xBA {
		return false
	}
	offset := 12 // an MPEG-1 pack header
	if pack[4]&0xC0 == 0x40 {
		offset = 14 + int(pack[13]&0x07) // an MPEG-2 pack header and its stuffing
	}
	for offset+7 <= len(pack) && pack[offset] == 0 && pack[offset+1] == 0 && pack[offset+2] == 1 {
		if isPacketScrambled(pack[offset:]) {
			return true
		}
		offset += 6 + (int(pack[offset+4])<<8 | int(pack[offset+5]))
	}
	return false
}

// isPacketScrambled is whether a PES packet of audio, video or private data says, in its MPEG-2
// header, that it is scrambled.
func isPacketScrambled(packet []byte) bool {
	id, flags := packet[3], packet[6]
	isMedia := id == 0xBD || id >= 0xC0 && id <= 0xEF
	return isMedia && flags&0xC0 == 0x80 && flags&0x30 != 0
}
