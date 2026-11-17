// SPDX-License-Identifier: AGPL-3.0-or-later

package container

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// headMs is how much of each track from its edit's start a cut-down index keeps besides its
// stretch: FFmpeg reads a film's first packets as it opens it, and times the film from them (its
// start, its frame rate, an edit's first sound), so they stay as the film has them.
const headMs = 10_000

// maxSampleDuration is the longest duration one sample of a cut-down index takes in stts: FFmpeg
// reads a duration as signed.
const maxSampleDuration = math.MaxInt32

// Trimmed is a film's movie box cut down to a stretch (D512), in its place: from At, Head, then zeros
// to At+Span, the padding of the free box that ends Head, so the file keeps its length and every
// sample its offset.
type Trimmed struct {
	At, Span int64
	Head     []byte
}

// ReadAt reads what the cut-down file holds at offset into buffer from Head and its zeros, and
// says how much of buffer lies before the movie box's place and how much after it.
func (trimmed Trimmed) ReadAt(buffer []byte, offset int64) (before, after int) {
	end := offset + int64(len(buffer))
	if end <= trimmed.At || offset >= trimmed.At+trimmed.Span {
		if end <= trimmed.At {
			return len(buffer), 0
		}
		return 0, len(buffer)
	}
	before = int(max(trimmed.At-offset, 0))
	inside := buffer[before:min(int64(len(buffer)), trimmed.At+trimmed.Span-offset)]
	from := max(offset-trimmed.At, 0)
	copied := 0
	if from < int64(len(trimmed.Head)) {
		copied = copy(inside, trimmed.Head[from:])
	}
	clear(inside[copied:])
	return before, len(buffer) - before - len(inside)
}

// Trim is the movie box cut down to the film's stretch fromMs to toMs, in the film's ms as FFmpeg
// times it: in each sliced track the samples of the stretch, a picture's from its sync sample at or
// before the stretch's start, after the track's first headMs. Every sample kept keeps its decode
// time, composition offset, size and offset: the last of the head lasts until the stretch's first.
func (index *Index) Trim(file io.ReaderAt, fromMs, toMs int64) (Trimmed, error) {
	if toMs < fromMs {
		return Trimmed{}, fmt.Errorf("%w: a stretch from %d ms to %d ms", ErrCannotTrim, fromMs, toMs)
	}
	replaced := map[*boxNode][]byte{}
	for _, found := range index.tracks {
		if err := found.slice(file, fromMs, toMs, index.filmStartMs, replaced); err != nil {
			return Trimmed{}, err
		}
	}
	head := appendNode(nil, index.root, replaced)
	padding := index.MoovBytes - int64(len(head))
	if padding < 8 {
		return Trimmed{}, fmt.Errorf("%w: the cut-down moov is no smaller", ErrCannotTrim)
	}
	head = append(head, boxHeader("free", padding)...)
	return Trimmed{At: index.MoovAt, Span: index.MoovBytes, Head: head}, nil
}

// appendNode appends node as the cut-down index has it: a table replaced, a box copied whole, or a
// container of its children.
func appendNode(out []byte, node *boxNode, replaced map[*boxNode][]byte) []byte {
	if table, found := replaced[node]; found {
		return append(out, table...)
	}
	if node.whole != nil {
		return append(out, node.whole...)
	}
	start := len(out)
	out = append(out, boxHeader(node.kind, 0)...)
	for _, inner := range node.children {
		out = appendNode(out, inner, replaced)
	}
	binary.BigEndian.PutUint32(out[start:], uint32(len(out)-start)) //nolint:gosec // within the moov's size
	return out
}

// boxHeader is a box's 32-bit header.
func boxHeader(kind string, size int64) []byte {
	header := binary.BigEndian.AppendUint32(nil, uint32(size)) //nolint:gosec // within the moov's size
	return append(header, kind...)
}

// span is the samples from, to, 0-based, to excluded, that a cut-down track keeps together.
type span struct{ from, to int64 }

