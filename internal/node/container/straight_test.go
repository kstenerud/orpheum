// SPDX-License-Identifier: AGPL-3.0-or-later

package container

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// emptyBox is an MP4 box of kind holding dataBytes zeros.
func emptyBox(kind string, dataBytes int) []byte {
	header := make([]byte, 8, 8+dataBytes)
	binary.BigEndian.PutUint32(header, uint32(8+dataBytes)) //nolint:gosec // a test's small sizes
	copy(header[4:], kind)
	return append(header, make([]byte, dataBytes)...)
}

// mp4Of is an MP4 of boxes of these kinds, in order.
func mp4Of(kinds ...string) []byte {
	var file []byte
	for _, kind := range kinds {
		file = append(file, emptyBox(kind, 64)...)
	}
	return file
}

func TestReadsStraightThrough_OnlyContainersReadInOnePassFromTheirStart(t *testing.T) {
	t.Parallel()
	transport := bytes.Repeat(append([]byte{transportSync}, make([]byte, transportPacketBytes-1)...), 4)
	for _, check := range []struct {
		name string
		file []byte
		want bool
	}{
		{"a Matroska file", append([]byte{0x1A, 0x45, 0xDF, 0xA3}, make([]byte, 400)...), true},
		{"an MPEG transport stream", transport, true},
		{"an MP4 with its index first", mp4Of("ftyp", "moov", "mdat"), true},
		{"an MP4 with its index at its end", mp4Of("ftyp", "mdat", "moov"), false},
		{"an AVI", append([]byte("RIFF\x00\x00\x00\x00AVI "), make([]byte, 400)...), false},
		{"a file too short to tell", []byte{0x1A}, false},
		{"a box larger than its file", append(emptyBox("ftyp", 16)[:4:4], []byte("moov")...), false},
	} {
		if got := ReadsStraightThrough(bytes.NewReader(check.file), int64(len(check.file))); got != check.want {
			t.Errorf("%s: ReadsStraightThrough = %t; want %t", check.name, got, check.want)
		}
	}
}
