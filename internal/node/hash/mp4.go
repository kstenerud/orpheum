// SPDX-License-Identifier: GPL-3.0-or-later

package hash

import (
	"fmt"
	"io"

	"github.com/kstenerud/orpheum/internal/node/container"
)

// mp4Payload returns the span from the first mdat box's data to the last mdat box's end. Tag
// editors rewrite moov and leave mdat alone (D179 §3, not yet measured); a file with several
// mdat boxes (fragmented MP4) is anchored from the first to the last.
func mp4Payload(file io.ReaderAt, sizeBytes int64) (payloadSpan, error) {
	first, err := container.ReadBox(file, sizeBytes, 0)
	if err != nil || first.Kind != "ftyp" {
		return payloadSpan{}, fmt.Errorf("%w: no ftyp box first", container.ErrNotThisContainer)
	}
	start, end := int64(-1), int64(-1)
	position := int64(0)
	for range container.MaxElementHops {
		if position >= sizeBytes {
			if start < 0 {
				return payloadSpan{}, fmt.Errorf("%w: no mdat box", container.ErrNotThisContainer)
			}
			return payloadSpan{start: start, end: end, anchor: AnchorMP4}, nil
		}
		box, err := container.ReadBox(file, sizeBytes, position)
		if err != nil {
			return payloadSpan{}, err
		}
		if box.Kind == "mdat" {
			if start < 0 {
				start = box.DataStart
			}
			end = box.End
		}
		position = box.End
	}
	return payloadSpan{}, fmt.Errorf("%w: too many top-level boxes", container.ErrNotThisContainer)
}
