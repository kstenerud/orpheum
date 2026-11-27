// SPDX-License-Identifier: AGPL-3.0-or-later

package trickplay

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// riffHeaderBytes is a WebP file's header: "RIFF", the size of what follows the size, "WEBP".
const riffHeaderBytes = 12

// maxSheetBytes bounds one sheet: a 10×10 sheet of 320 px pictures at quality 60 is a few hundred
// KiB, and a header claiming far more is not a sheet.
const maxSheetBytes = 32 << 20

// sheetSplitter cuts FFmpeg's stdout, WebP files one after another, into its sheets, by each file's
// RIFF header, and gives each to made with its index, counted from next. Any error it meets it gives
// to stop too, which ends the run: FFmpeg would otherwise block on a pipe no one reads.
type sheetSplitter struct {
	next    int
	made    func(index int, webp []byte) error
	stop    func(error)
	pending []byte
}

// Write takes stdout as it comes, giving each sheet it completes to made.
func (splitter *sheetSplitter) Write(written []byte) (int, error) {
	count, err := splitter.split(written)
	if err != nil {
		splitter.stop(err)
	}
	return count, err
}

func (splitter *sheetSplitter) split(written []byte) (int, error) {
	splitter.pending = append(splitter.pending, written...)
	for len(splitter.pending) >= riffHeaderBytes {
		header := splitter.pending[:riffHeaderBytes]
		if !bytes.Equal(header[:4], []byte("RIFF")) || !bytes.Equal(header[8:12], []byte("WEBP")) {
			return 0, fmt.Errorf("sheet %d does not start as a WebP: % x", splitter.next, header)
		}
		size := int64(binary.LittleEndian.Uint32(header[4:8])) + 8
		size += size & 1 // a RIFF chunk is padded to even
		if size > maxSheetBytes {
			return 0, fmt.Errorf("sheet %d says it is %d bytes, more than a sheet's %d", splitter.next, size,
				maxSheetBytes)
		}
		if int64(len(splitter.pending)) < size {
			break
		}
		sheet := bytes.Clone(splitter.pending[:size])
		splitter.pending = splitter.pending[size:]
		if err := splitter.made(splitter.next, sheet); err != nil {
			return 0, err
		}
		splitter.next++
	}
	return len(written), nil
}

// finish fails if stdout ended partway through a sheet.
func (splitter *sheetSplitter) finish() error {
	if len(splitter.pending) > 0 {
		return fmt.Errorf("FFmpeg's output ended %d bytes into sheet %d", len(splitter.pending), splitter.next)
	}
	return nil
}
