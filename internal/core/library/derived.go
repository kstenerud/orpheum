// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// What is kept that was made from a file's probe, and from which of its facts (D392 §3). A file read
// again whose probe differs from the one kept throws out each thing whose facts differ, and nothing
// else: derivedFromProbe is the one place that says what each is made from, so a new derived thing
// declares its facts there, and the server's wiring how it is thrown out (DerivedDroppers).

// derivedScope is which part of a file a derived thing is of: its main picture, each of its audio
// tracks, each of its own subtitle tracks.
type derivedScope int

const (
	ofMainPicture derivedScope = iota
	ofEachSound
	ofEachSubtitle
)

// The names of the derived things, as the log says them.
const (
	derivedPictureChunks = "the picture's chunks"
	derivedSoundChunks   = "a sound's chunks"
	derivedLoudness      = "a sound's loudness"
	derivedTimeline      = "the picture's timeline"
	derivedSubtitleText  = "a subtitle track's text"
	derivedFrameStills   = "the frame stills"
)

// The facts of a stream most derived things are made from, by their names in the probe.
const (
	factStreamIndex  = "streamIndex"
	factCodec        = "codec"
	factCodecProfile = "codecProfile"
)

// derivedThing is one kind of thing kept that was made from a file's probe: what it is of, the facts
// of the probe it was made from, and how it is thrown out, in the ingest's write (inWrite) or after it
// (afterWrite), one of the two.
type derivedThing struct {
	name string
	of   derivedScope
	// formatFacts are the facts of the probe's container it was made from, by their names in the
	// probe ("formatNames"); streamFacts those of the stream it is of, a name ending in "." covering
	// every fact below it ("video.").
	formatFacts, streamFacts []string
	inWrite                  func(DerivedDroppers) DropInWrite
	afterWrite               func(DerivedDroppers) DropAfterWrite
}

// derivedFromProbe is everything kept that was made from a file's probe, each with the facts it was
// made from (D392 §3). A store of chunks names the facts it was made from in its directory too
// (chunks.StoreKey), so one made before they changed is never served again, whoever reads the file
// again. The stream rows are the probe's own, re-derived on every read (syncStreams), and what a person
// set or watched is no probe's (D357 §3).
func derivedFromProbe() []derivedThing {
	return []derivedThing{
		{
			name: derivedPictureChunks, of: ofMainPicture, formatFacts: []string{"formatNames"},
			streamFacts: []string{factStreamIndex, factCodec, factCodecProfile, "video."},
			afterWrite:  func(droppers DerivedDroppers) DropAfterWrite { return droppers.PictureChunks },
		},
		{
			name: derivedSoundChunks, of: ofEachSound, formatFacts: []string{"formatNames"},
			streamFacts: []string{factStreamIndex, factCodec, factCodecProfile, "audio."},
			afterWrite:  func(droppers DerivedDroppers) DropAfterWrite { return droppers.SoundChunks },
		},
		{
			name: derivedLoudness, of: ofEachSound,
			streamFacts: []string{factStreamIndex, factCodec, factCodecProfile, "audio."},
			inWrite:     func(droppers DerivedDroppers) DropInWrite { return droppers.Loudness },
		},
		{
			name: derivedTimeline, of: ofMainPicture, streamFacts: []string{factStreamIndex, factCodec},
			inWrite: func(droppers DerivedDroppers) DropInWrite { return droppers.Timeline },
		},
		{
			name: derivedSubtitleText, of: ofEachSubtitle, streamFacts: []string{factStreamIndex, factCodec},
			inWrite: func(droppers DerivedDroppers) DropInWrite { return droppers.SubtitleText },
		},
		{
			name: derivedFrameStills, of: ofMainPicture, formatFacts: []string{"durationMs"},
			streamFacts: []string{factStreamIndex, "video.width", "video.height"},
			inWrite:     func(droppers DerivedDroppers) DropInWrite { return droppers.FrameStills },
		},
	}
}

// DerivedOf is the one thing of a file a drop throws out: the file, its bytes, and the stream it is
// of, as the kept probe numbers it, a sound's by its place among the file's audio tracks too.
type DerivedOf struct {
	FileID      int64
	PayloadHash []byte
	StreamIndex int
	AudioTrack  int
}

// DropInWrite throws out a thing persistent.db keeps, in the ingest's write that keeps the new probe,
// and says whether there was any to throw out.
type DropInWrite func(ctx context.Context, queries *sqlcgen.Queries, of DerivedOf) (bool, error)

