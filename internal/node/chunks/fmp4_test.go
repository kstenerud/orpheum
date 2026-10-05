// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"encoding/binary"
	"errors"
	"testing"
)

// boxOf is an ISO BMFF box of kind holding payload.
func boxOf(kind string, payload ...[]byte) []byte {
	size := 8
	for _, each := range payload {
		size += len(each)
	}
	box := binary.BigEndian.AppendUint32(nil, uint32(size))
	box = append(box, kind...)
	for _, each := range payload {
		box = append(box, each...)
	}
	return box
}

// initOf is an init of one track of timescale ticks a second, as FFmpeg writes one: ftyp and moov.
func initOf(timescale uint32) []byte {
	mdhd := make([]byte, 24) // version 0: flags, made, changed, timescale, duration, language
	binary.BigEndian.PutUint32(mdhd[12:], timescale)
	return append(boxOf("ftyp", []byte("isom\x00\x00\x02\x00")),
		boxOf("moov", boxOf("trak", boxOf("mdia", boxOf("mdhd", mdhd))))...)
}

// fragmentOf is one moof and its mdat: samples decoded from decodeTime, the first shown
// firstShownAfter ticks after it is decoded.
func fragmentOf(decodeTime uint64, samples uint32, firstShownAfter int32) []byte {
	tfdt := append([]byte{1, 0, 0, 0}, binary.BigEndian.AppendUint64(nil, decodeTime)...)
	trun := []byte{1, 0, 0x08, 0x01} // version 1: a data offset, and each sample's composition offset
	trun = binary.BigEndian.AppendUint32(trun, samples)
	trun = binary.BigEndian.AppendUint32(trun, 0)
	for index := range samples {
		offset := int32(0)
		if index == 0 {
			offset = firstShownAfter
		}
		trun = binary.BigEndian.AppendUint32(trun, uint32(offset))
	}
	return append(boxOf("moof", boxOf("traf", boxOf("tfdt", tfdt), boxOf("trun", trun))), boxOf("mdat", []byte("x"))...)
}

// rawOf is one raw file of FFmpeg's segment muxer: its init, its fragments and an mfra.
func rawOf(timescale uint32, fragments ...[]byte) []byte {
	raw := initOf(timescale)
	for _, each := range fragments {
		raw = append(raw, each...)
	}
	return append(raw, boxOf("mfra")...)
}

// decodeTimesOf are the fragments' decode times, in order.
func decodeTimesOf(t *testing.T, fragments []byte) []uint64 {
	t.Helper()
	boxes, err := decodeTimes(fragments)
	if err != nil {
		t.Fatal(err)
	}
	times := make([]uint64, 0, len(boxes))
	for _, each := range boxes {
		times = append(times, binary.BigEndian.Uint64(fragments[each.payload+4:]))
	}
	return times
}

// A raw file splits into its init and its fragments, its mfra dropped; the fragments' times move
// together to where the chunk is in the film; the first sample's composition offset and the
// samples are read as written (slow-links.md §10).
func TestFragments_SplitRetimeAndSayTheirOffsetAndSamples(t *testing.T) {
	t.Parallel()
	raw := rawOf(24000, fragmentOf(0, 3, 2002), fragmentOf(3003, 2, 0))
	init, fragments, err := Split(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(init) != string(initOf(24000)) {
		t.Error("the init is not the raw file's ftyp and moov")
	}
	if string(fragments) != string(append(fragmentOf(0, 3, 2002), fragmentOf(3003, 2, 0)...)) {
		t.Error("the fragments are not the raw file's moofs and mdats alone")
	}
	scale, scaleErr := Timescale(init)
	shown, shownErr := FirstShownAfterDecode(fragments)
	samples, samplesErr := Samples(fragments)
	if scale != 24000 || shown != 2002 || samples != 5 || scaleErr != nil || shownErr != nil || samplesErr != nil {
		t.Errorf("the timescale is %d, the first sample shown %d ticks after its decode, %d samples (%v, %v, %v);"+
			" want 24000, 2002, 5", scale, shown, samples, scaleErr, shownErr, samplesErr)
	}
	retimed, err := Retime(fragments, 100_000)
	if err != nil {
		t.Fatal(err)
	}
	if times := decodeTimesOf(t, retimed); len(times) != 2 || times[0] != 100_000 || times[1] != 103_003 {
		t.Errorf("retimed to 100000, the decode times are %v; want 100000 and 103003", times)
	}
	if times := decodeTimesOf(t, fragments); times[0] != 0 {
		t.Error("retiming changed the fragments it was given")
	}
}

// A negative composition offset, which version 1 allows, is read as negative.
func TestFirstShownAfterDecode_ReadsANegativeOffset(t *testing.T) {
	t.Parallel()
	if shown, err := FirstShownAfterDecode(fragmentOf(0, 1, -1001)); err != nil || shown != -1001 {
		t.Errorf("the offset is %d, %v; want -1001", shown, err)
	}
}

// What is not a raw file of the muxer's is refused, never read past its end.
func TestFragments_RefuseWhatIsNotARawFile(t *testing.T) {
	t.Parallel()
	whole := rawOf(48000, fragmentOf(0, 2, 0))
	for name, raw := range map[string][]byte{
		"cut short":          whole[:len(whole)-3],
		"no fragments":       initOf(48000),
		"no init":            fragmentOf(0, 2, 0),
		"a box past its end": append(binary.BigEndian.AppendUint32(nil, 1000), "moof"...),
	} {
		if _, _, err := Split(raw); !errors.Is(err, errMalformed) {
			t.Errorf("%s: split %v, want it refused as malformed", name, err)
		}
	}
	version0 := boxOf("moof", boxOf("traf", boxOf("tfdt", make([]byte, 8))))
	if _, err := Retime(version0, 5); !errors.Is(err, errMalformed) {
		t.Errorf("a version 0 decode time was retimed: %v; want it refused", err)
	}
}
