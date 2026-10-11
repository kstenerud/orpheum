// SPDX-License-Identifier: AGPL-3.0-or-later

package container

import (
	"bytes"
	"encoding/binary"
	"errors"
	"slices"
	"testing"
)

// element writes one Matroska element with an 8-byte size, so its length never depends on its
// contents'.
func element(id uint64, children ...[]byte) []byte {
	out := bytes.TrimLeft(binary.BigEndian.AppendUint64(nil, id), "\x00")
	size := 0
	for _, part := range children {
		size += len(part)
	}
	sizeAt := len(out)
	out = binary.BigEndian.AppendUint64(out, uint64(size))
	out[sizeAt] = 0x01 // the 8-byte length marker
	for _, part := range children {
		out = append(out, part...)
	}
	return out
}

// uintElement is an element holding an 8-byte unsigned integer.
func uintElement(id, value uint64) []byte {
	return element(id, binary.BigEndian.AppendUint64(nil, value))
}

// cue is a CuePoint at time on each of tracks.
func cue(time uint64, tracks ...uint64) []byte {
	parts := [][]byte{uintElement(IDCueTime, time)}
	for _, track := range tracks {
		parts = append(parts, element(IDCueTrackPositions, uintElement(IDCueTrack, track),
			uintElement(IDCueClusterPosition, 0)))
	}
	return element(IDCuePoint, parts...)
}

// matroskaFile is a Matroska file whose first track, number 1, is sound and second, number 2, a
// picture, ticking every scale nanoseconds, with cues after its one cluster, which the SeekHead names.
func matroskaFile(scale uint64, cues []byte) []byte {
	header := element(IDEBML)
	info := element(IDInfo, uintElement(IDTimecodeScale, scale))
	tracks := element(IDTracks,
		element(IDTrackEntry, uintElement(IDTrackNumber, 1), uintElement(IDTrackType, 2)),
		element(IDTrackEntry, uintElement(IDTrackNumber, 2), uintElement(IDTrackType, matroskaVideoTrackType)))
	cluster := element(IDCluster, bytes.Repeat([]byte{0x42}, 64))
	seekHeadLength := len(element(IDSeekHead, element(IDSeek, uintElement(IDSeekID, IDCues),
		uintElement(IDSeekPosition, 0))))
	cuesAt := uint64(seekHeadLength + len(info) + len(tracks) + len(cluster))
	seekHead := element(IDSeekHead, element(IDSeek, uintElement(IDSeekID, IDCues), uintElement(IDSeekPosition, cuesAt)))
	body := slices.Concat(seekHead, info, tracks, cluster)
	if cues != nil {
		body = append(body, cues...)
	}
	return append(header, element(IDSegment, body)...)
}

// box is an MP4 box of kind holding parts.
func box(kind string, parts ...[]byte) []byte {
	payload := slices.Concat(parts...)
	out := binary.BigEndian.AppendUint32(nil, uint32(8+len(payload))) //nolint:gosec // a test's boxes are small
	return append(append(out, kind...), payload...)
}

// fullBox is a version 0 box of kind holding words.
func fullBox(kind string, words ...uint32) []byte {
	payload := []byte{0, 0, 0, 0}
	for _, word := range words {
		payload = binary.BigEndian.AppendUint32(payload, word)
	}
	return box(kind, payload)
}

// mp4File is an MP4 whose first trak is sound and second a picture of ten samples of 512 ticks in a
// timescale of 12800, so 40 ms each, each shown 1024 ticks after it is decoded, with an edit that
// starts the media at 1024; tables are its picture's stbl boxes besides stts and ctts.
func mp4File(tables ...[]byte) []byte {
	sound := box("trak", box("mdia", fullBox("hdlr", 0, binary.BigEndian.Uint32([]byte("soun")), 0, 0, 0)))
	stbl := box("stbl", slices.Concat(
		[][]byte{fullBox("stts", 1, 10, 512), fullBox("ctts", 1, 10, 1024)}, tables)...)
	picture := box("trak",
		box("edts", fullBox("elst", 1, 4000, 1024, 0x00010000)),
		box("mdia",
			fullBox("mdhd", 0, 0, 12800, 5120, 0),
			fullBox("hdlr", 0, binary.BigEndian.Uint32([]byte("vide")), 0, 0, 0),
			box("minf", stbl)))
	return slices.Concat(box("ftyp", []byte("isom\x00\x00\x02\x00")), box("moov", sound, picture),
		box("mdat", bytes.Repeat([]byte{0x42}, 64)))
}

func keyframesOf(t *testing.T, file []byte, streamIndex int) ([]int64, error) {
	t.Helper()
	return KeyframesMs(bytes.NewReader(file), int64(len(file)), streamIndex)
}