// DropAfterWrite throws out what cache.db or memory keeps, once the ingest's write is committed, and
// says whether there was any.
type DropAfterWrite func(ctx context.Context, of DerivedOf) (bool, error)

// DerivedDroppers are how each thing derived from a probe is thrown out, each by the package that
// keeps it: set by the server's wiring (SetDerivedDroppers). One left nil throws out nothing.
type DerivedDroppers struct {
	Loudness      DropInWrite    // readthrough.ForgetLoudness
	Timeline      DropInWrite    // readthrough.ForgetTimeline
	SubtitleText  DropInWrite    // subtitles.ForgetText
	FrameStills   DropInWrite    // artwork.Service.RetireFrameStills
	PictureChunks DropAfterWrite // chunks.Service.ForgetPictureStores
	SoundChunks   DropAfterWrite // chunks.Service.ForgetSoundStores
}

// SetDerivedDroppers sets how what is derived from a probe is thrown out when the probe changes.
func (ingester *Ingester) SetDerivedDroppers(droppers DerivedDroppers) { ingester.droppers = droppers }

// UnwiredDroppers are the things derived from a probe that the droppers set leave nothing to throw
// out with, by name: none once the server's wiring is whole, so a thing added to derivedFromProbe is
// wired too.
func (ingester *Ingester) UnwiredDroppers() []string {
	var unwired []string
	for _, thing := range derivedFromProbe() {
		if (thing.inWrite != nil && thing.inWrite(ingester.droppers) == nil) ||
			(thing.afterWrite != nil && thing.afterWrite(ingester.droppers) == nil) {
			unwired = append(unwired, thing.name)
		}
	}
	return unwired
}

// probeFacts are a probe's facts by their names: "format.durationMs", "stream 1.video.width". A list
// is one fact, its JSON; a fact at its zero value is none, so a probe kept before a fact was added
// has the same facts as one read since.
func probeFacts(probe nodeproto.Probe) (map[string]string, error) {
	facts := map[string]string{}
	if err := addFacts(facts, "format.", probe.Format); err != nil {
		return nil, err
	}
	for _, stream := range probe.Streams {
		if err := addFacts(facts, "stream "+strconv.Itoa(stream.StreamIndex)+".", stream); err != nil {
			return nil, err
		}
	}
	if err := addFacts(facts, "", map[string]any{"chapters": probe.Chapters}); err != nil {
		return nil, err
	}
	return facts, nil
}

// factsOf are the facts of value, a part of a probe, by their names within it.
func factsOf(value any) (map[string]string, error) {
	facts := map[string]string{}
	return facts, addFacts(facts, "", value)
}

// addFacts adds the facts of value, a part of a probe, to facts, each name after prefix.
func addFacts(facts map[string]string, prefix string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode a probe's facts: %w", err)
	}
	var decoded any
	if err := strictjson.Unmarshal(encoded, &decoded); err != nil {
		return fmt.Errorf("read a probe's facts: %w", err)
	}
	return flattenFacts(facts, prefix, decoded)
}

// flattenFacts names each fact of a decoded JSON value, each as its JSON: an object's by its fields,
// anything else as itself, and none at its zero value.
func flattenFacts(facts map[string]string, name string, value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for field, inner := range typed {
			if err := flattenFacts(facts, name+field+".", inner); err != nil {
				return err
			}
		}
		return nil
	case nil:
		return nil
	case string, bool, float64, []any:
		if isZeroFact(typed) {
			return nil
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode a probe's fact %s: %w", name, err)
	}
	facts[strings.TrimSuffix(name, ".")] = string(encoded)
	return nil
}

// isZeroFact is whether a decoded JSON value is its kind's zero: no fact.
func isZeroFact(value any) bool {
	switch typed := value.(type) {
	case string:
		return typed == ""
	case bool:
		return !typed
	case float64:
		return typed == 0
	case []any:
		return len(typed) == 0
	}
	return false
}

// factChange is one fact a probe read again gives otherwise than the one kept, each value as its
// JSON, "" for none.
type factChange struct {
	Fact          string
	Before, After string
}

// String is the change as the log says it.
func (change factChange) String() string {
	return change.Fact + ": " + cmp.Or(change.Before, "none") + " → " + cmp.Or(change.After, "none")
}