// slice puts in replaced each of the track's tables cut down to the stretch.
func (found *track) slice(file io.ReaderAt, fromMs, toMs, filmStartMs int64, replaced map[*boxNode][]byte) error {
	spans, err := found.spans(file, fromMs, toMs, filmStartMs)
	if err != nil {
		return err
	}
	durations, err := found.durations(file, spans)
	if err != nil {
		return err
	}
	keptBytes, err := found.chunksBoxes(file, spans, replaced)
	if err != nil {
		return err
	}
	var keptTicks int64
	for _, next := range durations {
		keptTicks += next.count * next.value
	}
	tail, err := found.tailOf(keptTicks, keptBytes)
	if err != nil {
		return err
	}
	found.addTail(tail, replaced)
	for _, next := range tail {
		durations = appendRuns(durations, run{count: 1, value: next.duration})
	}
	replaced[found.stts.node] = runsBox("stts", found.stts.header, durations)
	return found.slicePerSample(file, spans, len(tail), replaced)
}

// slicePerSample puts in replaced the track's tables of a value per sample or per run of samples,
// cut down to the spans, the tail's tailSamples of them added: ctts, sbgp, stss and sdtp.
func (found *track) slicePerSample(
	file io.ReaderAt, spans []span, tailSamples int, replaced map[*boxNode][]byte,
) error {
	if found.ctts != nil {
		offsets, err := spanRuns(file, found.ctts, spans)
		if err != nil {
			return err
		}
		offsets = appendRuns(offsets, run{count: int64(tailSamples), value: 0})
		if shift := shiftOf(offsets); shift != found.shiftTicks {
			return fmt.Errorf("%w: a stretch whose times FFmpeg would move %d ticks, not the track's %d",
				ErrCannotTrim, shift, found.shiftTicks)
		}
		replaced[found.ctts.node] = runsBox("ctts", found.ctts.header, offsets)
	}
	for _, groups := range found.groups {
		runs, err := spanRuns(file, groups, spans)
		if err != nil {
			return err
		}
		replaced[groups.node] = runsBox(sbgpKind, groups.header, runs)
	}
	if found.stss != nil {
		syncs, err := found.syncsBox(file, spans)
		if err != nil {
			return err
		}
		replaced[found.stss.node] = syncs
	}
	if found.sdtp != nil {
		dependencies, err := found.dependenciesBox(file, spans, tailSamples)
		if err != nil {
			return err
		}
		replaced[found.sdtp.node] = dependencies
	}
	return nil
}

// spanRuns are a table of runs' runs over the spans, joined.
func spanRuns(file io.ReaderAt, placed *table, spans []span) ([]run, error) {
	var runs []run
	for _, kept := range spans {
		inSpan, err := runsIn(file, placed, kept)
		if err != nil {
			return nil, err
		}
		runs = appendRuns(runs, inSpan...)
	}
	return runs, nil
}

// dependenciesBox is the track's sdtp cut down to the spans, a byte of no dependencies said for
// each of the tail's tailSamples.
func (found *track) dependenciesBox(file io.ReaderAt, spans []span, tailSamples int) ([]byte, error) {
	dependencies := append([]byte(nil), found.sdtp.header...)
	for _, kept := range spans {
		inSpan := make([]byte, kept.to-kept.from)
		if err := found.readArray(file, found.sdtp, kept.from, inSpan); err != nil {
			return nil, err
		}
		dependencies = append(dependencies, inSpan...)
	}
	dependencies = append(dependencies, make([]byte, tailSamples)...)
	return append(boxHeader(sdtpKind, int64(8+len(dependencies))), dependencies...), nil
}

// maxTailSampleBytes is the largest sample size FFmpeg takes in an index.
const maxTailSampleBytes = 0x3FFFFFFF

// maxTailSamples bounds a cut-down track's tail.
const maxTailSamples = 4096

