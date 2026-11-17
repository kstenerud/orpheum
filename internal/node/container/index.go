// SPDX-License-Identifier: AGPL-3.0-or-later

package container

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
)

// ErrCannotTrim is an MP4 whose index this package does not cut down to a stretch of the film (D512):
// a fragmented file, a track it cannot slice, an edit list it does not follow. Its runs read the
// file as it is; the error says why.
var ErrCannotTrim = errors.New("the MP4's index cannot be cut down")

// checkpointEvery is how many entries of a long table lie between two of its checkpoints, so a
// stretch is found by reading at most this many entries past the nearest one.
const checkpointEvery = 1024

// maxKeptBytes bounds the bytes of the movie box copied whole into every cut-down index: its
// headers, descriptions, edit lists and the tables of tracks too short to slice.
const maxKeptBytes = 16 << 20

// maxIndexTracks bounds the tracks an index is cut down for.
const maxIndexTracks = 64

// minSlicedSamples is the fewest samples a track must have to be sliced; a shorter one, as a
// chapter or timecode track is, is copied whole.
const minSlicedSamples = 4096

// tableBlockBytes is how much of a table is read at once while it is scanned or sliced.
const tableBlockBytes = 1 << 20

// maxSliceEntries bounds the entries one slice of a table reads.
const maxSliceEntries = 1 << 22

// The kinds of box named more than once.
const (
	moovKind = "moov"
	co64Kind = "co64"
	sdtpKind = "sdtp"
	sbgpKind = "sbgp"
)

// Index is an MP4's movie box laid out for cutting down (D512): its boxes, those it copies whole read
// and each sliced track's tables placed, with checkpoints every checkpointEvery entries of the long
// ones, so a stretch of any track is found and read without reading its tables whole.
type Index struct {
	// MoovAt and MoovBytes are where the movie box starts, its header included, and its whole size.
	MoovAt, MoovBytes int64
	sizeBytes         int64
	root              *boxNode
	tracks            []*track
	movieTimescale    int64
	// filmStartMs is where FFmpeg starts the film, as far as a stretch must know: the least of the
	// tracks' empty edits.
	filmStartMs int64
}

// boxNode is one box of the movie box's tree as it is rebuilt: a container, a box copied whole, or a
// sliced track's table.
type boxNode struct {
	kind     string
	whole    []byte // a box copied whole, its header included
	table    *Box   // a sliced track's table, read in slices
	children []*boxNode
}

// containerDepth is how deep the boxes descended into nest: moov, trak, mdia, minf, stbl.
const containerDepth = 5

// isContainerKind is a box descended into, a container of boxes and nothing else.
func isContainerKind(kind string) bool {
	switch kind {
	case moovKind, "trak", "mdia", "minf", "stbl":
		return true
	}
	return false
}

// isSlicedKind is one of the tables a sliced track's sample table is made of: the only boxes it may
// hold.
func isSlicedKind(kind string) bool {
	switch kind {
	case "stsd", "stts", "ctts", "stss", "stsc", "stsz", "stco", co64Kind, sdtpKind, sbgpKind, "sgpd":
		return true
	}
	return false
}

// FindMoov is an ISO BMFF file's movie box: where its header starts, and the box. A file whose
// top-level boxes cannot be followed is ErrNotThisContainer, and one without a movie box ErrNoIndex.
func FindMoov(file io.ReaderAt, sizeBytes int64) (int64, Box, error) {
	position := int64(0)
	for range MaxElementHops {
		if position >= sizeBytes {
			break
		}
		found, err := ReadBox(file, sizeBytes, position)
		if err != nil {
			return 0, Box{}, err
		}
		if found.Kind == moovKind {
			return position, found, nil
		}
		position = found.End
	}
	return 0, Box{}, fmt.Errorf("%w: no moov box", ErrNoIndex)
}

