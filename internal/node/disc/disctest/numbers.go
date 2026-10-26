// SPDX-License-Identifier: AGPL-3.0-or-later

package disctest

import "encoding/binary"

// The writer's numbers are its own and small, a test's sizes and counts, so narrowing one to a
// field's width loses nothing: these helpers hold the conversions in one place.

func putLE16(data []byte, value int) {
	binary.LittleEndian.PutUint16(data, uint16(value)) //nolint:gosec // G115: see above
}

func putLE32(data []byte, value int) {
	binary.LittleEndian.PutUint32(data, uint32(value)) //nolint:gosec // G115: see above
}

func putLE64(data []byte, value int64) {
	binary.LittleEndian.PutUint64(data, uint64(value)) //nolint:gosec // G115: see above
}

func putBE16(data []byte, value int) {
	binary.BigEndian.PutUint16(data, uint16(value)) //nolint:gosec // G115: see above
}

func putBE32(data []byte, value int) {
	binary.BigEndian.PutUint32(data, uint32(value)) //nolint:gosec // G115: see above
}

func appendBE16(data []byte, value int) []byte {
	return binary.BigEndian.AppendUint16(data, uint16(value)) //nolint:gosec // G115: see above
}

func appendBE32(data []byte, value int) []byte {
	return binary.BigEndian.AppendUint32(data, uint32(value)) //nolint:gosec // G115: see above
}

func octet(value int) byte {
	return byte(value) //nolint:gosec // G115: see above
}