// A Matroska file's keyframes are its cues on the picture's track, in its ticks as milliseconds;
// a cue only on the sound's track is not one, and the sound's track has none (D259 §2).
func TestKeyframesMs_MatroskaCuesAreThePicturesKeyframes(t *testing.T) {
	t.Parallel()
	cues := element(IDCues, cue(0, 1, 2), cue(1000, 1), cue(2002, 2), cue(4004, 1, 2))
	got, err := keyframesOf(t, matroskaFile(1_000_000, cues), 1)
	if err != nil || !slices.Equal(got, []int64{0, 2002, 4004}) {
		t.Errorf("the picture's keyframes = %v, %v; want 0, 2002 and 4004 ms", got, err)
	}
	tenths := element(IDCues, cue(0, 2), cue(20_020, 2))
	if got, err = keyframesOf(t, matroskaFile(100_000, tenths), 1); err != nil || !slices.Equal(got, []int64{0, 2002}) {
		t.Errorf("with ticks of a tenth of a millisecond, the keyframes = %v, %v; want 0 and 2002 ms", got, err)
	}
	if got, err = keyframesOf(t, matroskaFile(1_000_000, cues), 0); !errors.Is(err, ErrNoIndex) {
		t.Errorf("the sound's track's keyframes = %v, %v; want ErrNoIndex", got, err)
	}
}

// An MP4's keyframes are its picture's sync samples, each shown at its decode time plus its
// composition offset, less where the edit starts the media (D259 §2): samples 1 and 6 at 0 and
// 200 ms.
func TestKeyframesMs_MP4SyncSamplesAreThePicturesKeyframes(t *testing.T) {
	t.Parallel()
	got, err := keyframesOf(t, mp4File(fullBox("stss", 2, 1, 6)), 1)
	if err != nil || !slices.Equal(got, []int64{0, 200}) {
		t.Errorf("the picture's keyframes = %v, %v; want 0 and 200 ms", got, err)
	}
	if got, err = keyframesOf(t, mp4File(fullBox("stss", 2, 1, 6)), 0); !errors.Is(err, ErrNoIndex) {
		t.Errorf("the sound's track's keyframes = %v, %v; want ErrNoIndex", got, err)
	}
}

// A file with no index of its keyframes says so: Matroska without cues, an MP4 without a
// sync-sample table, and anything else.
func TestKeyframesMs_AFileWithNoIndexSaysSo(t *testing.T) {
	t.Parallel()
	for what, file := range map[string][]byte{
		"a Matroska file without cues": matroskaFile(1_000_000, nil),
		"an MP4 without stss":          mp4File(),
		"an AVI":                       append([]byte("RIFF\x00\x10\x00\x00AVI LIST"), bytes.Repeat([]byte{0}, 64)...),
		"nothing":                      nil,
	} {
		if got, err := keyframesOf(t, file, 1); !errors.Is(err, ErrNoIndex) {
			t.Errorf("%s: %v, %v; want ErrNoIndex", what, got, err)
		}
	}
}

// No file panics the reader, and none has no keyframes without saying why.
func FuzzKeyframesMs(f *testing.F) {
	f.Add(matroskaFile(1_000_000, element(IDCues, cue(0, 2), cue(2002, 2))))
	f.Add(mp4File(fullBox("stss", 2, 1, 6)))
	f.Fuzz(func(t *testing.T, file []byte) {
		keyframes, err := KeyframesMs(bytes.NewReader(file), int64(len(file)), 1)
		if err == nil && len(keyframes) == 0 {
			t.Error("no keyframes, and no error saying why")
		}
	})
}

// A ctts table's offsets are found for every sync sample in one walk: each sample in its run, 0
// past the table's end, a sample out of order found from the table's start; and the table is parsed
// once, however many keyframes, where it was parsed for each (the M4 closing audit, 2026-10-08).
// AllocsPerRun panics in a parallel test.
//
//nolint:paralleltest // as above
func TestCompositionOffsets_FindsEachSamplesRunInOneWalk(t *testing.T) {
	// Samples 1-2 shown 10 ticks late, 3-5 5 ticks early (a version 1, signed, table), then none.
	ctts := []byte{1, 0, 0, 0}
	for _, word := range []uint32{2, 2, 10, 3, 0xFFFFFFFB} {
		ctts = binary.BigEndian.AppendUint32(ctts, word)
	}
	got := compositionOffsets(ctts, []uint32{1, 2, 3, 5, 6, 2})
	if want := []int64{10, 10, -5, -5, 0, 10}; !slices.Equal(got, want) {
		t.Errorf("the offsets = %v; want %v", got, want)
	}
	many := make([]uint32, 5000)
	for index := range many {
		many[index] = uint32(index + 1)
	}
	if allocations := testing.AllocsPerRun(5, func() { compositionOffsets(ctts, many) }); allocations > 2 {
		t.Errorf("finding 5000 samples' offsets allocated %v times; want the table parsed once", allocations)
	}
}
