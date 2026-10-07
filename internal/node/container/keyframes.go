// SPDX-License-Identifier: GPL-3.0-or-later

package container

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
)

// ErrNoIndex is a file whose container has no index of its picture's keyframes that this package
// reads: an AVI, an MPEG-TS, a Matroska file without cues, a fragmented MP4. Its keyframes are read
// through instead (D259 §2, §3).
var ErrNoIndex = errors.New("the file has no index of its picture's keyframes")

// Bounds on the indexes a hostile file can make the node read.
const (
	maxIndexElementBytes = 32 << 20
	maxMoovBytes         = 64 << 20
	maxInfoBytes         = 64 << 10
	maxTracksBytes       = 4 << 20
)

// matroskaVideoTrackType is a Matroska TrackEntry's TrackType for a picture.
const matroskaVideoTrackType = 1

// matroskaDefaultTimecodeScale is a Matroska file's tick, in nanoseconds, where its Info says none.
const matroskaDefaultTimecodeScale = 1_000_000

// KeyframesMs are the keyframes of the picture at streamIndex, as ffprobe numbers a file's streams,
// in milliseconds of the film, ascending, read from the file's own index without reading its media
// (D259 §2): a Matroska file's cues for that track, or an MP4's sync samples, shown at their
// composition times less the edit's start. A file with no such index is ErrNoIndex, and a failure
// to read the file is a ReadError.
func KeyframesMs(file io.ReaderAt, sizeBytes int64, streamIndex int) ([]int64, error) {
	for _, read := range []func(io.ReaderAt, int64, int) ([]int64, error){matroskaKeyframesMs, mp4KeyframesMs} {
		keyframes, err := read(file, sizeBytes, streamIndex)
		if err == nil && len(keyframes) > 0 {
			return keyframes, nil
		}
		if readErr, isReadError := errors.AsType[*ReadError](err); isReadError {
			return nil, readErr
		}
	}
	return nil, ErrNoIndex
}

// matroskaKeyframesMs reads the cues of a Matroska file's track at streamIndex, which must be a
// picture's: each cue's time, in the segment's ticks, as milliseconds.
func matroskaKeyframesMs(file io.ReaderAt, sizeBytes int64, streamIndex int) ([]int64, error) {
	header, err := ReadElement(file, sizeBytes, 0)
	if err != nil || header.ID != IDEBML {
		return nil, fmt.Errorf("%w: no EBML header", ErrNotThisContainer)
	}
	segment, err := ReadElement(file, sizeBytes, header.End())
	if err != nil || segment.ID != IDSegment {
		return nil, fmt.Errorf("%w: no Segment after the EBML header", ErrNotThisContainer)
	}
	found, err := findSegmentElements(file, sizeBytes, segment)
	if err != nil {
		return nil, err
	}
	trackNumber, err := matroskaVideoTrackNumber(file, sizeBytes, found.tracks, streamIndex)
	if err != nil {
		return nil, err
	}
	scale, err := matroskaTickNs(file, sizeBytes, found.info)
	if err != nil {
		return nil, err
	}
	if found.cues == nil {
		return nil, fmt.Errorf("%w: a Matroska file without cues", ErrNoIndex)
	}
	points, err := ReadChildren(file, sizeBytes, *found.cues, maxIndexElementBytes, 3)
	if err != nil {
		return nil, err
	}
	var keyframes []int64
	for _, point := range points {
		if time, isCued := cueTimeForTrack(point, trackNumber); isCued {
			keyframes = append(keyframes, int64(time*scale/1_000_000)) //nolint:gosec // a film's ticks fit
		}
	}
	slices.Sort(keyframes)
	return slices.Compact(keyframes), nil
}

// matroskaTickNs is a Matroska segment's tick, in nanoseconds, as its Info says, or the default.
func matroskaTickNs(file io.ReaderAt, sizeBytes int64, info *Element) (uint64, error) {
	scale := uint64(matroskaDefaultTimecodeScale)
	if info == nil {
		return scale, nil
	}
	fields, err := ReadChildren(file, sizeBytes, *info, maxInfoBytes, 1)
	if err != nil {
		return 0, err
	}
	for _, field := range fields {
		if field.ID == IDTimecodeScale && field.Integer() > 0 {
			scale = field.Integer()
		}
	}
	return scale, nil
}

// segmentElements are the elements of a Matroska segment a keyframe index needs: its Info, its
// Tracks and its Cues, each nil where the file has none.
type segmentElements struct {
	info, tracks, cues *Element
}

