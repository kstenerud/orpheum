// SPDX-License-Identifier: AGPL-3.0-or-later

package disc

import (
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// Kind is what disc an image is of.
type Kind string

// The kinds of disc.
const (
	DVD    Kind = "dvd"
	BluRay Kind = "bluray"
)

// Track is what the disc says of one of its title's audio or subtitle streams, which the stream
// itself does not carry.
type Track struct {
	// ID is the stream's identifier as FFmpeg's demuxer gives it ("id" in ffprobe's output): a
	// DVD's private stream number (0x80 for the first AC-3 track, 0x20 for the first subpicture), a
	// Blu-ray's PID.
	ID int64
	// Kind is "audio" or "subtitle", ffprobe's codec_type.
	Kind string
	// Language is the stream's language as the disc writes it: ISO 639-1 on a DVD ("en"), 639-2 on a
	// Blu-ray ("eng").
	Language string
}

// Title is a disc's main title, its film (D350): the runs of the image that hold its stream, in
// play order, which joined are a program stream (DVD) or an M2TS transport stream (Blu-ray), and
// what the disc says of it.
type Title struct {
	Kind Kind
	// Number is the title's number on a DVD, the playlist's on a Blu-ray (00800.mpls is 800).
	Number     int
	Extents    []Extent
	DurationMs int64
	// ChaptersMs are where its chapters start, ascending, the first at 0.
	ChaptersMs []int64
	Tracks     []Track
}

// SizeBytes is the length of the title's stream.
func (title Title) SizeBytes() int64 {
	var size int64
	for _, extent := range title.Extents {
		size += extent.Length
	}
	return size
}

// Locate is where lengthBytes from offset of the title's stream lie in the image, as runs.
func (title Title) Locate(offset, lengthBytes int64) ([]Extent, error) {
	return slice(title.Extents, offset, lengthBytes)
}

// IsImageName is whether a file's name says it is a disc image: an .iso, whatever its case.
func IsImageName(name string) bool { return strings.EqualFold(path.Ext(name), ".iso") }

// MainTitle reads a DVD's or a Blu-ray's image for its main title (D350 §1): the longest title of
// a DVD, the longest playlist of a Blu-ray. An image that is neither is ErrNotADisc; one whose
// title cannot be played as the node serves it says why (ErrEncrypted, ErrInterleaved); a read
// the storage failed is a *ReadError.
func MainTitle(image io.ReaderAt, sizeBytes int64) (Title, error) {
	volume, err := openVolume(image, sizeBytes)
	if err != nil {
		return Title{}, err
	}
	title, err := volume.mainTitle()
	if err != nil {
		return Title{}, err
	}
	if title.SizeBytes() == 0 {
		return Title{}, fmt.Errorf("%w: its main title holds nothing", ErrNoTitle)
	}
	if err := checkNotEncrypted(volume, title); err != nil {
		return Title{}, err
	}
	return title, nil
}

// mainTitle is a DVD's main title when the volume has VIDEO_TS.IFO, and a Blu-ray's when it has
// BDMV/PLAYLIST.
func (disc *volume) mainTitle() (Title, error) {
	isDVD, err := disc.has("VIDEO_TS/VIDEO_TS.IFO")
	if err != nil {
		return Title{}, err
	}
	if isDVD {
		return dvdMainTitle(disc)
	}
	isBluRay, err := disc.has("BDMV/PLAYLIST")
	if err != nil {
		return Title{}, err
	}
	if !isBluRay {
		return Title{}, fmt.Errorf("%w: neither VIDEO_TS nor BDMV in its volume", ErrNotADisc)
	}
	return blurayMainTitle(disc)
}

// has is whether the volume has a file or directory at name, a path; a failure to read it is its
// error.
func (disc *volume) has(name string) (bool, error) {
	_, err := disc.lookup(name)
	if errors.Is(err, errNotFound) {
		return false, nil
	}
	return err == nil, err
}

// findError keeps a lookup's failure to read as it is, and makes any other a malformed disc.
func findError(what string, err error) error {
	if errors.Is(err, errNotFound) {
		return fmt.Errorf("%w: %s: %w", ErrMalformed, what, err)
	}
	return err
}