// ReadIndex lays out the movie box at moovAt for cutting down: it reads the boxes copied whole, and
// scans each long table once for its checkpoints. What it does not follow is ErrCannotTrim, saying
// why, and a failed read a ReadError.
func ReadIndex(file io.ReaderAt, sizeBytes, moovAt int64, moov Box) (*Index, error) {
	index := &Index{MoovAt: moovAt, MoovBytes: moov.End - moovAt, sizeBytes: sizeBytes}
	if index.MoovBytes > 1<<32-1 {
		return nil, fmt.Errorf("%w: a moov of %d bytes", ErrCannotTrim, index.MoovBytes)
	}
	budget := int64(maxKeptBytes)
	root, err := index.readTree(file, moov, &budget, 0)
	if err != nil {
		return nil, err
	}
	index.root = root
	if index.movieTimescale, err = movieTimescale(root); err != nil {
		return nil, err
	}
	for _, trak := range root.children {
		if trak.kind != "trak" {
			continue
		}
		if len(index.tracks) == maxIndexTracks {
			return nil, fmt.Errorf("%w: more than %d tracks", ErrCannotTrim, maxIndexTracks)
		}
		found, trackErr := index.readTrack(file, trak, &budget)
		if trackErr != nil {
			return nil, trackErr
		}
		if found != nil {
			index.tracks = append(index.tracks, found)
		}
	}
	if len(index.tracks) == 0 {
		return nil, fmt.Errorf("%w: no track long enough to slice", ErrCannotTrim)
	}
	index.filmStartMs = index.tracks[0].emptyMs
	for _, found := range index.tracks {
		index.filmStartMs = min(index.filmStartMs, found.emptyMs)
	}
	return index, nil
}

// readTree reads box's children: containers descended into, a sample table's tables placed, every
// other box read whole within budget.
func (index *Index) readTree(file io.ReaderAt, box Box, budget *int64, depth int) (*boxNode, error) {
	node := &boxNode{kind: box.Kind}
	for position := box.DataStart; position < box.End; {
		found, err := ReadBox(file, box.End, position)
		if err != nil {
			return nil, cannotTrim(err)
		}
		switch {
		case found.Kind == "mvex" || found.Kind == "moof":
			return nil, fmt.Errorf("%w: a fragmented file", ErrCannotTrim)
		case isContainerKind(found.Kind) && depth < containerDepth:
			inner, innerErr := index.readTree(file, found, budget, depth+1)
			if innerErr != nil {
				return nil, innerErr
			}
			node.children = append(node.children, inner)
		case box.Kind == "stbl" && isLongTable(found.Kind):
			placed := found
			node.children = append(node.children, &boxNode{kind: found.Kind, table: &placed})
		default:
			whole, readErr := readWhole(file, position, found.End, budget)
			if readErr != nil {
				return nil, readErr
			}
			node.children = append(node.children, &boxNode{kind: found.Kind, whole: whole})
		}
		position = found.End
	}
	return node, nil
}

// isLongTable is a sample table's table that may be long: placed, not read whole.
func isLongTable(kind string) bool {
	return kind != "stsd" && kind != "sgpd" && isSlicedKind(kind)
}

// readWhole reads the box from start to end, within budget.
func readWhole(file io.ReaderAt, start, end int64, budget *int64) ([]byte, error) {
	if end-start > *budget {
		return nil, fmt.Errorf("%w: more than %d bytes of the moov to copy whole", ErrCannotTrim, maxKeptBytes)
	}
	*budget -= end - start
	whole := make([]byte, end-start)
	if err := ReadAt(file, end, start, whole); err != nil {
		return nil, cannotTrim(err)
	}
	return whole, nil
}

// cannotTrim is err, a structure not followed being a reason not to cut the index down.
func cannotTrim(err error) error {
	if errors.Is(err, ErrNotThisContainer) {
		return fmt.Errorf("%w: %w", ErrCannotTrim, err)
	}
	return err
}

// childNode is node's first child of kind.
func (node *boxNode) childNode(kind string) *boxNode {
	for _, found := range node.children {
		if found.kind == kind {
			return found
		}
	}
	return nil
}

