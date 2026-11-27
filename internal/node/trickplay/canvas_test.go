// SPDX-License-Identifier: AGPL-3.0-or-later

package trickplay

import (
	"encoding/binary"
	"testing"
)

// webpWithChunk is a WebP file whose first chunk is name holding data.
func webpWithChunk(name string, data []byte) []byte {
	chunk := append([]byte(name), binary.LittleEndian.AppendUint32(nil, uint32(len(data)))...) //nolint:gosec // short
	chunk = append(chunk, data...)
	size := uint32(4 + len(chunk)) //nolint:gosec // short
	file := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, size)...)
	return append(append(file, "WEBP"...), chunk...)
}

// vp8Sheet is a lossy frame's header of a canvas width × height, as libwebp writes one without alpha.
func vp8Sheet(width, height uint16) []byte {
	data := []byte{0x10, 0x20, 0x00, 0x9d, 0x01, 0x2a}
	data = binary.LittleEndian.AppendUint16(data, width)
	return append(binary.LittleEndian.AppendUint16(data, height), 0, 0)
}

// vp8xSheet is an extended WebP's header of a canvas width × height.
func vp8xSheet(width, height uint32) []byte {
	data := append([]byte{0, 0, 0, 0}, binary.LittleEndian.AppendUint32(nil, width-1)[:3]...)
	return webpWithChunk("VP8X", append(data, binary.LittleEndian.AppendUint32(nil, height-1)[:3]...))
}

// vp8lSheet is a lossless WebP's header of a canvas width × height.
func vp8lSheet(width, height uint32) []byte {
	return webpWithChunk("VP8L", append(binary.LittleEndian.AppendUint32([]byte{0x2f}, (width-1)|(height-1)<<14),
		0, 0, 0, 0, 0))
}

// trickplay.md §5: a sheet's pictures are as high as its canvas over its rows, read from its header
// whichever kind of WebP FFmpeg wrote; a sheet that says no size, or not whole rows, is refused.
func TestPictureHeight_IsTheCanvasHeightOverTheRowsOfEveryKindOfWebP(t *testing.T) {
	t.Parallel()
	for name, sheet := range map[string][]byte{
		"lossy":    webpWithChunk("VP8 ", vp8Sheet(3200, 1800)),
		"extended": vp8xSheet(3200, 1800),
		"lossless": vp8lSheet(3200, 1800),
	} {
		if height, err := PictureHeight(sheet, 10); err != nil || height != 180 {
			t.Errorf("a %s sheet 1800 px high in 10 rows: %d, %v; want pictures 180 px high", name, height, err)
		}
	}
	for name, sheet := range map[string][]byte{
		"no size":           webpOf("twos"),
		"no key frame":      webpWithChunk("VP8 ", make([]byte, 12)),
		"not whole rows":    webpWithChunk("VP8 ", vp8Sheet(3200, 1805)),
		"no lossless start": webpWithChunk("VP8L", make([]byte, 10)),
		"no WebP":           []byte("not a picture at all, nor a sheet of them"),
	} {
		if height, err := PictureHeight(sheet, 10); err == nil {
			t.Errorf("a sheet with %s: pictures %d px high; want it refused", name, height)
		}
	}
}
