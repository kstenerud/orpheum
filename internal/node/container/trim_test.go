// SPDX-License-Identifier: AGPL-3.0-or-later

package container_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/container"
	"github.com/kstenerud/orpheum/internal/node/container/mp4test"
)

// picture is a track of 20,000 pictures at 24 a second, a sync sample every 48, its sizes varying,
// stored seven to a chunk.
func picture() mp4test.MP4Track {
	track := mp4test.MP4Track{Description: "avc1", Timescale: 24_000, SamplesPerChunk: 7}
	for n := range 20_000 {
		track.Durations = append(track.Durations, 1000)
		track.Sizes = append(track.Sizes, uint32(10+n%7))
		if n%48 == 0 {
			track.Syncs = append(track.Syncs, uint32(n+1))
		}
	}
	return track
}

// withOffsets is the track with B-frames' composition offsets, negative when isNegative.
func withOffsets(track mp4test.MP4Track, isNegative bool) mp4test.MP4Track {
	cycle := []int32{2000, 5000, 0, 1000}
	if isNegative {
		cycle = []int32{0, 3000, -2000, -1000}
	}
	for n := range track.Durations {
		track.Offsets = append(track.Offsets, cycle[n%len(cycle)])
	}
	return track
}

// sound is a track of 40,000 frames of 1024 samples at 48 kHz, all of six bytes, 43 to a chunk.
func sound() mp4test.MP4Track {
	track := mp4test.MP4Track{Description: "mp4a", Timescale: 48_000, SamplesPerChunk: 43}
	for range 40_000 {
		track.Durations = append(track.Durations, 1024)
		track.Sizes = append(track.Sizes, 6)
	}
	return track
}

// sample is one sample as a whole index has it.
type sample struct {
	decodeAt, offsetTicks, size, at int64
	isSync                          bool
}

// trackOf is one track as a whole index has it: its timescale, its edits' bytes and its samples,
// whether their sizes are listed one by one, and their duration and bytes added up.
type trackOf struct {
	timescale            int64
	edits                []byte
	samples              []sample
	isSized              bool
	durationTicks, bytes int64
}

// children are the boxes in data.
func children(data []byte) map[string][][]byte {
	found := map[string][][]byte{}
	for len(data) >= 8 {
		size := int(binary.BigEndian.Uint32(data))
		if size < 8 || size > len(data) {
			break
		}
		kind := string(data[4:8])
		found[kind] = append(found[kind], data[8:size])
		data = data[size:]
	}
	return found
}

// tracksOf decodes every sample of the file's movie box: the reference a cut-down index is held to.
func tracksOf(t *testing.T, file []byte) []trackOf {
	t.Helper()
	moov := children(file)["moov"]
	if len(moov) != 1 {
		t.Fatalf("%d movie boxes", len(moov))
	}
	var tracks []trackOf
	for _, trak := range children(moov[0])["trak"] {
		mdia := children(children(trak)["mdia"][0])
		stbl := children(children(mdia["minf"][0])["stbl"][0])
		found := trackOf{timescale: int64(binary.BigEndian.Uint32(mdia["mdhd"][0][12:]))}
		if edts := children(trak)["edts"]; edts != nil {
			found.edits = edts[0]
		}
		sizes := stbl["stsz"][0]
		count := int(binary.BigEndian.Uint32(sizes[8:]))
		found.samples = make([]sample, count)
		found.isSized = binary.BigEndian.Uint32(sizes[4:]) == 0
		for n := range found.samples {
			found.samples[n].size = int64(binary.BigEndian.Uint32(sizes[4:]))
			if found.isSized {
				found.samples[n].size = int64(binary.BigEndian.Uint32(sizes[12+4*n:]))
			}
			found.bytes += found.samples[n].size
		}
		timed := 0
		eachRun(stbl["stts"][0], func(value uint32) {
			found.samples[timed].decodeAt = found.durationTicks
			found.durationTicks += int64(value)
			timed++
		})
		if ctts := stbl["ctts"]; ctts != nil {
			offset := 0
			eachRun(ctts[0], func(value uint32) {
				found.samples[offset].offsetTicks = int64(int32(value)) //nolint:gosec // signed
				offset++
			})
		}
		if stss := stbl["stss"]; stss != nil {
			for entry := range int(binary.BigEndian.Uint32(stss[0][4:])) {
				found.samples[binary.BigEndian.Uint32(stss[0][8+4*entry:])-1].isSync = true
			}
		} else {
			for n := range found.samples {
				found.samples[n].isSync = true
			}
		}
		placeSamples(stbl, found.samples)
		tracks = append(tracks, found)
	}
	return tracks
}