// data is a box copied whole without its header.
func (node *boxNode) data() []byte {
	if node == nil || len(node.whole) < 8 {
		return nil
	}
	if binary.BigEndian.Uint32(node.whole) == 1 {
		return node.whole[min(16, len(node.whole)):]
	}
	return node.whole[8:]
}

// movieTimescale is the movie header's timescale, which an edit list's durations count in.
func movieTimescale(root *boxNode) (int64, error) {
	return timescaleOf(root.childNode("mvhd").data(), "movie")
}

// timescaleOf is the timescale a movie or media header holds, after its times of 32 or 64 bits.
func timescaleOf(header []byte, what string) (int64, error) {
	if len(header) < 24 {
		return 0, fmt.Errorf("%w: no %s header", ErrCannotTrim, what)
	}
	timescaleAt := 12
	if header[0] == 1 {
		timescaleAt = 20
	}
	if len(header) < timescaleAt+4 || binary.BigEndian.Uint32(header[timescaleAt:]) == 0 {
		return 0, fmt.Errorf("%w: no %s timescale", ErrCannotTrim, what)
	}
	return int64(binary.BigEndian.Uint32(header[timescaleAt:])), nil
}

// table is one of a track's long tables: its entries' place and size, and, every checkpointEvery
// entries, a checkpoint of what precedes that entry.
type table struct {
	node       *boxNode
	kind       string
	header     []byte // its version and flags, and for stsz its sample size, as the file has them
	entriesAt  int64
	entries    int64
	entryBytes int64
	marks      []mark
}

// mark is a table's checkpoint at entry n*checkpointEvery: the samples before it and their
// duration, or, for stss, the sample number it holds.
type mark struct{ samples, time int64 }

// track is a sliced track: its timescale and edit, its count of samples and its tables.
type track struct {
	stbl      *boxNode
	fileSize  int64
	timescale int64
	// editStart is the media time its edit shows first, and emptyMs the empty edit before it.
	editStart, emptyMs int64
	samples            int64
	chunks             int64
	stts, ctts, stsc   *table
	stss, stsz, offset *table
	sdtp               *table
	groups             []*table
	// durationTicks and dataBytes are the track's whole duration, by its stts, and its samples'
	// sizes added up, zero for samples of one size: FFmpeg says its bit rate from them (Trim's
	// tail). shiftTicks is how far FFmpeg moves its times by its least composition offset.
	durationTicks, dataBytes, shiftTicks int64
}

// readTrack places a trak's tables and scans the long ones, when it is long enough to slice; a short
// one's tables are read whole, and it is nil.
func (index *Index) readTrack(file io.ReaderAt, trak *boxNode, budget *int64) (*track, error) {
	stbl := trak.childNode("mdia").childNodeOrNil("minf").childNodeOrNil("stbl")
	if stbl == nil {
		return nil, fmt.Errorf("%w: a track without a sample table", ErrCannotTrim)
	}
	stsz := stbl.childNode("stsz")
	if stsz == nil || stsz.table == nil {
		return nil, fmt.Errorf("%w: a track without sample sizes (stsz)", ErrCannotTrim)
	}
	var head [12]byte
	if err := ReadAt(file, stsz.table.End, stsz.table.DataStart, head[:]); err != nil {
		return nil, cannotTrim(err)
	}
	if samples := int64(binary.BigEndian.Uint32(head[8:])); samples < minSlicedSamples {
		for _, node := range stbl.children {
			if node.table != nil {
				whole, err := readWhole(file, node.table.DataStart-8, node.table.End, budget)
				if err != nil {
					return nil, err
				}
				node.whole, node.table = whole, nil
			}
		}
		return nil, nil //nolint:nilnil // a track copied whole is no track to slice
	}
	return index.placeTrack(file, trak, stbl)
}

// childNodeOrNil is childNode on a node that may be nil.
func (node *boxNode) childNodeOrNil(kind string) *boxNode {
	if node == nil {
		return nil
	}
	return node.childNode(kind)
}

