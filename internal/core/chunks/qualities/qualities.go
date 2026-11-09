// SPDX-License-Identifier: AGPL-3.0-or-later

package qualities

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Quality is one of the lower qualities a film's picture is encoded to for a slow link (D247 §6):
// its average bit rate, whose peaks are capped at one and a half times it, and its picture's
// height, the width following the source's shape.
type Quality struct {
	Kbps   int
	Height int
}

// The bounds of a quality. A bit rate under 100 kb/s shows nothing a person can watch, and one over
// 50 Mb/s is more than any source the server holds carries; a picture under 144 lines is a
// thumbnail, and one over 2160 is past 4K. A height is even, as H.264's 4:2:0 picture needs.
const (
	MinKbps   = 100
	MaxKbps   = 50_000
	MinHeight = 144
	MaxHeight = 2160
)

// ErrNotQualities is a value that is not a list of qualities.
var ErrNotQualities = errors.New("not a list of qualities, highest first, such as 3000k@720p,1200k@480p")

// qualityPattern is one quality as the setting writes it: 1200k@480p.
var qualityPattern = regexp.MustCompile(`^([1-9]\d{0,5})k@([1-9]\d{0,4})p$`)

// Parse reads a list of qualities as the setting chunkQualities holds it: comma-separated
// <kbps>k@<height>p, highest first, such as 3000k@720p,2000k@540p. It is the one check of the
// setting's value and the one reading of it: anything not of that shape, out of bounds, written
// twice or not highest first by both bit rate and height fails with ErrNotQualities.
func Parse(text string) ([]Quality, error) {
	var all []Quality
	for written := range strings.SplitSeq(text, ",") {
		quality, err := parseOne(written)
		if err != nil {
			return nil, err
		}
		if len(all) > 0 {
			before := all[len(all)-1]
			if quality.Kbps >= before.Kbps || quality.Height > before.Height {
				return nil, fmt.Errorf("%w: %q is not lower than the quality before it", ErrNotQualities, written)
			}
		}
		all = append(all, quality)
	}
	return all, nil
}

// parseOne reads one quality, <kbps>k@<height>p, within its bounds.
func parseOne(written string) (Quality, error) {
	match := qualityPattern.FindStringSubmatch(written)
	if match == nil {
		return Quality{}, fmt.Errorf("%w: %q", ErrNotQualities, written)
	}
	kbps, kbpsErr := strconv.Atoi(match[1])
	height, heightErr := strconv.Atoi(match[2])
	switch {
	case kbpsErr != nil || heightErr != nil:
		return Quality{}, fmt.Errorf("%w: %q", ErrNotQualities, written)
	case kbps < MinKbps || kbps > MaxKbps:
		return Quality{}, fmt.Errorf("%w: %q's bit rate is not within %d to %d kb/s", ErrNotQualities, written,
			MinKbps, MaxKbps)
	case height < MinHeight || height > MaxHeight || height%2 != 0:
		return Quality{}, fmt.Errorf("%w: %q's height is not an even number of lines within %d to %d",
			ErrNotQualities, written, MinHeight, MaxHeight)
	}
	return Quality{Kbps: kbps, Height: height}, nil
}

// IsValid is whether text is a list of qualities Parse reads: the setting's check.
func IsValid(text string) bool {
	_, err := Parse(text)
	return err == nil
}
