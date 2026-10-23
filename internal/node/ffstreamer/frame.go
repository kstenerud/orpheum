// SPDX-License-Identifier: AGPL-3.0-or-later

package ffstreamer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The message types (the specification's Message Types).
const (
	typeHello      int8 = 1
	typeCapability int8 = 2
	typeRequest    int8 = 3
	typeRead       int8 = 4
	typeData       int8 = 5
	typeDone       int8 = 6
)

// The frame and its fixed bodies' lengths in bytes (Frames, Messages).
const (
	headerBytes     = 6
	helloBytes      = 4
	capabilityBytes = 9
	requestBytes    = 27
	readBytes       = 12
	doneOKBytes     = 9
	doneErrFixed    = 16
	longestMessage  = 1024
	doneLongest     = doneErrFixed + longestMessage
	mostOutstanding = 16
	sequenceModulus = 128
)

// The versions the node speaks: 1 alone.
const (
	lowestVersion  = 1
	highestVersion = 1
)

// DONE's results and its error kinds (DONE).
const (
	resultOK  = 0
	resultErr = 1
)

// Kind is what a DONE that failed says went wrong on its channel.
type Kind int8

// The kinds.
const (
	KindTerms       Kind = 1
	KindChannel     Kind = 2
	KindProtocol    Kind = 3
	KindInterrupted Kind = 4
)

func (kind Kind) String() string {
	switch kind {
	case KindTerms:
		return "terms"
	case KindChannel:
		return "channel"
	case KindProtocol:
		return "protocol"
	case KindInterrupted:
		return "interrupted"
	}
	return fmt.Sprintf("kind %d", int8(kind))
}

// ErrViolation is a frame from FFmpeg that breaks the protocol. FFmpeg may have been taken over by
// the film it reads, so a violation ends the run (Serving).
var ErrViolation = errors.New("FFmpeg broke the ffstreamer protocol")

// header is one frame's header.
type header struct {
	kind     int8
	sequence int8
	length   int32
}

func decodeHeader(raw []byte) header {
	return header{
		kind: int8Of(raw[0]), sequence: int8Of(raw[1]),
		length: int32(binary.LittleEndian.Uint32(raw[2:6])), //nolint:gosec // G115: the wire's signed field
	}
}

// frame is a frame's bytes: its header, then body.
func frame(kind, sequence int8, body []byte) []byte {
	length := int32(len(body)) //nolint:gosec // G115: a body is bounded by its type, far below 2 GiB
	return append(put(put(put(make([]byte, 0, headerBytes+len(body)), kind), sequence), length), body...)
}

// signed are the integers the protocol writes: signed, of a fixed size (Frames).
type signed interface {
	~int8 | ~int16 | ~int32 | ~int64
}

// put appends value to raw, little-endian, as the protocol writes every field (Frames).
func put[T signed](raw []byte, value T) []byte {
	//nolint:errcheck // binary.Append fails only for a type of no fixed size, which signed rules out
	out, _ := binary.Append(raw, binary.LittleEndian, value)
	return out
}

// bodyBounds are the lengths a body of each type FFmpeg sends may have, checked before it is read.
func bodyBounds(kind int8) (low, high int32, isKnown bool) {
	switch kind {
	case typeHello:
		return helloBytes, helloBytes, true
	case typeCapability:
		return capabilityBytes, capabilityBytes, true
	case typeRead:
		return readBytes, readBytes, true
	case typeDone:
		return doneOKBytes, doneLongest, true
	}
	return 0, 0, false
}

// checkHeader is whether a header FFmpeg sent may be read on: a known type from FFmpeg, the number
// expected next, and a body within its type's bounds (Frames, Sequence Numbers).
func checkHeader(got header, expectedSequence int8) error {
	if got.sequence < 0 || got.length < 0 {
		return fmt.Errorf("%w: a negative field in a header: sequence %d, length %d", ErrViolation, got.sequence,
			got.length)
	}
	if got.sequence != expectedSequence {
		return fmt.Errorf("%w: sequence %d, expected %d", ErrViolation, got.sequence, expectedSequence)
	}
	low, high, isKnown := bodyBounds(got.kind)
	if !isKnown {
		return fmt.Errorf("%w: a frame of type %d from FFmpeg", ErrViolation, got.kind)
	}
	if got.length < low || got.length > high {
		return fmt.Errorf("%w: type %d with a body of %d bytes", ErrViolation, got.kind, got.length)
	}
	return nil
}

// versionRange is a HELLO's body: the lowest and highest versions its sender speaks.
type versionRange struct{ lowest, highest int16 }

func decodeHello(body []byte) versionRange {
	return versionRange{lowest: int16Of(body[0:2]), highest: int16Of(body[2:4])}
}

func helloBody(versions versionRange) []byte {
	return put(put(make([]byte, 0, helloBytes), versions.lowest), versions.highest)
}

// agreedVersion is the version two ranges share, the highest, or an error when either is invalid
// or they share none (HELLO).
func agreedVersion(theirs, ours versionRange) (int16, error) {
	if theirs.lowest < 1 || theirs.highest < theirs.lowest {
		return 0, fmt.Errorf("%w: an invalid range of versions, %d to %d", ErrViolation, theirs.lowest, theirs.highest)
	}
	version := min(theirs.highest, ours.highest)
	if version < max(theirs.lowest, ours.lowest) {
		return 0, fmt.Errorf("%w: no version shared: FFmpeg speaks %d to %d, the node %d to %d", ErrViolation,
			theirs.lowest, theirs.highest, ours.lowest, ours.highest)
	}
	return version, nil
}

// capability is FFmpeg's CAPABILITY.
type capability struct {
	preferredBlock, largestBody int32
	outstanding                 int8
}

func decodeCapability(body []byte) (capability, error) {
	got := capability{
		preferredBlock: int32Of(body[0:4]), largestBody: int32Of(body[4:8]),
		outstanding: int8Of(body[8]),
	}
	if got.preferredBlock < 1 || got.largestBody < got.preferredBlock || got.outstanding < 1 ||
		got.outstanding > mostOutstanding {
		return capability{}, fmt.Errorf("%w: a CAPABILITY outside its rules: block %d, largest body %d, outstanding %d",
			ErrViolation, got.preferredBlock, got.largestBody, got.outstanding)
	}
	return got, nil
}

// Terms are the parent's REQUEST: the run's identifier, the file's size, and how FFmpeg may read
// it (REQUEST).
type Terms struct {
	Version     int16
	Run         int64
	FileSize    int64
	BlockSize   int32
	LongestRead int32
	Outstanding int8
}

func requestBody(terms Terms) []byte {
	raw := put(put(put(make([]byte, 0, requestBytes), terms.Version), terms.Run), terms.FileSize)
	return put(put(put(raw, terms.BlockSize), terms.LongestRead), terms.Outstanding)
}

// readRequest is one READ: a range of the file.
type readRequest struct {
	sequence int8
	offset   int64
	length   int32
}

// decodeRead is a READ's range, checked against the terms without arithmetic that can overflow: a
// bound is checked by subtraction from a known non-negative value, never by addition (Checks Without Overflow).
func decodeRead(sequence int8, body []byte, terms Terms) (readRequest, error) {
	got := readRequest{sequence: sequence, offset: int64Of(body[0:8]), length: int32Of(body[8:12])}
	if got.offset < 0 || got.offset > terms.FileSize || got.length <= 0 || got.length > terms.LongestRead ||
		int64(got.length) > terms.FileSize-got.offset {
		return readRequest{}, fmt.Errorf("%w: a READ outside the file: offset %d, length %d, file %d, longest %d",
			ErrViolation, got.offset, got.length, terms.FileSize, terms.LongestRead)
	}
	return got, nil
}

// Done is FFmpeg's account of its channel (DONE): the run it echoes, whether it saw a fault, and
// when it did, the kind, FFmpeg's error code and its message, made safe to log.
type Done struct {
	Run     int64
	IsOK    bool
	Kind    Kind
	Code    int32
	Message string
}

func decodeDone(body []byte) (Done, error) {
	done := Done{Run: int64Of(body[0:8])}
	switch body[8] {
	case resultOK:
		if len(body) != doneOKBytes {
			return Done{}, fmt.Errorf("%w: a DONE that says OK with a body of %d bytes", ErrViolation, len(body))
		}
		done.IsOK = true
		return done, nil
	case resultErr:
	default:
		return Done{}, fmt.Errorf("%w: a DONE with result %d", ErrViolation, body[8])
	}
	if len(body) < doneErrFixed {
		return Done{}, fmt.Errorf("%w: a DONE that failed with a body of %d bytes", ErrViolation, len(body))
	}
	done.Kind = Kind(int8Of(body[9]))
	done.Code = int32Of(body[10:14])
	messageLength := int(int16Of(body[14:16]))
	if done.Kind < KindTerms || done.Kind > KindInterrupted || done.Code < 0 || done.Run < 0 ||
		messageLength < 0 || messageLength > longestMessage || len(body) != doneErrFixed+messageLength {
		return Done{}, fmt.Errorf("%w: a malformed DONE: kind %d, code %d, message of %d bytes in a body of %d",
			ErrViolation, done.Kind, done.Code, messageLength, len(body))
	}
	done.Message = Sanitised(body[doneErrFixed:])
	return done, nil
}

// Sanitised is text from FFmpeg made safe to log: decoded as UTF-8 with replacement, and every
// character in the Unicode categories Cc and Cf escaped, which takes in the C0 and C1 controls, CSI in
// either form, DEL and the bidirectional overrides (Logging Text).
func Sanitised(raw []byte) string {
	var safe strings.Builder
	for len(raw) > 0 {
		character, size := utf8.DecodeRune(raw)
		raw = raw[size:]
		if unicode.In(character, unicode.Cc, unicode.Cf) {
			fmt.Fprintf(&safe, "\\u%04x", character)
			continue
		}
		safe.WriteRune(character)
	}
	return safe.String()
}

// nextSequence is the number after sequence, wrapping from 127 to 0 (Sequence Numbers).
func nextSequence(sequence int8) int8 {
	return int8((int(sequence) + 1) % sequenceModulus)
}

func int8Of(raw byte) int8 { return int8(raw) } //nolint:gosec // G115: the wire's signed byte

func int16Of(raw []byte) int16 { return int16(binary.LittleEndian.Uint16(raw)) } //nolint:gosec // G115: signed wire

func int32Of(raw []byte) int32 { return int32(binary.LittleEndian.Uint32(raw)) } //nolint:gosec // G115: signed wire

func int64Of(raw []byte) int64 { return int64(binary.LittleEndian.Uint64(raw)) } //nolint:gosec // G115: signed wire