// placeTrack checks a long track is one this package slices, and places and scans its tables.
func (index *Index) placeTrack(file io.ReaderAt, trak, stbl *boxNode) (*track, error) {
	found := &track{stbl: stbl, fileSize: index.sizeBytes}
	for _, node := range stbl.children {
		if !isSlicedKind(node.kind) {
			return nil, fmt.Errorf("%w: a sample table holding %q", ErrCannotTrim, node.kind)
		}
	}
	if err := checkDescription(stbl.childNode("stsd").data()); err != nil {
		return nil, err
	}
	var err error
	if found.timescale, err = mediaTimescaleOf(trak); err != nil {
		return nil, err
	}
	if found.editStart, found.emptyMs, err = editOf(trak.childNode("edts"), index.movieTimescale); err != nil {
		return nil, err
	}
	if err := found.placeTables(file, stbl); err != nil {
		return nil, err
	}
	if err := found.scan(file); err != nil {
		return nil, err
	}
	return found, nil
}

// checkDescription refuses a track of more than one sample description, or an encrypted one.
func checkDescription(stsd []byte) error {
	if len(stsd) < 16 {
		return fmt.Errorf("%w: no sample description", ErrCannotTrim)
	}
	if count := binary.BigEndian.Uint32(stsd[4:]); count != 1 {
		return fmt.Errorf("%w: %d sample descriptions", ErrCannotTrim, count)
	}
	if kind := string(stsd[12:16]); kind[:3] == "enc" || kind == "drms" || kind == "drmi" {
		return fmt.Errorf("%w: an encrypted track (%s)", ErrCannotTrim, kind)
	}
	return nil
}

// mediaTimescaleOf is a trak's media timescale.
func mediaTimescaleOf(trak *boxNode) (int64, error) {
	return timescaleOf(trak.childNode("mdia").childNodeOrNil("mdhd").data(), "media")
}

// editOf is where a trak's edit list starts its media, in the media's timescale, and the empty edit
// before it, in ms: none, one edit, or an empty one and then one, each at the normal rate; any other
// list is ErrCannotTrim.
func editOf(edts *boxNode, movieTimescale int64) (start, emptyMs int64, err error) {
	if edts == nil {
		return 0, 0, nil
	}
	list, found := child(edts.data(), "elst")
	if !found {
		return 0, 0, nil
	}
	if len(list) < 8 {
		return 0, 0, fmt.Errorf("%w: a short edit list", ErrCannotTrim)
	}
	entryBytes := 12
	if list[0] == 1 {
		entryBytes = 20
	}
	count := int(binary.BigEndian.Uint32(list[4:]))
	if count > 2 || count > (len(list)-8)/entryBytes {
		return 0, 0, fmt.Errorf("%w: an edit list of %d edits", ErrCannotTrim, count)
	}
	for number := range count {
		duration, mediaTime, rate := editEntry(list[8+number*entryBytes:], entryBytes)
		switch {
		case mediaTime == -1 && number == 0 && count == 2:
			emptyMs = duration * 1000 / movieTimescale
		case mediaTime >= 0 && number == count-1 && rate == 1<<16:
			return mediaTime, emptyMs, nil
		default:
			return 0, 0, fmt.Errorf("%w: an edit list it does not follow", ErrCannotTrim)
		}
	}
	return 0, 0, nil
}

// editEntry is an edit list entry's duration, media time and rate, of a version 1 list when
// entryBytes is 20.
func editEntry(entry []byte, entryBytes int) (duration, mediaTime int64, rate uint32) {
	rate = binary.BigEndian.Uint32(entry[entryBytes-4:])
	if entryBytes == 20 {
		duration = int64(binary.BigEndian.Uint64(entry))      //nolint:gosec // bounded by the file
		mediaTime = int64(binary.BigEndian.Uint64(entry[8:])) //nolint:gosec // a signed time
		return duration, mediaTime, rate
	}
	mediaTime = int64(int32(binary.BigEndian.Uint32(entry[4:]))) //nolint:gosec // a signed time
	return int64(binary.BigEndian.Uint32(entry)), mediaTime, rate
}