// keep keeps element when it is one of the segment's Info, Tracks or Cues.
func (found *segmentElements) keep(element Element) {
	switch element.ID {
	case IDInfo:
		found.info = &element
	case IDTracks:
		found.tracks = &element
	case IDCues:
		found.cues = &element
	}
}

// findSegmentElements steps over the segment's top-level elements to its first Cluster, keeping its
// Info, Tracks and Cues, and finds Cues placed after the clusters through the SeekHead.
func findSegmentElements(file io.ReaderAt, sizeBytes int64, segment Element) (segmentElements, error) {
	segmentEnd := sizeBytes
	if segment.DataSize != UnknownSize {
		segmentEnd = min(segment.End(), sizeBytes)
	}
	var found segmentElements
	cuesPosition := int64(-1)
	position := segment.DataStart
	for range MaxElementHops {
		if position >= segmentEnd {
			break
		}
		element, err := ReadElement(file, sizeBytes, position)
		if err != nil {
			return segmentElements{}, err
		}
		if element.ID == IDCluster || element.DataSize == UnknownSize {
			break
		}
		if element.ID == IDSeekHead {
			if seeked := seekedPosition(file, sizeBytes, element, segment.DataStart, IDCues); seeked >= 0 {
				cuesPosition = seeked
			}
		}
		found.keep(element)
		position = element.End()
	}
	if found.cues == nil && cuesPosition >= 0 {
		if cues, err := ReadElement(file, sizeBytes, cuesPosition); err == nil && cues.ID == IDCues {
			found.cues = &cues
		}
	}
	return found, nil
}

// seekedPosition is the absolute position a SeekHead gives the element target, or -1.
func seekedPosition(file io.ReaderAt, sizeBytes int64, seekHead Element, segmentDataStart int64, target uint64) int64 {
	seeks, err := ReadChildren(file, sizeBytes, seekHead, maxInfoBytes, 2)
	if err != nil {
		return -1
	}
	for _, seek := range seeks {
		var id, position uint64
		for _, field := range seek.Children {
			switch field.ID {
			case IDSeekID:
				id = field.Integer()
			case IDSeekPosition:
				position = field.Integer()
			}
		}
		if seek.ID == IDSeek && id == target && position < uint64(sizeBytes) { //nolint:gosec // sizeBytes >= 0
			return segmentDataStart + int64(position)
		}
	}
	return -1
}

// matroskaVideoTrackNumber is the TrackNumber of the segment's track at streamIndex, ffprobe's
// numbering of a Matroska file's tracks in their order, when it is a picture.
func matroskaVideoTrackNumber(file io.ReaderAt, sizeBytes int64, tracks *Element, streamIndex int) (uint64, error) {
	if tracks == nil {
		return 0, fmt.Errorf("%w: a Matroska file without tracks", ErrNotThisContainer)
	}
	entries, err := ReadChildren(file, sizeBytes, *tracks, maxTracksBytes, 2)
	if err != nil {
		return 0, err
	}
	index := 0
	for _, entry := range entries {
		if entry.ID != IDTrackEntry {
			continue
		}
		if index == streamIndex {
			var number, kind uint64
			for _, field := range entry.Children {
				switch field.ID {
				case IDTrackNumber:
					number = field.Integer()
				case IDTrackType:
					kind = field.Integer()
				}
			}
			if kind != matroskaVideoTrackType || number == 0 {
				return 0, fmt.Errorf("%w: track %d is not a picture", ErrNoIndex, streamIndex)
			}
			return number, nil
		}
		index++
	}
	return 0, fmt.Errorf("%w: no track %d", ErrNoIndex, streamIndex)
}

// cueTimeForTrack is a CuePoint's time when one of its positions is on track.
func cueTimeForTrack(point Node, track uint64) (uint64, bool) {
	if point.ID != IDCuePoint {
		return 0, false
	}
	var time uint64
	hasTime, isOnTrack := false, false
	for _, field := range point.Children {
		switch field.ID {
		case IDCueTime:
			time, hasTime = field.Integer(), true
		case IDCueTrackPositions:
			for _, position := range field.Children {
				if position.ID == IDCueTrack && position.Integer() == track {
					isOnTrack = true
				}
			}
		}
	}
	return time, hasTime && isOnTrack
}

