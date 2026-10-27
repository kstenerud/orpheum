// SPDX-License-Identifier: AGPL-3.0-or-later

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

// aacFrameTicks is how long an AAC frame lasts at its sample rate: 1024 samples.
const aacFrameTicks = 1024

// lastingFragmentOf is one moof and its mdat of samples each lasting an AAC frame, as FFmpeg writes a
// sound's: the duration the fragment's default (tfhd), the trun naming each sample's size alone.
func lastingFragmentOf(samples uint32) []byte {
	const ticks = aacFrameTicks
	tfhd := []byte{0, 0x02, 0, 0x08} // the base is the moof, and a default duration
	tfhd = binary.BigEndian.AppendUint32(tfhd, 1)
	tfhd = binary.BigEndian.AppendUint32(tfhd, ticks)
	tfdt := append([]byte{1, 0, 0, 0}, binary.BigEndian.AppendUint64(nil, 0)...)
	trun := []byte{0, 0, 0x02, 0x01} // a data offset, and each sample's size
	trun = binary.BigEndian.AppendUint32(trun, samples)
	trun = binary.BigEndian.AppendUint32(trun, 0)
	for range samples {
		trun = binary.BigEndian.AppendUint32(trun, 400)
	}
	traf := boxOf("traf", boxOf("tfhd", tfhd), boxOf("tfdt", tfdt), boxOf("trun", trun))
	return append(boxOf("moof", traf), boxOf("mdat", []byte("x"))...)
}

// timedFragmentOf is one moof and its mdat whose trun names each sample's duration, as FFmpeg writes
// one whose samples last unequally: a copied sound's first, moved a tick before the film.
func timedFragmentOf(ticks ...uint32) []byte {
	if len(ticks) > 1<<16 {
		panic("a fragment of more samples than a test makes")
	}
	tfdt := append([]byte{1, 0, 0, 0}, binary.BigEndian.AppendUint64(nil, 0)...)
	trun := []byte{0, 0, 0x03, 0x01}                               // a data offset, and each sample's duration and size
	trun = binary.BigEndian.AppendUint32(trun, uint32(len(ticks))) //nolint:gosec // bounded above
	trun = binary.BigEndian.AppendUint32(trun, 0)
	for _, each := range ticks {
		trun = binary.BigEndian.AppendUint32(trun, each)
		trun = binary.BigEndian.AppendUint32(trun, 400)
	}
	return append(boxOf("moof", boxOf("traf", boxOf("tfdt", tfdt), boxOf("trun", trun))), boxOf("mdat", []byte("x"))...)
}

// initLastingOf is initOf whose track's samples last ticks by default (trex).
func initLastingOf(timescale, ticks uint32) []byte {
	trex := make([]byte, 24) // version and flags, track, description, duration, size, flags
	binary.BigEndian.PutUint32(trex[4:], 1)
	binary.BigEndian.PutUint32(trex[12:], ticks)
	mdhd := make([]byte, 24)
	binary.BigEndian.PutUint32(mdhd[12:], timescale)
	return append(boxOf("ftyp", []byte("isom\x00\x00\x02\x00")),
		boxOf("moov", boxOf("trak", boxOf("mdia", boxOf("mdhd", mdhd))), boxOf("mvex", boxOf("trex", trex)))...)
}

// Fragments last as long as their samples: each sample's own duration where the trun names it, else
// the fragment's default, else the track's.
func TestDuration_IsEachSamplesOwnOrTheFragmentsOrTheTracksDefault(t *testing.T) {
	t.Parallel()
	for name, each := range map[string]struct {
		init, fragments []byte
		want            int64
	}{
		"each its own":         {initOf(48000), timedFragmentOf(1, 1024, 1024), 2049},
		"the fragment's":       {initOf(48000), lastingFragmentOf(94), 94 * 1024},
		"the track's":          {initLastingOf(48000, 960), fragmentOf(0, 50, 0), 50 * 960},
		"two fragments summed": {initOf(48000), append(lastingFragmentOf(2), timedFragmentOf(5, 6)...), 2059},
	} {
		if got, err := Duration(each.init, each.fragments); err != nil || got != each.want {
			t.Errorf("%s: the fragments last %d, %v; want %d", name, got, err, each.want)
		}
	}
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