// placeTables finds a sliced track's tables in its sample table, and each one's entries.
func (found *track) placeTables(file io.ReaderAt, stbl *boxNode) error {
	place := func(kind string, entryBytes, headerBytes int64) (*table, error) {
		node := stbl.childNode(kind)
		if node == nil {
			return nil, nil //nolint:nilnil // a table the track does not have
		}
		box := node.table
		header := make([]byte, headerBytes+4)
		if err := ReadAt(file, box.End, box.DataStart, header); err != nil {
			return nil, cannotTrim(err)
		}
		placed := &table{
			node: node, kind: kind, header: header[:headerBytes], entriesAt: box.DataStart + headerBytes + 4,
			entries: int64(binary.BigEndian.Uint32(header[headerBytes:])), entryBytes: entryBytes,
		}
		if kind == "stsz" && binary.BigEndian.Uint32(header[4:]) != 0 {
			placed.entryBytes = 0 // every sample of the one size, listed nowhere
			return placed, nil
		}
		if placed.entries > (box.End-placed.entriesAt)/max(entryBytes, 1) {
			return nil, fmt.Errorf("%w: a %s of %d entries in %d bytes", ErrCannotTrim, kind, placed.entries,
				box.End-box.DataStart)
		}
		return placed, nil
	}
	var err error
	if found.stsz, err = place("stsz", 4, 8); err != nil {
		return err
	}
	if found.stsz == nil {
		return fmt.Errorf("%w: a track without sample sizes (stsz)", ErrCannotTrim)
	}
	found.samples = found.stsz.entries
	for _, step := range []struct {
		into  **table
		kind  string
		bytes int64
	}{
		{&found.stts, "stts", 8}, {&found.ctts, "ctts", 8}, {&found.stsc, "stsc", 12}, {&found.stss, "stss", 4},
	} {
		if *step.into, err = place(step.kind, step.bytes, 4); err != nil {
			return err
		}
	}
	if found.offset, err = placeOffsets(place); err != nil {
		return err
	}
	if found.stts == nil || found.stsc == nil {
		return fmt.Errorf("%w: a track without stts or stsc", ErrCannotTrim)
	}
	found.chunks = found.offset.entries
	return found.placeSampleTables(file, stbl)
}

// placeOffsets is a track's one table of chunk offsets, an stco or a co64, placed by place.
func placeOffsets(place func(kind string, entryBytes, headerBytes int64) (*table, error)) (*table, error) {
	narrow, err := place("stco", 4, 4)
	if err != nil {
		return nil, err
	}
	wide, err := place(co64Kind, 8, 4)
	switch {
	case err != nil:
		return nil, err
	case (wide != nil) == (narrow != nil):
		return nil, fmt.Errorf("%w: not one table of chunk offsets", ErrCannotTrim)
	case wide != nil:
		return wide, nil
	}
	return narrow, nil
}

// placeSampleTables places a track's tables of a value per sample (sdtp) or per run of samples
// (sbgp): sdtp's bytes run from after its version and flags, one a sample.
func (found *track) placeSampleTables(file io.ReaderAt, stbl *boxNode) error {
	for _, node := range stbl.children {
		switch node.kind {
		case sdtpKind:
			box := node.table
			header := make([]byte, 4)
			if err := ReadAt(file, box.End, box.DataStart, header); err != nil {
				return cannotTrim(err)
			}
			if box.End-box.DataStart-4 != found.samples {
				return fmt.Errorf("%w: an sdtp not of one byte a sample", ErrCannotTrim)
			}
			found.sdtp = &table{
				node: node, kind: sdtpKind, header: header, entriesAt: box.DataStart + 4, entries: found.samples,
				entryBytes: 1,
			}
		case sbgpKind:
			groups, err := placeGroups(file, node)
			if err != nil {
				return err
			}
			found.groups = append(found.groups, groups)
		}
	}
	return nil
}