// tailSample is one of the samples a cut-down track ends with, past its stretch, which no run
// reaches: together they hold the bytes and last the time of the samples cut away, so FFmpeg says
// the track's bit rate as it says the whole file's, which a copy writes in its init.
type tailSample struct{ size, duration int64 }

// tailOf is the tail of a track whose kept samples last keptTicks and hold keptBytes: none for a
// track of samples of one size, whose bit rate FFmpeg does not say.
func (found *track) tailOf(keptTicks, keptBytes int64) ([]tailSample, error) {
	if found.stsz.entryBytes == 0 {
		return nil, nil
	}
	restBytes, restTicks := found.dataBytes-keptBytes, found.durationTicks-keptTicks
	if restBytes < 0 || restTicks < 0 {
		return nil, fmt.Errorf("%w: a stretch of more than its track", ErrCannotTrim)
	}
	count := max((restBytes+maxTailSampleBytes-1)/maxTailSampleBytes,
		(restTicks+maxSampleDuration-1)/maxSampleDuration, 1)
	if count > maxTailSamples {
		return nil, fmt.Errorf("%w: a track of %d bytes past its stretch", ErrCannotTrim, restBytes)
	}
	tail := make([]tailSample, count)
	for number := range tail {
		tail[number] = tailSample{size: restBytes / count, duration: restTicks / count}
		if int64(number) < restBytes%count {
			tail[number].size++
		}
		if int64(number) < restTicks%count {
			tail[number].duration++
		}
	}
	return tail, nil
}

// addTail adds the tail to the track's sizes and chunks in replaced: one chunk, at the file's end.
func (found *track) addTail(tail []tailSample, replaced map[*boxNode][]byte) {
	if len(tail) == 0 {
		return
	}
	tailCount := uint32(len(tail)) //nolint:gosec // bounded by maxTailSamples
	stsz := replaced[found.stsz.node]
	addCount(stsz[8+len(found.stsz.header):], tailCount)
	for _, next := range tail {
		stsz = binary.BigEndian.AppendUint32(stsz, uint32(next.size)) //nolint:gosec // bounded by maxTailSampleBytes
	}
	replaced[found.stsz.node] = resized(stsz)
	offsets := replaced[found.offset.node]
	chunks := addCount(offsets[8+len(found.offset.header):], 1)
	if found.offset.kind == co64Kind {
		offsets = binary.BigEndian.AppendUint64(offsets, uint64(found.fileSize)) //nolint:gosec // a file's size
	} else {
		lastOffset := uint32(min(found.fileSize, math.MaxUint32)) //nolint:gosec // bounded by MaxUint32
		offsets = binary.BigEndian.AppendUint32(offsets, lastOffset)
	}
	replaced[found.offset.node] = resized(offsets)
	stsc := replaced[found.stsc.node]
	addCount(stsc[8+len(found.stsc.header):], 1)
	stsc = binary.BigEndian.AppendUint32(stsc, chunks)
	stsc = binary.BigEndian.AppendUint32(stsc, tailCount)
	replaced[found.stsc.node] = resized(binary.BigEndian.AppendUint32(stsc, 1))
}

// addCount adds more to the count of entries at the start of entries, and says the new count.
func addCount(entries []byte, more uint32) uint32 {
	count := binary.BigEndian.Uint32(entries) + more
	binary.BigEndian.PutUint32(entries, count)
	return count
}

// resized is a box whose header says its size again after it has grown.
func resized(box []byte) []byte {
	binary.BigEndian.PutUint32(box, uint32(len(box))) //nolint:gosec // a cut-down table
	return box
}

// shiftOf is how far FFmpeg moves a track's times by its ctts runs: the least offset, if negative,
// of all but the last two.
func shiftOf(offsets []run) int64 {
	var shift int64
	for at := 0; at+2 < len(offsets); at++ {
		shift = max(shift, -int64(int32(offsets[at].value))) //nolint:gosec // signed, as FFmpeg reads it
	}
	return shift
}