// eachRun calls visit with each sample's value of an stts or ctts.
func eachRun(table []byte, visit func(value uint32)) {
	for entry := range int(binary.BigEndian.Uint32(table[4:])) {
		count, value := binary.BigEndian.Uint32(table[8+8*entry:]), binary.BigEndian.Uint32(table[12+8*entry:])
		for range count {
			visit(value)
		}
	}
}

// placeSamples sets each sample's offset in the file from the sample table's chunks.
func placeSamples(stbl map[string][][]byte, samples []sample) {
	var offsets []int64
	if co64 := stbl["co64"]; co64 != nil {
		for entry := range int(binary.BigEndian.Uint32(co64[0][4:])) {
			offsets = append(offsets, int64(binary.BigEndian.Uint64(co64[0][8+8*entry:]))) //nolint:gosec // small
		}
	} else {
		for entry := range int(binary.BigEndian.Uint32(stbl["stco"][0][4:])) {
			offsets = append(offsets, int64(binary.BigEndian.Uint32(stbl["stco"][0][8+4*entry:])))
		}
	}
	stsc := stbl["stsc"][0]
	entries := int(binary.BigEndian.Uint32(stsc[4:]))
	placed := 0
	for entry := range entries {
		first := int(binary.BigEndian.Uint32(stsc[8+12*entry:]))
		perChunk := int(binary.BigEndian.Uint32(stsc[12+12*entry:]))
		last := len(offsets) + 1
		if entry+1 < entries {
			last = int(binary.BigEndian.Uint32(stsc[8+12*(entry+1):]))
		}
		for chunk := first; chunk < last; chunk++ {
			at := offsets[chunk-1]
			for range perChunk {
				samples[placed].at, at, placed = at, at+samples[placed].size, placed+1
			}
		}
	}
}