// placeGroups places an sbgp: its header, of version and flags, the grouping type and, in version
// 1, its parameter, then its runs of samples.
func placeGroups(file io.ReaderAt, node *boxNode) (*table, error) {
	box := node.table
	header := make([]byte, 16)
	if err := ReadAt(file, box.End, box.DataStart, header[:12]); err != nil {
		return nil, cannotTrim(err)
	}
	headerBytes := int64(8)
	if header[0] == 1 {
		headerBytes = 12
		if err := ReadAt(file, box.End, box.DataStart, header); err != nil {
			return nil, cannotTrim(err)
		}
	}
	entries := int64(binary.BigEndian.Uint32(header[headerBytes:]))
	entriesAt := box.DataStart + headerBytes + 4
	if entries > (box.End-entriesAt)/8 {
		return nil, fmt.Errorf("%w: a short sbgp", ErrCannotTrim)
	}
	return &table{
		node: node, kind: sbgpKind, header: header[:headerBytes], entriesAt: entriesAt, entries: entries, entryBytes: 8,
	}, nil
}

// cmpErr is first when it is not nil, and otherwise fallback.
func cmpErr(first, fallback error) error {
	if first != nil {
		return first
	}
	return fallback
}

// scan reads each long table once, checking it and taking its checkpoints.
func (found *track) scan(file io.ReaderAt) error {
	if err := found.scanRuns(file, found.stts, true); err != nil {
		return err
	}
	if found.ctts != nil {
		if err := found.scanRuns(file, found.ctts, false); err != nil {
			return err
		}
	}
	for _, groups := range found.groups {
		if err := found.scanGroups(file, groups); err != nil {
			return err
		}
	}
	if err := found.scanChunks(file); err != nil {
		return err
	}
	if err := found.scanSizes(file); err != nil {
		return err
	}
	if found.stss != nil {
		return found.scanSyncs(file)
	}
	return nil
}

// scanBlock is how many of a table's entries a scan reads at once: tableBlockBytes of them.
func scanBlock(placed *table) int64 { return max(tableBlockBytes/max(placed.entryBytes, 1), 1) }

// eachEntry visits a table's entries from first, perBlock read at once, until visit says
// to stop or the table ends.
func eachEntry(
	file io.ReaderAt, placed *table, first, perBlock int64, visit func(number int64, entry []byte) bool,
) error {
	for number := first; number < placed.entries; {
		count := min(perBlock, placed.entries-number)
		block := make([]byte, count*placed.entryBytes)
		position := placed.entriesAt + number*placed.entryBytes
		if err := ReadAt(file, position+int64(len(block)), position, block); err != nil {
			return cannotTrim(err)
		}
		for n := range count {
			if !visit(number+n, block[n*placed.entryBytes:(n+1)*placed.entryBytes]) {
				return nil
			}
		}
		number += count
	}
	return nil
}

// scanRuns checks a table of runs, stts or ctts, covers the track's samples, and takes its
// checkpoints: the samples before each, and for stts their duration.
func (found *track) scanRuns(file io.ReaderAt, placed *table, isTimes bool) error {
	var samples, time int64
	isHuge := false
	err := eachEntry(file, placed, 0, scanBlock(placed), func(number int64, entry []byte) bool {
		if number%checkpointEvery == 0 {
			placed.marks = append(placed.marks, mark{samples: samples, time: time})
		}
		count := int64(binary.BigEndian.Uint32(entry))
		samples += count
		if isTimes {
			time += count * int64(binary.BigEndian.Uint32(entry[4:]))
		} else if number+2 < placed.entries {
			offset := int64(int32(binary.BigEndian.Uint32(entry[4:]))) //nolint:gosec // signed, as FFmpeg reads it
			isHuge = isHuge || offset < -hugeOffsetTicks || offset > hugeOffsetTicks
			found.shiftTicks = max(found.shiftTicks, -offset)
		}
		return true
	})
	switch {
	case err != nil:
		return err
	case samples != found.samples:
		return fmt.Errorf("%w: an %s of %d samples for %d", ErrCannotTrim, placed.kind, samples, found.samples)
	case isHuge:
		return fmt.Errorf("%w: a ctts FFmpeg drops whole, an offset past %d ticks", ErrCannotTrim, hugeOffsetTicks)
	}
	if isTimes {
		found.durationTicks = time
	}
	return nil
}

