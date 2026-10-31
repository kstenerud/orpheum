// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// takeListingReadingAgain is takeListing for a scan that reads readAgain again.
func (world *harness) takeListingReadingAgain(readAgain ReadAgain, records []nodeproto.ScanRecord) TakenListing {
	world.t.Helper()
	world.clock.Advance(time.Minute)
	all := append([]nodeproto.ScanRecord{{Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{
		RootPath: world.rootPath, FilesystemDevice: 7, IsMountPoint: true,
	}}}, records...)
	taken, err := world.ingester.TakeListingReadingAgain(world.t.Context(), ListingScan{
		NodeID: world.nodeID, Root: world.rootPath, Scope: wholeRoot, ReadAgain: readAgain,
	}, all, testRules)
	if err != nil {
		world.t.Fatal(err)
	}
	return taken
}

// A scan that reads files again reads each it names, changed on disk or not, and every NFO and
// subtitle file it lists; what it does not name, stored as listed, is seen and not read; one that
// reads everything again reads every file (D392 §2, §4).
func TestTakeListing_AScanReadingAgainReadsWhatItNamesAndEveryNFOAndSubtitle(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	subtitle := sidecarRecord("Arkon/Arkon.en.srt", nodeproto.SidecarSubtitle, "1\n")
	subtitle.Sidecar.Identity = &nodeproto.Identity{
		Scheme: "payload-v1", Anchor: "whole-file", PayloadHash: hashOf("1\n"), PayloadBytes: 2,
	}
	world.scan([]nodeproto.ScanRecord{
		observedRecord("Arkon/Arkon.mkv", "arkon"), observedRecord("Metropolis.mkv", "heat"),
		sidecarRecord("Arkon/Arkon.nfo", nodeproto.SidecarNFO, "<movie/>"), subtitle,
	}, true)
	listing := []nodeproto.ScanRecord{
		listedFile("Arkon/Arkon.en.srt", 2, 1),
		listedFile("Arkon/Arkon.mkv", int64(len("arkon")), 1),
		listedFile("Arkon/Arkon.nfo", int64(len("<movie/>")), 1),
		listedFile("Metropolis.mkv", int64(len("heat")), 1),
	}
	if got := examined(world.takeListingReadingAgain(nil, listing)); len(got) != 0 {
		t.Errorf("a scan reading nothing again reads %v; want nothing, every file as stored", got)
	}
	one := examined(world.takeListingReadingAgain(ReadAgain{"Arkon/Arkon.mkv"}, listing))
	if want := []string{"subtitle Arkon/Arkon.en.srt", "video Arkon/Arkon.mkv", "nfo Arkon/Arkon.nfo"}; !slices.Equal(
		one, want) {
		t.Errorf("a scan reading one film again reads %v; want %v", one, want)
	}
	everything := examined(world.takeListingReadingAgain(readEverythingAgain(), listing))
	if want := []string{
		"subtitle Arkon/Arkon.en.srt", "video Arkon/Arkon.mkv", "nfo Arkon/Arkon.nfo", "video Metropolis.mkv",
	}; !slices.Equal(everything, want) {
		t.Errorf("a scan reading everything again reads %v; want %v", everything, want)
	}
}

// What a scan reads again is kept with it: an ask that joins a scan not yet with its node adds what it
// reads again, and one asked for while a scan runs reads it again in the scan that follows (D392 §2,
// §6).
func TestScans_WhatAScanReadsAgainSurvivesAJoinAndTheScanAfter(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan([]nodeproto.ScanRecord{observedRecord("Metropolis.mkv", "heat"), observedRecord("Nosferatu.mkv", "ronin")}, true)
	ctx := t.Context()
	listing := []nodeproto.ScanRecord{
		listedFile("Metropolis.mkv", int64(len("heat")), 1), listedFile("Nosferatu.mkv", int64(len("ronin")), 1),
	}
	bench.requestUnder(wholeRoot)
	err := bench.scans.RequestReadingAgain(ctx, bench.nodeID, root, wholeRoot, ReadAgain{"Metropolis.mkv"},
		jobs.PriorityPersonsRequest)
	if err != nil {
		t.Fatal(err)
	}
	bench.hand()
	err = bench.scans.RequestReadingAgain(ctx, bench.nodeID, root, wholeRoot, ReadAgain{"Nosferatu.mkv"},
		jobs.PriorityPersonsRequest)
	if err != nil {
		t.Fatal(err)
	}
	bench.list(listing...)
	if _, files := bench.examine(); len(files) != 1 || files[0].Path.Text != "Metropolis.mkv" {
		t.Fatalf("the scan joined reads %+v; want Metropolis.mkv, which the ask that joined it reads again", files)
	}
	bench.examineAs(observedRecord("Metropolis.mkv", "heat"))
	bench.list(listing...)
	if _, files := bench.examine(); len(files) != 1 || files[0].Path.Text != "Nosferatu.mkv" {
		t.Errorf("the scan after reads %+v; want Nosferatu.mkv, asked for while the first ran", files)
	}
}