// cutDown is the file with its index cut down to fromMs to toMs, as a run would be served it.
func cutDown(t *testing.T, file []byte, fromMs, toMs int64) ([]byte, error) {
	t.Helper()
	moovAt, moov, err := container.FindMoov(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	index, err := container.ReadIndex(bytes.NewReader(file), int64(len(file)), moovAt, moov)
	if err != nil {
		return nil, err
	}
	trimmed, err := index.Trim(bytes.NewReader(file), fromMs, toMs)
	if err != nil {
		return nil, err
	}
	served := slices.Clone(file)
	if before, after := trimmed.ReadAt(served, 0); before != int(moovAt) || after != len(file)-int(moov.End) {
		t.Fatalf("the cut-down index is %d bytes in and %d from the end; want its movie box's %d and %d",
			before, after, moovAt, len(file)-int(moov.End))
	}
	if !bytes.Equal(served[:moovAt], file[:moovAt]) || !bytes.Equal(served[moov.End:], file[moov.End:]) {
		t.Fatal("the cut-down file differs outside its movie box")
	}
	return served, nil
}

// checkCutDown holds the file cut down to fromMs to toMs to its whole index: every track's first
// sample and every sample of the stretch, a picture's from its sync sample at or before the
// stretch, kept, and every sample kept the file's, at its decode time, with its composition offset,
// size, offset and sync; the edits as they were; the movie box's place filled by it and a free box.
// A track whose sizes are listed one by one ends with a tail at the file's end, which no run reads,
// holding the bytes and lasting the time of the samples cut away, so its bit rate is the file's.
func checkCutDown(t *testing.T, file []byte, fromMs, toMs int64) {
	t.Helper()
	served, err := cutDown(t, file, fromMs, toMs)
	if err != nil {
		t.Fatal(err)
	}
	top := children(served[len(served)-len(children(file)["moov"][0])-8:])
	if len(top["moov"]) != 1 || len(top["free"]) != 1 {
		t.Fatalf("the movie box's place holds %d movie boxes and %d free; want one of each",
			len(top["moov"]), len(top["free"]))
	}
	whole, cut := tracksOf(t, file), tracksOf(t, served)
	if len(cut) != len(whole) {
		t.Fatalf("the cut-down index has %d tracks, want %d", len(cut), len(whole))
	}
	filmStartMs := int64(1 << 62)
	for _, track := range whole {
		_, emptyMs := editOf(track)
		filmStartMs = min(filmStartMs, emptyMs)
	}
	for number := range whole {
		checkTrackCutDown(t, number, whole[number], cut[number])
		start, emptyMs := editOf(whole[number])
		stretch := [2]int64{fromMs + filmStartMs - emptyMs, toMs + filmStartMs - emptyMs}
		checkStretchKept(t, number, whole[number], keptOf(t, number, whole[number], cut[number], len(file)),
			start, stretch)
	}
}

// checkTrackCutDown holds a track cut down to the file's: its timescale and edits, fewer samples
// from its first, and, when its sizes are listed one by one, its bytes and time.
func checkTrackCutDown(t *testing.T, number int, whole, cut trackOf) {
	t.Helper()
	if cut.timescale != whole.timescale || !bytes.Equal(cut.edits, whole.edits) {
		t.Fatalf("track %d: its timescale or edits changed", number)
	}
	if len(cut.samples) >= len(whole.samples) || cut.samples[0] != whole.samples[0] {
		t.Fatalf("track %d: %d of %d samples kept, the first %+v; want fewer, from %+v", number,
			len(cut.samples), len(whole.samples), cut.samples[0], whole.samples[0])
	}
	if whole.isSized && (cut.bytes != whole.bytes || cut.durationTicks != whole.durationTicks) {
		t.Fatalf("track %d: the cut-down index holds %d bytes over %d ticks; want the file's %d over %d",
			number, cut.bytes, cut.durationTicks, whole.bytes, whole.durationTicks)
	}
}

// keptOf are the decode times of the cut track's samples, each held to the file's sample at its
// time; the tail, past the file's end, is not.
func keptOf(t *testing.T, number int, whole, cut trackOf, fileBytes int) map[int64]bool {
	t.Helper()
	byDecode := map[int64]sample{}
	for _, kept := range whole.samples {
		byDecode[kept.decodeAt] = kept
	}
	kept := map[int64]bool{}
	for _, got := range cut.samples {
		if got.at >= int64(fileBytes) && whole.isSized {
			continue // the tail
		}
		if want, found := byDecode[got.decodeAt]; !found || got != want {
			t.Fatalf("track %d: the cut-down index has %+v, the file %+v (%v)", number, got, want, found)
		}
		kept[got.decodeAt] = true
	}
	return kept
}

// checkStretchKept holds that every sample of the track in the stretch, in ms of the track's own
// time from its edit's start, is kept, and the sync sample at or before it.
func checkStretchKept(t *testing.T, number int, whole trackOf, kept map[int64]bool, start int64, stretch [2]int64) {
	t.Helper()
	var lastSyncBefore int64 = -1
	for _, want := range whole.samples {
		ms := (want.decodeAt - start) * 1000 / whole.timescale
		if want.isSync && ms <= stretch[0] {
			lastSyncBefore = want.decodeAt
		}
		if ms >= stretch[0] && ms <= stretch[1] && !kept[want.decodeAt] {
			t.Fatalf("track %d: the sample at %d ms, in the stretch, is not kept", number, ms)
		}
	}
	if lastSyncBefore >= 0 && !kept[lastSyncBefore] {
		t.Fatalf("track %d: the sync sample before the stretch is not kept", number)
	}
}

// editOf is where the track's edit starts its media, or zero, and the empty edit before it, in ms
// of the tests' movie timescale of 1000.
func editOf(track trackOf) (startTicks, emptyMs int64) {
	if track.edits == nil {
		return 0, 0
	}
	list := children(track.edits)["elst"][0]
	count := int(binary.BigEndian.Uint32(list[4:]))
	if count == 2 {
		emptyMs = int64(binary.BigEndian.Uint32(list[8:]))
	}
	return int64(int32(binary.BigEndian.Uint32(list[8+12*(count-1)+4:]))), emptyMs //nolint:gosec // signed
}

// A cut-down index keeps every sample of its stretch, at the start, the middle and the end of the
// film, and each sample it keeps is the file's, at its time, with its offset and size, its picture's
// from the sync sample before the stretch, whether the file has composition offsets or none,
// negative ones, 32-bit offsets or 64-bit ones, an edit list or none (D512).
func TestTrim_KeepsEverySampleOfItsStretchAsTheFileHasIt(t *testing.T) {
	t.Parallel()
	edited := func() mp4test.MP4 {
		shown, heard := withOffsets(picture(), false), sound()
		shown.Edits = [][3]int64{{800_000, 2000, 1 << 16}}
		heard.Edits = [][3]int64{{500, -1, 1 << 16}, {800_000, 0, 1 << 16}}
		return mp4test.MP4{Timescale: 1000, Tracks: []mp4test.MP4Track{shown, heard}}
	}
	wide := mp4test.MP4{Timescale: 1000, Tracks: []mp4test.MP4Track{withOffsets(picture(), false), sound()}}
	for number := range wide.Tracks {
		wide.Tracks[number].IsWide = true
	}
	for name, movie := range map[string]mp4test.MP4{
		"offsets":  {Timescale: 1000, Tracks: []mp4test.MP4Track{withOffsets(picture(), false), sound()}},
		"none":     {Timescale: 1000, Tracks: []mp4test.MP4Track{picture(), sound()}},
		"negative": {Timescale: 1000, Tracks: []mp4test.MP4Track{withOffsets(picture(), true), sound()}},
		"wide":     wide,
		"edited":   edited(),
	} {
		file := mp4test.MP4File(movie)
		for _, stretch := range [][2]int64{{0, 20_000}, {400_123, 430_000}, {790_000, 900_000}} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				checkCutDown(t, file, stretch[0], stretch[1])
			})
		}
	}
}

