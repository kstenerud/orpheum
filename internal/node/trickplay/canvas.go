// SPDX-License-Identifier: AGPL-3.0-or-later

package trickplay

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// webpChunkAt is where a WebP file's first chunk begins, after its RIFF header; webpChunkDataAt is
// where its data begins, after the chunk's four-byte name and four-byte size.
const (
	webpChunkAt     = riffHeaderBytes
	webpChunkDataAt = webpChunkAt + 8
)

// PictureHeight is the height of each picture of a sheet laid in rows: the sheet's canvas height,
// read from its WebP header without decoding it, over its rows. FFmpeg's tile filter lays every
// sheet at the full size, a short last one padded, with no margin, so each picture is the canvas's
// height over the rows exactly; the core and the scrub bar cut the pictures by it (trickplay.md §5).
func PictureHeight(webp []byte, rows int) (int, error) {
	height, err := canvasHeight(webp)
	if err != nil {
		return 0, err
	}
	if rows <= 0 || height%rows != 0 {
		return 0, fmt.Errorf("a sheet %d px high is not %d rows of pictures", height, rows)
	}
	return height / rows, nil
}

// canvasHeight reads a WebP's height from its first chunk: an extended file's canvas (VP8X), or a
// simple file's lossy (VP8) or lossless (VP8L) frame header.
func canvasHeight(webp []byte) (int, error) {
	if len(webp) < webpChunkDataAt+10 || !bytes.Equal(webp[:4], []byte("RIFF")) ||
		!bytes.Equal(webp[8:12], []byte("WEBP")) {
		return 0, fmt.Errorf("a sheet of %d bytes has no WebP header to read its size from", len(webp))
	}
	data := webp[webpChunkDataAt:]
	switch chunk := string(webp[webpChunkAt : webpChunkAt+4]); chunk {
	case "VP8X":
		// Flags and three reserved bytes, the canvas's width less one in 24 bits, its height less one.
		return int(uint32(data[7])|uint32(data[8])<<8|uint32(data[9])<<16) + 1, nil
	case "VP8 ":
		// A key frame's three-byte tag, its start code, then its width and height, 14 bits each.
		if !bytes.Equal(data[3:6], []byte{0x9d, 0x01, 0x2a}) {
			return 0, fmt.Errorf("a sheet's VP8 frame has no key frame's start code: % x", data[3:6])
		}
		return int(binary.LittleEndian.Uint16(data[8:10]) & 0x3fff), nil
	case "VP8L":
		// Its signature, then the width less one in 14 bits and the height less one in the next 14.
		if data[0] != 0x2f {
			return 0, fmt.Errorf("a sheet's VP8L frame has no signature: %#x", data[0])
		}
		return int(binary.LittleEndian.Uint32(data[1:5])>>14&0x3fff) + 1, nil
	default:
		return 0, fmt.Errorf("a sheet's first chunk is %q, which says no size", chunk)
	}
}