// spans are the samples the track keeps: its head, and the stretch's, joined when they meet.
func (found *track) spans(file io.ReaderAt, fromMs, toMs, filmStartMs int64) ([]span, error) {
	tickAt := func(ms int64) int64 {
		return (ms+filmStartMs-found.emptyMs)*found.timescale/1000 + found.editStart
	}
	first, err := found.sampleAt(file, tickAt(fromMs))
	if err != nil {
		return nil, err
	}
	if found.stss != nil {
		if first, err = found.syncAtOrBefore(file, first); err != nil {
			return nil, err
		}
	}
	last, err := found.sampleAt(file, tickAt(toMs))
	if err != nil {
		return nil, err
	}
	headLast, err := found.sampleAt(file, found.editStart+headMs*found.timescale/1000)
	if err != nil {
		return nil, err
	}
	if headLast+1 >= first {
		return []span{{from: 0, to: last + 1}}, nil
	}
	return []span{{from: 0, to: headLast + 1}, {from: first, to: last + 1}}, nil
}

// durations are the kept samples' durations: each its own, but the head's last ones, which last
// until the stretch's first sample, each no longer than maxSampleDuration.
func (found *track) durations(file io.ReaderAt, spans []span) ([]run, error) {
	head, err := runsIn(file, found.stts, spans[0])
	if err != nil || len(spans) == 1 {
		return head, err
	}
	stretch, err := runsIn(file, found.stts, spans[1])
	if err != nil {
		return nil, err
	}
	headEnd, err := found.decodeTime(file, spans[0].to)
	if err != nil {
		return nil, err
	}
	stretchStart, err := found.decodeTime(file, spans[1].from)
	if err != nil {
		return nil, err
	}
	// The head's last samples, as few as the gap needs, take it on as well as their own durations.
	var count int64
	total := stretchStart - headEnd
	for count == 0 || total > count*maxSampleDuration {
		if len(head) == 0 {
			return nil, fmt.Errorf("%w: a head too short to span the stretch's start", ErrCannotTrim)
		}
		last := &head[len(head)-1]
		total += last.value
		count++
		last.count--
		if last.count == 0 {
			head = head[:len(head)-1]
		}
	}
	for at := range count {
		share := total / count
		if at < total%count {
			share++
		}
		head = appendRuns(head, run{count: 1, value: share})
	}
	return appendRuns(head, stretch...), nil
}

// appendRuns appends runs to runs, a run of the value the last one has joining it.
func appendRuns(runs []run, more ...run) []run {
	for _, next := range more {
		if next.count == 0 {
			continue
		}
		if len(runs) > 0 && runs[len(runs)-1].value == next.value {
			runs[len(runs)-1].count += next.count
			continue
		}
		runs = append(runs, next)
	}
	return runs
}

// runsBox is a table of runs, stts, ctts or sbgp, after its header as the file has it.
func runsBox(kind string, header []byte, runs []run) []byte {
	body := append([]byte(nil), header...)
	body = binary.BigEndian.AppendUint32(body, uint32(len(runs))) //nolint:gosec // bounded by the samples kept
	for _, next := range runs {
		body = binary.BigEndian.AppendUint32(body, uint32(next.count)) //nolint:gosec // bounded by the samples
		body = binary.BigEndian.AppendUint32(body, uint32(next.value)) //nolint:gosec // the file's own 32 bits
	}
	return append(boxHeader(kind, int64(8+len(body))), body...)
}

// runsIn are a table of runs' runs over the samples kept, each value its 32 bits as the file has
// them; a table that ends before them, as an sbgp may, has none past its end.
func runsIn(file io.ReaderAt, placed *table, kept span) ([]run, error) {
	if len(placed.marks) == 0 {
		return nil, nil
	}
	start := markBefore(placed, func(at mark) bool { return at.samples > kept.from })
	samples := placed.marks[start/checkpointEvery].samples
	var runs []run
	err := eachEntry(file, placed, start, checkpointEvery, func(_ int64, entry []byte) bool {
		count := int64(binary.BigEndian.Uint32(entry))
		from, to := max(samples, kept.from), min(samples+count, kept.to)
		if to > from {
			runs = appendRuns(runs, run{count: to - from, value: int64(binary.BigEndian.Uint32(entry[4:]))})
		}
		samples += count
		return samples < kept.to
	})
	return runs, err
}