// probedRecord is observedRecord with its probe.
func probedRecord(path, content string, probe nodeproto.Probe) nodeproto.ScanRecord {
	record := observedRecord(path, content)
	record.Observed.Probe = probe
	return record
}

// filmProbe is a film's probe: an HEVC picture, a stereo AAC and a 5.1 AC-3 sound, and a SubRip
// subtitle track, as kept by the scan that first read it.
func filmProbe() nodeproto.Probe {
	return nodeproto.Probe{
		Format: nodeproto.ProbeFormat{FormatNames: []string{"mov", "mp4"}, DurationMs: 600_000},
		Streams: []nodeproto.ProbeStream{
			{StreamIndex: 0, Kind: "video", Codec: "hevc", Video: nodeproto.ProbeVideo{
				Width: 1920, Height: 1080, FrameRate: "24/1",
			}},
			{StreamIndex: 1, Kind: "audio", Codec: "aac", Audio: nodeproto.ProbeAudio{ChannelCount: 2}},
			{StreamIndex: 2, Kind: "audio", Codec: "ac3", Audio: nodeproto.ProbeAudio{ChannelCount: 6}},
			{StreamIndex: 3, Kind: "subtitle", Codec: "subrip", Language: "en"},
		},
	}
}

// droppedLog is what fake droppers were asked to throw out, as "<thing> <stream index>".
type droppedLog struct{ dropped []string }

func (log *droppedLog) inWrite(name string) DropInWrite {
	return func(_ context.Context, _ *sqlcgen.Queries, of DerivedOf) (bool, error) {
		log.dropped = append(log.dropped, name+" "+strconv.Itoa(of.StreamIndex))
		return true, nil
	}
}

func (log *droppedLog) afterWrite(name string) DropAfterWrite {
	return func(_ context.Context, of DerivedOf) (bool, error) {
		log.dropped = append(log.dropped, name+" "+strconv.Itoa(of.StreamIndex)+" track "+strconv.Itoa(of.AudioTrack))
		return true, nil
	}
}

func (log *droppedLog) droppers() DerivedDroppers {
	return DerivedDroppers{
		Loudness: log.inWrite("loudness"), Timeline: log.inWrite("timeline"), SubtitleText: log.inWrite("text"),
		FrameStills: log.inWrite("stills"), PictureChunks: log.afterWrite("picture"),
		SoundChunks: log.afterWrite("sound"),
	}
}

// fullScanBench is a scan bench whose ingest logs to its buffer and throws out through fake droppers.
func fullScanBench(t *testing.T) (*scanBench, *droppedLog, *bytes.Buffer) {
	t.Helper()
	bench := newScanBench(t)
	var logged bytes.Buffer
	bench.ingester = NewIngester(bench.database, bench.clock, bench.settings, slog.New(slog.NewJSONHandler(&logged,
		&slog.HandlerOptions{Level: slog.LevelDebug})))
	dropped := &droppedLog{}
	bench.ingester.SetDerivedDroppers(dropped.droppers())
	bench.scans = bench.restart()
	return bench, dropped, &logged
}