// An MP4 whose index a cut-down one might not serve as it is, is not cut down, and says why
// (ErrCannotTrim, D512): a fragmented file, a track of two sample descriptions or an encrypted one,
// an edit list of more than one edit of media or of another rate, a sample table holding a box it
// does not slice, tables that disagree on the samples, a film with no track long enough to slice,
// and a picture whose least composition offset, by which FFmpeg moves its times, lies outside the
// stretch.
func TestReadIndex_RefusesAnIndexItDoesNotFollow(t *testing.T) {
	t.Parallel()
	with := func(change func(track *mp4test.MP4Track)) mp4test.MP4 {
		shown := picture()
		change(&shown)
		return mp4test.MP4{Timescale: 1000, Tracks: []mp4test.MP4Track{shown, sound()}}
	}
	fragmented := with(func(*mp4test.MP4Track) {})
	fragmented.IsFragmented = true
	short := picture()
	short.Durations, short.Sizes, short.Syncs = short.Durations[:100], short.Sizes[:100], short.Syncs[:3]
	for name, movie := range map[string]mp4test.MP4{
		"fragmented":   fragmented,
		"descriptions": with(func(track *mp4test.MP4Track) { track.Descriptions = 2 }),
		"encrypted":    with(func(track *mp4test.MP4Track) { track.Description = "encv" }),
		"two edits": with(func(track *mp4test.MP4Track) {
			track.Edits = [][3]int64{{1000, 0, 1 << 16}, {1000, 5000, 1 << 16}}
		}),
		"fast edit": with(func(track *mp4test.MP4Track) { track.Edits = [][3]int64{{1000, 0, 2 << 16}} }),
		"subsamples": with(func(track *mp4test.MP4Track) {
			track.Extra = [][]byte{mp4test.FullBox("subs", 0, make([]byte, 4))}
		}),
		"disagree": with(func(track *mp4test.MP4Track) { track.Durations = track.Durations[:19_000] }),
		"short":    {Timescale: 1000, Tracks: []mp4test.MP4Track{short}},
		"shift": with(func(track *mp4test.MP4Track) {
			*track = withOffsets(*track, false)
			track.Offsets[15_000] = -7000
		}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := cutDown(t, mp4test.MP4File(movie), 400_000, 430_000)
			if !errors.Is(err, container.ErrCannotTrim) {
				t.Fatalf("cut down with %v; want ErrCannotTrim", err)
			}
		})
	}
}