// sampleAt is the sample decoded at tick: the last at or before it, the first before the track's
// start, the last past its end.
func (found *track) sampleAt(file io.ReaderAt, tick int64) (int64, error) {
	if tick <= 0 {
		return 0, nil
	}
	start := markBefore(found.stts, func(at mark) bool { return at.time > tick })
	samples, time := found.stts.marks[start/checkpointEvery].samples, found.stts.marks[start/checkpointEvery].time
	result := found.samples - 1
	err := eachEntry(file, found.stts, start, checkpointEvery, func(_ int64, entry []byte) bool {
		count, duration := int64(binary.BigEndian.Uint32(entry)), int64(binary.BigEndian.Uint32(entry[4:]))
		if duration > 0 && tick < time+count*duration {
			result = samples + (tick-time)/duration
			return false
		}
		samples += count
		time += count * duration
		return true
	})
	return min(result, found.samples-1), err
}

// decodeTime is the decode time of sample, 0-based.
func (found *track) decodeTime(file io.ReaderAt, sample int64) (int64, error) {
	start := markBefore(found.stts, func(at mark) bool { return at.samples > sample })
	samples, time := found.stts.marks[start/checkpointEvery].samples, found.stts.marks[start/checkpointEvery].time
	err := eachEntry(file, found.stts, start, checkpointEvery, func(_ int64, entry []byte) bool {
		count, duration := int64(binary.BigEndian.Uint32(entry)), int64(binary.BigEndian.Uint32(entry[4:]))
		if sample < samples+count {
			time += (sample - samples) * duration
			samples = sample
			return false
		}
		samples += count
		time += count * duration
		return true
	})
	return time, err
}

// syncAtOrBefore is the last sync sample at or before sample, 0-based; the track's first sample when
// there is none.
func (found *track) syncAtOrBefore(file io.ReaderAt, sample int64) (int64, error) {
	number := sample + 1
	start := markBefore(found.stss, func(at mark) bool { return at.samples > number })
	best := int64(0)
	err := eachEntry(file, found.stss, start, checkpointEvery, func(_ int64, entry []byte) bool {
		if value := int64(binary.BigEndian.Uint32(entry)); value <= number {
			best = value
			return true
		}
		return false
	})
	return max(best-1, 0), err
}

// syncsBox is stss of the kept samples' sync samples, numbered as they are kept.
func (found *track) syncsBox(file io.ReaderAt, spans []span) ([]byte, error) {
	body := append([]byte(nil), found.stss.header...)
	body = append(body, 0, 0, 0, 0)
	var count, keptBefore int64
	for _, kept := range spans {
		start := markBefore(found.stss, func(at mark) bool { return at.samples > kept.from+1 })
		err := eachEntry(file, found.stss, start, checkpointEvery, func(_ int64, entry []byte) bool {
			value := int64(binary.BigEndian.Uint32(entry)) - 1
			if value >= kept.to {
				return false
			}
			if value >= kept.from {
				body = binary.BigEndian.AppendUint32(body, uint32(keptBefore+value-kept.from+1)) //nolint:gosec // kept
				count++
			}
			return true
		})
		if err != nil {
			return nil, err
		}
		keptBefore += kept.to - kept.from
	}
	binary.BigEndian.PutUint32(body[len(found.stss.header):], uint32(count)) //nolint:gosec // bounded by the samples
	return append(boxHeader("stss", int64(8+len(body))), body...), nil
}