// readAllAgain runs a full scan of the bench's root, whose node reads film.mp4 with probe.
func (bench *scanBench) readAllAgain(probe nodeproto.Probe) {
	bench.t.Helper()
	err := bench.scans.RequestReadingAgain(bench.t.Context(), bench.nodeID, root, wholeRoot, readEverythingAgain(),
		jobs.PriorityPersonsRequest)
	if err != nil {
		bench.t.Fatal(err)
	}
	bench.list(listedFile("film.mp4", int64(len("film")), 1))
	bench.examineAs(probedRecord("film.mp4", "film", probe))
}

// A file read again whose probe differs from the one kept throws out only what the changed facts
// touch, each thing by its own facts: a picture newly marked for re-timing its picture's chunks, not
// its timeline or stills; a sound whose channels changed its chunks and its loudness, not the other
// sound's; and logs the file, what differed and what was thrown out, with the scan's totals (D392 §3,
// §5; file 5667).
func TestScans_AFileReadAgainThrowsOutOnlyWhatItsChangedFactsTouch(t *testing.T) {
	t.Parallel()
	bench, dropped, logged := fullScanBench(t)
	bench.scan([]nodeproto.ScanRecord{probedRecord("film.mp4", "film", filmProbe())}, true)
	again := filmProbe()
	again.Streams[0].Video.NeedsRetiming = true
	again.Streams[2].Audio.ChannelCount, again.Streams[2].Audio.ChannelLayout = 8, "7.1"
	logged.Reset()
	bench.readAllAgain(again)
	want := []string{"loudness 2", "picture 0 track 0", "sound 2 track 1"}
	slices.Sort(dropped.dropped)
	if !slices.Equal(dropped.dropped, want) {
		t.Errorf("thrown out %v; want %v: the picture's chunks and the changed sound's chunks and loudness alone",
			dropped.dropped, want)
	}
	lines := logged.String()
	for _, said := range []string{
		`"msg":"a file read again differs from what was kept of it; what that touched is thrown out"`,
		`stream 0.video.needsRetiming: none → true`, `stream 2.audio.channelCount: 6 → 8`,
		`"thrownOut":["a sound's loudness (stream 2)","the picture's chunks","a sound's chunks (stream 2)"]`,
		`"msg":"a scan compared the files it read again with what was kept"`, `"comparedCount":1`,
		`"correctedCount":1`, `"thrownOutCount":3`,
	} {
		if !strings.Contains(lines, said) {
			t.Errorf("the log does not say %s:\n%s", said, lines)
		}
	}
	kept, err := bench.database.Read().GetFileProbeJSON(t.Context(), bench.liveFilesByPath()[root+"/film.mp4"].ID)
	if err != nil || !strings.Contains(kept, `"needsRetiming":true`) {
		t.Errorf("the kept probe = %s, %v; want the one read again", kept, err)
	}
}

// A second full scan of a library nothing changed in throws nothing out and changes nothing kept of
// its files, and says so in its totals (D392 §5).
func TestScans_ASecondFullScanOfAnUnchangedLibraryChangesNothing(t *testing.T) {
	t.Parallel()
	bench, dropped, logged := fullScanBench(t)
	bench.scan([]nodeproto.ScanRecord{probedRecord("film.mp4", "film", filmProbe())}, true)
	bench.readAllAgain(filmProbe())
	fileID := bench.liveFilesByPath()[root+"/film.mp4"].ID
	keptBefore, streamsBefore := bench.keptOf(fileID)
	logged.Reset()
	bench.readAllAgain(filmProbe())
	keptAfter, streamsAfter := bench.keptOf(fileID)
	if len(dropped.dropped) != 0 || keptAfter != keptBefore || !reflect.DeepEqual(streamsAfter, streamsBefore) {
		t.Errorf("an unchanged file read again threw out %v and kept %s %v; want nothing thrown out and %s %v",
			dropped.dropped, keptAfter, streamsAfter, keptBefore, streamsBefore)
	}
	lines := logged.String()
	if strings.Contains(lines, "differs from what was kept") || !strings.Contains(lines, `"comparedCount":1`) ||
		!strings.Contains(lines, `"correctedCount":0`) || !strings.Contains(lines, `"thrownOutCount":0`) {
		t.Errorf("the second full scan logged:\n%s\nwant its totals, one compared and none corrected", lines)
	}
}