// hugeOffsetTicks is the composition offset past which FFmpeg drops a track's ctts whole, but in
// its last two runs, as it does in moving its times.
const hugeOffsetTicks = 1 << 28

// scanSizes adds up the sizes of the track's samples, when they are not all of one size.
func (found *track) scanSizes(file io.ReaderAt) error {
	if found.stsz.entryBytes == 0 {
		return nil
	}
	return eachEntry(file, found.stsz, 0, scanBlock(found.stsz), func(_ int64, entry []byte) bool {
		found.dataBytes += int64(binary.BigEndian.Uint32(entry))
		return true
	})
}

// scanGroups checks an sbgp covers no more than the track's samples, and takes its checkpoints.
func (found *track) scanGroups(file io.ReaderAt, placed *table) error {
	var samples int64
	err := eachEntry(file, placed, 0, scanBlock(placed), func(number int64, entry []byte) bool {
		if number%checkpointEvery == 0 {
			placed.marks = append(placed.marks, mark{samples: samples})
		}
		samples += int64(binary.BigEndian.Uint32(entry))
		return true
	})
	if err == nil && samples > found.samples {
		return fmt.Errorf("%w: an sbgp of %d samples for %d", ErrCannotTrim, samples, found.samples)
	}
	return err
}

// scanChunks checks stsc's runs of chunks, each of one description, cover the track's samples, and
// takes its checkpoints: the samples before each run's first chunk.
func (found *track) scanChunks(file io.ReaderAt) error {
	var samples, lastChunk, lastPerChunk int64
	bad := error(nil)
	err := eachEntry(file, found.stsc, 0, scanBlock(found.stsc), func(number int64, entry []byte) bool {
		firstChunk := int64(binary.BigEndian.Uint32(entry))
		perChunk := int64(binary.BigEndian.Uint32(entry[4:]))
		switch {
		case number == 0 && firstChunk != 1, number > 0 && firstChunk <= lastChunk, firstChunk > found.chunks:
			bad = fmt.Errorf("%w: an stsc whose chunks do not run in order", ErrCannotTrim)
		case perChunk == 0, binary.BigEndian.Uint32(entry[8:]) != 1:
			bad = fmt.Errorf("%w: an stsc run of no samples or not of the one description", ErrCannotTrim)
		}
		if bad != nil {
			return false
		}
		if number > 0 {
			samples += (firstChunk - lastChunk) * lastPerChunk
		}
		if number%checkpointEvery == 0 {
			found.stsc.marks = append(found.stsc.marks, mark{samples: samples})
		}
		lastChunk, lastPerChunk = firstChunk, perChunk
		return true
	})
	if failed := cmpErr(err, bad); failed != nil {
		return failed
	}
	if found.stsc.entries == 0 || samples+(found.chunks-lastChunk+1)*lastPerChunk < found.samples {
		return fmt.Errorf("%w: chunks holding fewer samples than the track", ErrCannotTrim)
	}
	return nil
}

// scanSyncs checks stss's sample numbers ascend within the track, and takes its checkpoints: the
// number at each.
func (found *track) scanSyncs(file io.ReaderAt) error {
	var last int64
	bad := false
	err := eachEntry(file, found.stss, 0, scanBlock(found.stss), func(entryAt int64, entry []byte) bool {
		number := int64(binary.BigEndian.Uint32(entry))
		if number <= last || number > found.samples {
			bad = true
			return false
		}
		if entryAt%checkpointEvery == 0 {
			found.stss.marks = append(found.stss.marks, mark{samples: number})
		}
		last = number
		return true
	})
	if err == nil && (bad || found.stss.entries == 0) {
		return fmt.Errorf("%w: an stss whose samples do not ascend within the track", ErrCannotTrim)
	}
	return err
}

// markBefore is the entry of the last checkpoint at or before what is, by key.
func markBefore(placed *table, isAfter func(mark) bool) int64 {
	n := sort.Search(len(placed.marks), func(at int) bool { return isAfter(placed.marks[at]) })
	return int64(max(n-1, 0)) * checkpointEvery
}