// changedFacts are the facts two probes give otherwise, by name.
func changedFacts(before, after map[string]string) []factChange {
	names := slices.Sorted(maps.Keys(before))
	for name := range after {
		if _, isBefore := before[name]; !isBefore {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	var changes []factChange
	for _, name := range names {
		if before[name] != after[name] {
			changes = append(changes, factChange{Fact: name, Before: before[name], After: after[name]})
		}
	}
	return changes
}

// derivedPart is one part of a file a derived thing is of, in the probe kept and in the one read
// again: the facts of its stream there, by the names the table gives them ("codec"), none where it
// has no such stream; and of the probes' containers.
type derivedPart struct {
	of                        DerivedOf
	beforeFacts, afterFacts   map[string]string
	beforeFormat, afterFormat map[string]string
}

// streamPair is a stream of the kept probe and the one read again pairs with it, nil for none.
type streamPair struct{ before, after *nodeproto.ProbeStream }

// pairsOf pairs the kept probe's streams of scope with those of the one read again: its main picture;
// each audio track, by its place among them; each subtitle track, by its stream index. Only a stream
// the kept probe has can have anything derived from it.
func pairsOf(scope derivedScope, before, after nodeproto.Probe) []streamPair {
	var pairs []streamPair
	switch scope {
	case ofMainPicture:
		if picture, has := decide.MainVideo(before); has {
			pair := streamPair{before: &picture}
			if again, hasAgain := decide.MainVideo(after); hasAgain {
				pair.after = &again
			}
			pairs = append(pairs, pair)
		}
	case ofEachSound:
		tracks, again := decide.AudioStreams(before), decide.AudioStreams(after)
		for place := range tracks {
			pair := streamPair{before: &tracks[place]}
			if place < len(again) {
				pair.after = &again[place]
			}
			pairs = append(pairs, pair)
		}
	case ofEachSubtitle:
		for index := range before.Streams {
			if kept := &before.Streams[index]; kept.Kind == streamKindSubtitle {
				pairs = append(pairs, streamPair{before: kept, after: streamAt(after, kept.StreamIndex)})
			}
		}
	}
	return pairs
}

// streamAt is a probe's stream of the index, nil for none.
func streamAt(probe nodeproto.Probe, streamIndex int) *nodeproto.ProbeStream {
	at := slices.IndexFunc(probe.Streams, func(stream nodeproto.ProbeStream) bool {
		return stream.StreamIndex == streamIndex
	})
	if at < 0 {
		return nil
	}
	return &probe.Streams[at]
}

// partsOf are the parts of a file a derived thing of scope is of (pairsOf), each with its facts in
// both probes.
func partsOf(scope derivedScope, file DerivedOf, before, after nodeproto.Probe) ([]derivedPart, error) {
	beforeFormat, err := factsOf(before.Format)
	if err != nil {
		return nil, err
	}
	afterFormat, err := factsOf(after.Format)
	if err != nil {
		return nil, err
	}
	pairs := pairsOf(scope, before, after)
	parts := make([]derivedPart, 0, len(pairs))
	for place, pair := range pairs {
		part := derivedPart{
			of: file, afterFacts: map[string]string{}, beforeFormat: beforeFormat, afterFormat: afterFormat,
		}
		part.of.StreamIndex = pair.before.StreamIndex
		if scope == ofEachSound {
			part.of.AudioTrack = place
		}
		if part.beforeFacts, err = factsOf(*pair.before); err != nil {
			return nil, err
		}
		if pair.after != nil {
			if part.afterFacts, err = factsOf(*pair.after); err != nil {
				return nil, err
			}
		}
		parts = append(parts, part)
	}
	return parts, nil
}

// streamKindSubtitle is a subtitle stream's kind in a probe.
const streamKindSubtitle = "subtitle"

// isMadeFrom is whether a fact, by its name in its stream or format, is one of facts: named, or below
// one ending in ".".
func isMadeFrom(fact string, facts []string) bool {
	return slices.ContainsFunc(facts, func(each string) bool {
		return fact == each || (strings.HasSuffix(each, ".") && strings.HasPrefix(fact, each))
	})
}

// isTouched is whether a part's facts the thing was made from differ between the probes.
func (thing derivedThing) isTouched(part derivedPart) bool {
	return slices.ContainsFunc(changedFacts(part.beforeFormat, part.afterFormat), func(change factChange) bool {
		return isMadeFrom(change.Fact, thing.formatFacts)
	}) || slices.ContainsFunc(changedFacts(part.beforeFacts, part.afterFacts), func(change factChange) bool {
		return isMadeFrom(change.Fact, thing.streamFacts)
	})
}

// factsKey names the facts of a probe that the derived thing called name, of the main picture or of
// the audio track at place track, was made from: the first 16 hex characters of the SHA-256 of each
// such fact, by name, so a store of chunks made from other facts is another store (chunks.StoreKey). A
// probe with no such part names none, "".
func factsKey(name string, probe nodeproto.Probe, track int) string {
	things := derivedFromProbe()
	thing := things[slices.IndexFunc(things, func(each derivedThing) bool { return each.name == name })]
	parts, err := partsOf(thing.of, DerivedOf{}, probe, nodeproto.Probe{})
	if err != nil || track >= len(parts) {
		return ""
	}
	part := parts[track]
	var written []string
	for _, fact := range slices.Sorted(maps.Keys(part.beforeFormat)) {
		if isMadeFrom(fact, thing.formatFacts) {
			written = append(written, "format."+fact+"="+part.beforeFormat[fact])
		}
	}
	for _, fact := range slices.Sorted(maps.Keys(part.beforeFacts)) {
		if isMadeFrom(fact, thing.streamFacts) {
			written = append(written, fact+"="+part.beforeFacts[fact])
		}
	}
	hash := sha256.Sum256([]byte(strings.Join(written, "\n")))
	return hex.EncodeToString(hash[:])[:factsKeyHexLength]
}

// factsKeyHexLength is how many hex characters of the hash of a thing's facts name them.
const factsKeyHexLength = 16

// PictureChunksFactsKey names the facts of a probe its main picture's chunks are made from, copied or
// encoded: its container's and its picture's (D392 §3).
func PictureChunksFactsKey(probe nodeproto.Probe) string {
	return factsKey(derivedPictureChunks, probe, 0)
}

// SoundChunksFactsKey names the facts of a probe an audio track's chunks are made from, the track by
// its place among the file's audio tracks: its container's and the track's (D392 §3).
func SoundChunksFactsKey(probe nodeproto.Probe, track int) string {
	return factsKey(derivedSoundChunks, probe, track)
}

// probeCorrection is what a probe read again changed of a file: the facts it gives otherwise, and the
// derived things they touched, each thrown out.
type probeCorrection struct {
	fileID  int64
	changes []factChange
	// dropped are the things thrown out in the write; toDropAfter, after it.
	dropped     []string
	toDropAfter []pendingDrop
}

// pendingDrop is a thing to throw out once the ingest's write is committed.
type pendingDrop struct {
	name string
	of   DerivedOf
	drop DropAfterWrite
}

// correctProbe compares a file's probe read again with the one kept, and throws out, in the write,
// each derived thing whose facts differ, saying what it threw out and what it leaves to throw out
// after the write. Probes alike change nothing.
func (batch *ingestBatch) correctProbe(
	ctx context.Context, file DerivedOf, keptJSON string, again nodeproto.Probe,
) (probeCorrection, error) {
	correction := probeCorrection{fileID: file.FileID}
	kept, err := DecodeProbe(keptJSON)
	if err != nil {
		return correction, err
	}
	keptFacts, err := probeFacts(kept)
	if err != nil {
		return correction, err
	}
	againFacts, err := probeFacts(again)
	if err != nil {
		return correction, err
	}
	correction.changes = changedFacts(keptFacts, againFacts)
	if len(correction.changes) == 0 {
		return correction, nil
	}
	for _, thing := range derivedFromProbe() {
		parts, err := partsOf(thing.of, file, kept, again)
		if err != nil {
			return correction, err
		}
		for _, part := range parts {
			if !thing.isTouched(part) {
				continue
			}
			if err := batch.drop(ctx, &correction, thing, part.of); err != nil {
				return correction, err
			}
		}
	}
	return correction, nil
}

// drop throws out a derived thing of one part of a file whose facts changed: in the write, noted in
// the correction as thrown out when there was any; or, kept outside persistent.db, left in it to throw
// out after the write.
func (batch *ingestBatch) drop(
	ctx context.Context, correction *probeCorrection, thing derivedThing, part DerivedOf,
) error {
	droppers := batch.ingester.droppers
	if thing.afterWrite != nil {
		if drop := thing.afterWrite(droppers); drop != nil {
			correction.toDropAfter = append(correction.toDropAfter,
				pendingDrop{name: partName(thing, part), of: part, drop: drop})
		}
		return nil
	}
	drop := thing.inWrite(droppers)
	if drop == nil {
		return nil
	}
	isDropped, err := drop(ctx, batch.queries, part)
	if err != nil {
		return fmt.Errorf("throw out %s of file %d: %w", thing.name, part.FileID, err)
	}
	if isDropped {
		correction.dropped = append(correction.dropped, partName(thing, part))
	}
	return nil
}

// partName is a derived thing of one part of a file, as the log says it.
func partName(thing derivedThing, of DerivedOf) string {
	switch thing.of {
	case ofEachSound, ofEachSubtitle:
		return thing.name + " (stream " + strconv.Itoa(of.StreamIndex) + ")"
	case ofMainPicture:
	}
	return thing.name
}

// finishCorrections throws out, once the ingest's write that kept them is committed, what the
// corrected probes touched that cache.db and memory keep, and logs each correction: the file, the
// facts that differed and what was thrown out because of them (D392 §5). A drop that fails is
// logged; the store's name holds its facts, so what it left is never served (chunks.StoreKey).
func (ingester *Ingester) finishCorrections(ctx context.Context, key scanKey, corrections []probeCorrection) {
	for _, correction := range corrections {
		dropped := slices.Clone(correction.dropped)
		for _, pending := range correction.toDropAfter {
			isDropped, err := pending.drop(ctx, pending.of)
			if err != nil {
				ingester.logger.ErrorContext(ctx, "could not throw out what a changed probe touched", "fileId",
					correction.fileID, "derived", pending.name, "err", err)
				continue
			}
			if isDropped {
				dropped = append(dropped, pending.name)
			}
		}
		if scan := ingester.scans[key]; scan != nil {
			scan.corrections.droppedCount += len(dropped) - len(correction.dropped)
		}
		differed := make([]string, 0, len(correction.changes))
		for _, change := range correction.changes {
			differed = append(differed, change.String())
		}
		ingester.logger.InfoContext(ctx, "a file read again differs from what was kept of it; what that touched "+
			"is thrown out",
			slogx.EventKey, "readAgainDiffers",
			"fileId", correction.fileID, "differed", differed, "thrownOut", dropped)
	}
}

// logCorrectionTotals logs a scan's totals of the probes it read again, at its end: every full
// rescan's, and any other's that corrected one (D392 §5).
func (ingester *Ingester) logCorrectionTotals(ctx context.Context, nodeID int64, scan *scanState) {
	totals := scan.corrections
	if !scan.isReadingAgain && totals.correctedCount == 0 {
		return
	}
	ingester.logger.InfoContext(ctx, "a scan compared the files it read again with what was kept",
		slogx.EventKey, "readAgainCompared",
		"nodeId", nodeID, "rootPath", scan.root, "scopePath", scan.scope, "isReadingAgain", scan.isReadingAgain,
		"comparedCount", totals.comparedCount, "correctedCount", totals.correctedCount,
		"thrownOutCount", totals.droppedCount, "chunksCheckedCount", totals.chunksCheckedCount,
		"chunksDroppedCount", totals.chunksDroppedCount)
}

// noteChunksChecked counts, in its scan's totals, what the node found checking a file's stores of
// chunks against their records, and logs, at warn, each file whose chunks it dropped: the stores, the
// chunks and what differed (D392 §4, §5). A chunk dropped is made again when a player asks for it, as
// one never made, and nothing the core keeps changes.
func (ingester *Ingester) noteChunksChecked(ctx context.Context, scan *scanState, checked *nodeproto.ChunksChecked) {
	if checked == nil {
		return
	}
	dropped, droppedCount := make([]string, 0, len(checked.Dropped)), 0
	for _, each := range checked.Dropped {
		droppedCount += each.FileCount
		name := each.Name
		if name == "" {
			name = strconv.Itoa(each.FileCount) + " files, the whole store"
		}
		dropped = append(dropped, each.StoreDir+"/"+name+": "+each.Why)
	}
	if scan != nil {
		scan.corrections.chunksCheckedCount += checked.CheckedCount
		scan.corrections.chunksDroppedCount += droppedCount
	}
	if droppedCount > 0 {
		ingester.logger.WarnContext(ctx, "chunks of a file read again were missing or damaged; they are dropped and "+
			"made again when asked for",
			slogx.EventKey, "readAgainChunksDropped",
			"fileId", checked.FileID, "checkedCount", checked.CheckedCount,
			"droppedCount", droppedCount, "dropped", dropped)
	}
}