// keptOf is a file's kept probe and its stream rows.
func (bench *scanBench) keptOf(fileID int64) (string, []sqlcgen.ListStreamsOfBytesFileRow) {
	bench.t.Helper()
	probe, err := bench.database.Read().GetFileProbeJSON(bench.t.Context(), fileID)
	if err != nil {
		bench.t.Fatal(err)
	}
	streams, err := bench.database.Read().ListStreamsOfBytesFile(bench.t.Context(), fileID)
	if err != nil {
		bench.t.Fatal(err)
	}
	return probe, streams
}

// A stream's facts key changes with the facts its chunks are made from alone: the picture's with its
// picture's and its container's, a sound's with its own; a fact no chunk is made from, a track's
// language, changes none; and a fact added at its zero value changes none (D392 §3).
func TestFactsKey_ChangesWithTheFactsAStreamsChunksAreMadeFromAlone(t *testing.T) {
	t.Parallel()
	before := filmProbe()
	picture, first, second := PictureChunksFactsKey(before), SoundChunksFactsKey(before, 0),
		SoundChunksFactsKey(before, 1)
	if len(picture) != factsKeyHexLength || picture == first || first == second {
		t.Fatalf("the keys %q %q %q; want three of 16 hex characters, each its own", picture, first, second)
	}
	retimed := filmProbe()
	retimed.Streams[0].Video.NeedsRetiming = true
	rechanneled := filmProbe()
	rechanneled.Streams[2].Audio.ChannelCount = 8
	relabelled := filmProbe()
	relabelled.Streams[1].Language = "de"
	for _, test := range []struct {
		name                    string
		probe                   nodeproto.Probe
		isPicture, isFirst, is2 bool
	}{
		{"a picture marked for re-timing", retimed, true, false, false},
		{"the second sound's channels", rechanneled, false, false, true},
		{"the first sound's language", relabelled, false, false, false},
		{"the probe as it was", filmProbe(), false, false, false},
	} {
		changed := []bool{
			PictureChunksFactsKey(test.probe) != picture, SoundChunksFactsKey(test.probe, 0) != first,
			SoundChunksFactsKey(test.probe, 1) != second,
		}
		if want := []bool{test.isPicture, test.isFirst, test.is2}; !slices.Equal(changed, want) {
			t.Errorf("%s changed the picture's, the first and the second sound's keys %v; want %v", test.name,
				changed, want)
		}
	}
}

// Every thing derived from a probe declares the facts it is made from and one way to throw it out,
// each of the droppers throws out one thing, and the ingest says which it was left without (D392 §3).
func TestDerivedFromProbe_EachThingDeclaresItsFactsAndOneWayToThrowItOut(t *testing.T) {
	t.Parallel()
	things := derivedFromProbe()
	for _, thing := range things {
		if thing.name == "" || len(thing.streamFacts) == 0 || (thing.inWrite == nil) == (thing.afterWrite == nil) {
			t.Errorf("%q declares stream facts %v and in-write %t, after-write %t; want facts and one way",
				thing.name, thing.streamFacts, thing.inWrite != nil, thing.afterWrite != nil)
		}
	}
	every := (&droppedLog{}).droppers()
	ingester := NewIngester(nil, nil, nil, slog.New(slog.DiscardHandler))
	if unwired := ingester.UnwiredDroppers(); len(unwired) != len(things) {
		t.Errorf("an ingest given no droppers is without %v; want every thing", unwired)
	}
	fields := reflect.ValueOf(every)
	for index := range fields.NumField() {
		only := reflect.New(fields.Type()).Elem()
		only.Field(index).Set(fields.Field(index))
		droppers, isDroppers := reflect.TypeAssert[DerivedDroppers](only)
		if !isDroppers {
			t.Fatalf("%v is not the droppers", only.Type())
		}
		ingester.SetDerivedDroppers(droppers)
		if unwired := ingester.UnwiredDroppers(); len(unwired) != len(things)-1 {
			t.Errorf("given %s alone, the ingest is without %v; want every thing but one",
				fields.Type().Field(index).Name, unwired)
		}
	}
	ingester.SetDerivedDroppers(every)
	if unwired := ingester.UnwiredDroppers(); len(unwired) != 0 {
		t.Errorf("an ingest given every dropper is without %v; want none", unwired)
	}
}