// mp4KeyframesMs reads an MP4's moov for the track at streamIndex, ffprobe's numbering of its traks
// in order, which must be a picture's with a sync-sample table.
func mp4KeyframesMs(file io.ReaderAt, sizeBytes int64, streamIndex int) ([]int64, error) {
	first, err := ReadBox(file, sizeBytes, 0)
	if err != nil || first.Kind != "ftyp" {
		return nil, fmt.Errorf("%w: no ftyp box first", ErrNotThisContainer)
	}
	moov, err := topLevelBox(file, sizeBytes, "moov")
	if err != nil {
		return nil, err
	}
	if moov.End-moov.DataStart > maxMoovBytes {
		return nil, fmt.Errorf("%w: a moov of %d bytes", ErrNoIndex, moov.End-moov.DataStart)
	}
	data := make([]byte, moov.End-moov.DataStart)
	if readErr := ReadAt(file, sizeBytes, moov.DataStart, data); readErr != nil {
		return nil, readErr
	}
	trak, err := nthChild(data, "trak", streamIndex)
	if err != nil {
		return nil, err
	}
	return trakKeyframesMs(trak)
}

// topLevelBox is the file's first top-level box of kind.
func topLevelBox(file io.ReaderAt, sizeBytes int64, kind string) (Box, error) {
	position := int64(0)
	for range MaxElementHops {
		if position >= sizeBytes {
			break
		}
		found, err := ReadBox(file, sizeBytes, position)
		if err != nil {
			return Box{}, err
		}
		if found.Kind == kind {
			return found, nil
		}
		position = found.End
	}
	return Box{}, fmt.Errorf("%w: no %s box", ErrNoIndex, kind)
}

// children are the boxes data holds, by kind, in order, each its data.
func children(data []byte) ([]Box, error) {
	reader := bytes.NewReader(data)
	size := int64(len(data))
	var boxes []Box
	for position := int64(0); position < size; {
		found, err := ReadBox(reader, size, position)
		if err != nil {
			return nil, err
		}
		boxes = append(boxes, found)
		position = found.End
	}
	return boxes, nil
}

// child is the data of the first box of kind among data's, and whether there is one.
func child(data []byte, kind string) ([]byte, bool) {
	boxes, err := children(data)
	if err != nil {
		return nil, false
	}
	for _, found := range boxes {
		if found.Kind == kind {
			return data[found.DataStart:found.End], true
		}
	}
	return nil, false
}

// nthChild is the data of the index-th box of kind among data's.
func nthChild(data []byte, kind string, index int) ([]byte, error) {
	boxes, err := children(data)
	if err != nil {
		return nil, err
	}
	for _, found := range boxes {
		if found.Kind != kind {
			continue
		}
		if index == 0 {
			return data[found.DataStart:found.End], nil
		}
		index--
	}
	return nil, fmt.Errorf("%w: no %s %d", ErrNoIndex, kind, index)
}

// path is the data of the box reached through kinds, each the first of its kind inside the last.
func path(data []byte, kinds ...string) ([]byte, bool) {
	for _, kind := range kinds {
		var found bool
		if data, found = child(data, kind); !found {
			return nil, false
		}
	}
	return data, true
}

// trakKeyframesMs are a picture trak's sync samples' times: each one's decode time from stts, plus
// its composition offset from ctts, less the edit list's media start, in its media's timescale.
func trakKeyframesMs(trak []byte) ([]int64, error) {
	handler, hasHandler := path(trak, "mdia", "hdlr")
	if !hasHandler || len(handler) < 12 || string(handler[8:12]) != "vide" {
		return nil, fmt.Errorf("%w: the track is not a picture", ErrNoIndex)
	}
	timescale, err := mediaTimescale(trak)
	if err != nil {
		return nil, err
	}
	stbl, hasTable := path(trak, "mdia", "minf", "stbl")
	syncTable, hasSync := child(stbl, "stss")
	timeTable, hasTimes := child(stbl, "stts")
	if !hasTable || !hasSync || !hasTimes {
		return nil, fmt.Errorf("%w: no sync-sample table", ErrNoIndex)
	}
	syncSamples, err := fullBoxEntries(syncTable, 4)
	if err != nil {
		return nil, err
	}
	decodeTimes, err := sampleTimes(timeTable, syncSamples)
	if err != nil {
		return nil, err
	}
	offsetTable, hasOffsets := child(stbl, "ctts")
	start := editMediaStart(trak)
	keyframes := make([]int64, 0, len(syncSamples))
	for index, sample := range syncSamples {
		shown := decodeTimes[index] - start
		if hasOffsets {
			shown += compositionOffset(offsetTable, sample)
		}
		if shown >= 0 {
			keyframes = append(keyframes, shown*1000/timescale)
		}
	}
	slices.Sort(keyframes)
	return slices.Compact(keyframes), nil
}