// readArray reads into values the entries of an array table, stsz, sdtp or a chunk offset table,
// from first.
func (found *track) readArray(file io.ReaderAt, placed *table, first int64, values []byte) error {
	if int64(len(values)) > maxSliceEntries*max(placed.entryBytes, 1) {
		return fmt.Errorf("%w: a stretch of more than %d entries", ErrCannotTrim, maxSliceEntries)
	}
	if first < 0 || first*placed.entryBytes+int64(len(values)) > placed.entries*placed.entryBytes {
		return fmt.Errorf("%w: entries past the end of %s", ErrCannotTrim, placed.kind)
	}
	position := placed.entriesAt + first*placed.entryBytes
	return cannotTrim(ReadAt(file, position+int64(len(values)), position, values))
}

// stscEntry is a run of chunks of stsc: its first chunk, 1-based, its samples per chunk, and the
// samples before it.
type stscEntry struct{ firstChunk, perChunk, samplesBefore int64 }

// chunksOf are the runs of stsc over the samples kept.
func (found *track) chunksOf(file io.ReaderAt, kept span) ([]stscEntry, error) {
	start := markBefore(found.stsc, func(at mark) bool { return at.samples > kept.from })
	samples := found.stsc.marks[start/checkpointEvery].samples
	var entries []stscEntry
	err := eachEntry(file, found.stsc, start, checkpointEvery, func(_ int64, entry []byte) bool {
		next := stscEntry{
			firstChunk: int64(binary.BigEndian.Uint32(entry)), perChunk: int64(binary.BigEndian.Uint32(entry[4:])),
		}
		if len(entries) > 0 {
			last := entries[len(entries)-1]
			samples += (next.firstChunk - last.firstChunk) * last.perChunk
		}
		if samples >= kept.to {
			return false
		}
		next.samplesBefore = samples
		entries = append(entries, next)
		return true
	})
	return entries, err
}

// chunk is a cut-down track's chunk: where its first kept sample is and how many it holds.
type chunk struct{ offset, samples int64 }

// chunksBoxes puts in replaced the kept samples' sizes (stsz) and their chunks (stsc, and stco or
// co64): every chunk the samples kept lie in, the first of a span from its first sample kept. It
// says how many bytes the samples kept hold by stsz's sizes: none for samples of one size.
func (found *track) chunksBoxes(file io.ReaderAt, spans []span, replaced map[*boxNode][]byte) (int64, error) {
	var sizes []byte
	var chunks []chunk
	var keptSamples int64
	for _, kept := range spans {
		inSpan, spanSizes, err := found.chunksIn(file, kept)
		if err != nil {
			return 0, err
		}
		chunks = append(chunks, inSpan...)
		sizes = append(sizes, spanSizes...)
		keptSamples += kept.to - kept.from
	}
	stsz := append([]byte(nil), found.stsz.header...) // its version, flags and one size
	stsz = binary.BigEndian.AppendUint32(stsz, uint32(keptSamples))
	stsz = append(stsz, sizes...)
	replaced[found.stsz.node] = append(boxHeader("stsz", int64(8+len(stsz))), stsz...)
	stsc := append([]byte(nil), found.stsc.header...)
	stsc = append(stsc, 0, 0, 0, 0)
	var stscCount uint32
	isWide := found.offset.kind == "co64"
	offsets := append([]byte(nil), found.offset.header...)
	offsets = binary.BigEndian.AppendUint32(offsets, uint32(len(chunks))) //nolint:gosec // bounded by the samples
	for at, next := range chunks {
		if at == 0 || chunks[at-1].samples != next.samples {
			stsc = binary.BigEndian.AppendUint32(stsc, uint32(at+1))
			stsc = binary.BigEndian.AppendUint32(stsc, uint32(next.samples)) //nolint:gosec // bounded
			stsc = binary.BigEndian.AppendUint32(stsc, 1)
			stscCount++
		}
		if isWide {
			offsets = binary.BigEndian.AppendUint64(offsets, uint64(next.offset)) //nolint:gosec // within the file
		} else {
			offsets = binary.BigEndian.AppendUint32(offsets, uint32(next.offset)) //nolint:gosec // within stco's own
		}
	}
	binary.BigEndian.PutUint32(stsc[len(found.stsc.header):], stscCount)
	replaced[found.stsc.node] = append(boxHeader("stsc", int64(8+len(stsc))), stsc...)
	replaced[found.offset.node] = append(boxHeader(found.offset.kind, int64(8+len(offsets))), offsets...)
	var keptBytes int64
	for at := 0; at+4 <= len(sizes); at += 4 {
		keptBytes += int64(binary.BigEndian.Uint32(sizes[at:]))
	}
	return keptBytes, nil
}