// mediaTimescale is a trak's media timescale, from its mdhd.
func mediaTimescale(trak []byte) (int64, error) {
	header, found := path(trak, "mdia", "mdhd")
	if !found || len(header) < 24 {
		return 0, fmt.Errorf("%w: no media header", ErrNoIndex)
	}
	timescaleAt := 12 // version 0: creation and modification times of 4 bytes each, after version and flags
	if header[0] == 1 {
		timescaleAt = 20
	}
	timescale := int64(binary.BigEndian.Uint32(header[timescaleAt : timescaleAt+4]))
	if timescale <= 0 {
		return 0, fmt.Errorf("%w: a timescale of 0", ErrNoIndex)
	}
	return timescale, nil
}

// fullBoxEntries are a full box's entries of entryBytes each, read as their first 32-bit word,
// after its version, flags and entry count: stss's sample numbers.
func fullBoxEntries(data []byte, entryBytes int) ([]uint32, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("%w: a table too short", ErrNoIndex)
	}
	count := int(binary.BigEndian.Uint32(data[4:8]))
	if count < 0 || count > (len(data)-8)/entryBytes {
		return nil, fmt.Errorf("%w: a table of %d entries in %d bytes", ErrNoIndex, count, len(data))
	}
	entries := make([]uint32, count)
	for index := range entries {
		entries[index] = binary.BigEndian.Uint32(data[8+index*entryBytes:])
	}
	return entries, nil
}

// sampleTimes are the decode times of samples, 1-based numbers ascending, from an stts table of runs
// of a count of samples and each one's duration.
func sampleTimes(stts []byte, samples []uint32) ([]int64, error) {
	runs, err := runsOf(stts)
	if err != nil {
		return nil, err
	}
	times := make([]int64, 0, len(samples))
	var first, decodedAt int64 = 1, 0 // the run's first sample and its decode time
	next := 0
	for _, run := range runs {
		for next < len(samples) && int64(samples[next]) < first+run.count {
			times = append(times, decodedAt+(int64(samples[next])-first)*run.value)
			next++
		}
		first += run.count
		decodedAt += run.count * run.value
	}
	if next != len(samples) {
		return nil, fmt.Errorf("%w: a sync sample past the time table", ErrNoIndex)
	}
	return times, nil
}

// run is one entry of stts or ctts: a count of samples and their duration or offset.
type run struct{ count, value int64 }

// runsOf are a table's runs: stts, or ctts, whose version 1 offsets are signed.
func runsOf(table []byte) ([]run, error) {
	if len(table) < 8 {
		return nil, fmt.Errorf("%w: a table too short", ErrNoIndex)
	}
	isSigned := table[0] == 1
	count := int(binary.BigEndian.Uint32(table[4:8]))
	if count < 0 || count > (len(table)-8)/8 {
		return nil, fmt.Errorf("%w: a table of %d runs in %d bytes", ErrNoIndex, count, len(table))
	}
	runs := make([]run, count)
	for index := range runs {
		entry := table[8+index*8:]
		value := int64(binary.BigEndian.Uint32(entry[4:8]))
		if isSigned {
			value = int64(int32(binary.BigEndian.Uint32(entry[4:8]))) //nolint:gosec // a signed offset
		}
		runs[index] = run{count: int64(binary.BigEndian.Uint32(entry[0:4])), value: value}
	}
	return runs, nil
}

// compositionOffset is sample's offset in a ctts table, 1-based; 0 past its end.
func compositionOffset(ctts []byte, sample uint32) int64 {
	runs, err := runsOf(ctts)
	if err != nil {
		return 0
	}
	first := int64(1)
	for _, entry := range runs {
		if int64(sample) < first+entry.count {
			return entry.value
		}
		first += entry.count
	}
	return 0
}

// editMediaStart is where a trak's edit list starts its media, in the media's timescale: the first
// edit that is not empty, as FFmpeg shows the media from there; 0 without one.
func editMediaStart(trak []byte) int64 {
	list, found := path(trak, "edts", "elst")
	if !found || len(list) < 8 {
		return 0
	}
	isWide := list[0] == 1
	entryBytes := 12
	if isWide {
		entryBytes = 20
	}
	count := int(binary.BigEndian.Uint32(list[4:8]))
	for index := range min(count, (len(list)-8)/entryBytes) {
		entry := list[8+index*entryBytes:]
		var mediaTime int64
		if isWide {
			mediaTime = int64(binary.BigEndian.Uint64(entry[8:16])) //nolint:gosec // a signed time
		} else {
			mediaTime = int64(int32(binary.BigEndian.Uint32(entry[4:8]))) //nolint:gosec // a signed time
		}
		if mediaTime >= 0 {
			return mediaTime
		}
	}
	return 0
}