// chunksIn are the chunks of the samples kept, and those samples' sizes as stsz lists them.
func (found *track) chunksIn(file io.ReaderAt, kept span) ([]chunk, []byte, error) {
	entries, err := found.chunksOf(file, kept)
	if err != nil {
		return nil, nil, err
	}
	if len(entries) == 0 {
		return nil, nil, fmt.Errorf("%w: samples in no chunk", ErrCannotTrim)
	}
	// Where each sample lies: its chunk, 1-based, and the chunk's first sample.
	entryAt := 0
	locate := func(sample int64) (number, firstSample, perChunk int64) {
		for entryAt+1 < len(entries) && entries[entryAt+1].samplesBefore <= sample {
			entryAt++
		}
		entry := entries[entryAt]
		number = entry.firstChunk + (sample-entry.samplesBefore)/entry.perChunk
		return number, entry.samplesBefore + (number-entry.firstChunk)*entry.perChunk, entry.perChunk
	}
	firstNumber, firstSample, _ := locate(kept.from)
	lastNumber, _, _ := locate(kept.to - 1)
	entryAt = 0
	if lastNumber > found.chunks {
		return nil, nil, fmt.Errorf("%w: a sample past the last chunk", ErrCannotTrim)
	}
	offsetBytes := make([]byte, (lastNumber-firstNumber+1)*found.offset.entryBytes)
	if readErr := found.readArray(file, found.offset, firstNumber-1, offsetBytes); readErr != nil {
		return nil, nil, readErr
	}
	offsetOf := func(number int64) int64 {
		at := (number - firstNumber) * found.offset.entryBytes
		if found.offset.entryBytes == 8 {
			return int64(binary.BigEndian.Uint64(offsetBytes[at:])) //nolint:gosec // checked against the file below
		}
		return int64(binary.BigEndian.Uint32(offsetBytes[at:]))
	}
	sizeOf, sizes, err := found.sizes(file, firstSample, kept.to)
	if err != nil {
		return nil, nil, err
	}
	var chunks []chunk
	for sample := kept.from; sample < kept.to; {
		number, chunkFirst, perChunk := locate(sample)
		offset := offsetOf(number)
		for before := chunkFirst; before < sample; before++ {
			offset += sizeOf(before)
		}
		count := min(chunkFirst+perChunk, kept.to) - sample
		if offset < 0 || offset > found.fileBytes() {
			return nil, nil, fmt.Errorf("%w: a chunk past the file's end", ErrCannotTrim)
		}
		chunks = append(chunks, chunk{offset: offset, samples: count})
		sample += count
	}
	return chunks, sizes[(kept.from-firstSample)*found.stsz.entryBytes:], nil
}

// sizes reads the sizes of samples from to to: a sample's size by its number, and their entries.
func (found *track) sizes(file io.ReaderAt, from, to int64) (func(int64) int64, []byte, error) {
	if found.stsz.entryBytes == 0 {
		constant := int64(binary.BigEndian.Uint32(found.stsz.header[4:]))
		return func(int64) int64 { return constant }, nil, nil
	}
	entries := make([]byte, (to-from)*4)
	if err := found.readArray(file, found.stsz, from, entries); err != nil {
		return nil, nil, err
	}
	return func(sample int64) int64 { return int64(binary.BigEndian.Uint32(entries[(sample-from)*4:])) }, entries, nil
}

// fileBytes is the size of the file the track's samples lie in.
func (found *track) fileBytes() int64 { return found.fileSize }
