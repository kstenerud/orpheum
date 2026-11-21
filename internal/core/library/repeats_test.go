// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// repeatsBench is the fingerprint detector over a harness's library, with one series whose seasons
// a test adds, the jobs its sweep queues and the work it gives.
type repeatsBench struct {
	*harness
	repeats  *Repeats
	jobs     *queuedJobs
	sent     *sentWork
	seriesID int64
	episodes map[int64]int64 // each episode's file, by its title
}

func newRepeatsBench(t *testing.T) *repeatsBench {
	t.Helper()
	bench := &repeatsBench{harness: newHarness(t), jobs: &queuedJobs{}, sent: &sentWork{}, episodes: map[int64]int64{}}
	bench.seriesID = librarytest.Item(t, bench.database,
		librarytest.ItemSpec{Kind: "series", LibraryID: bench.libraryID})
	storetest.ExecRaw(t, bench.database.Path(),
		fmt.Sprintf("INSERT INTO sequence (id, kind, created_by) VALUES (%d, 'series', 'scanner')", bench.seriesID),
		fmt.Sprintf("INSERT INTO ordering (id, sequence_id, name) VALUES (%d, %d, 'aired')", bench.seriesID,
			bench.seriesID))
	bench.repeats = NewRepeats(RepeatsParts{
		Database: bench.database, Jobs: bench.jobs, Assigner: bench.sent, Nodes: &fakeNodes{id: bench.nodeID},
		Clock: bench.clock, Logger: slog.New(slog.DiscardHandler),
	})
	return bench
}

// season adds a season of episodes, each one video of an hour with a sound track as its second
// stream, and returns their files in order.
func (bench *repeatsBench) season(number, episodes int) []int64 {
	bench.t.Helper()
	probe, err := json.Marshal(nodeproto.Probe{
		Format:  nodeproto.ProbeFormat{DurationMs: 3_600_000},
		Streams: []nodeproto.ProbeStream{{StreamIndex: 0, Kind: "video"}, {StreamIndex: 1, Kind: "audio"}},
	})
	if err != nil {
		bench.t.Fatal(err)
	}
	var files []int64
	for episode := 1; episode <= episodes; episode++ {
		title := librarytest.Title(bench.t, bench.database,
			librarytest.TitleSpec{Library: bench.libraryID, Kind: "episode"})
		version := librarytest.Version(bench.t, bench.database, librarytest.VersionSpec{TitleID: title})
		file := librarytest.File(bench.t, bench.database, librarytest.FileSpec{
			LibraryID: bench.libraryID, NodeID: bench.nodeID, VersionID: version,
			Path:        fmt.Sprintf("%s/series/%d-%d.mkv", bench.rootPath, number, episode),
			PayloadHash: fmt.Appendf(nil, "%d-%d", number, episode), SizeBytes: 100,
		})
		for index, kind := range []string{"video", "audio"} {
			librarytest.Stream(bench.t, bench.database,
				librarytest.StreamSpec{BytesFile: file, StreamIndex: int64(index), Kind: kind})
		}
		librarytest.Probe(bench.t, bench.database, librarytest.ProbeSpec{
			FileID: file, ProbeJSON: string(probe), DurationMs: 3_600_000,
		})
		storetest.ExecRaw(bench.t, bench.database.Path(), storetest.Positions(fmt.Sprintf("(%d, %d, %d, %d, %d)",
			bench.seriesID, title, number, episode, number*100+episode)))
		bench.episodes[title] = file
		files = append(files, file)
	}
	return files
}

// sweep runs the detector's sweep and returns the jobs it queued.
func (bench *repeatsBench) sweep() []jobs.Job {
	bench.t.Helper()
	before := len(bench.jobs.requests)
	if _, err := bench.repeats.QueueRepeats(bench.t.Context()); err != nil {
		bench.t.Fatal(err)
	}
	var queued []jobs.Job
	for _, request := range bench.jobs.requests[before:] {
		if request.Kind == jobs.KindFindRepeats {
			queued = append(queued, jobs.Job{Kind: request.Kind, Target: request.Target, ArgsJSON: request.ArgsJSON})
		}
	}
	return queued
}

// targets are the files the jobs are aimed at.
func targets(queued []jobs.Job) []int64 {
	var fileIDs []int64
	for _, job := range queued {
		fileIDs = append(fileIDs, job.FileID)
	}
	return fileIDs
}

// run runs a job and returns what it gave its node, or nil when it gave nothing.
func (bench *repeatsBench) run(job jobs.Job) *nodeproto.FindRepeats {
	bench.t.Helper()
	before := len(bench.sent.commands)
	if err := bench.repeats.HandleFindRepeats(bench.t.Context(), job); err != nil {
		bench.t.Fatal(err)
	}
	if len(bench.sent.commands) == before {
		return nil
	}
	return bench.sent.commands[len(bench.sent.commands)-1].FindRepeats
}

// videosOf are the files a comparison names.
func videosOf(command *nodeproto.FindRepeats) []int64 {
	var fileIDs []int64
	for _, video := range command.Videos {
		fileIDs = append(fileIDs, video.FileID)
	}
	return fileIDs
}

// answer is the node's answer to a job's comparison.
func (bench *repeatsBench) answer(job jobs.Job, sent *nodeproto.FindRepeats, videos ...nodeproto.VideoRepeats) {
	bench.t.Helper()
	result := nodeproto.Result{Repeats: &nodeproto.Repeats{Videos: videos}}
	attempt := nodes.Attempt{
		NodeID: bench.nodeID, Work: nodeproto.Work{Type: nodeproto.CommandFindRepeats, FindRepeats: sent},
	}
	if err := bench.repeats.Assignee(nil).Answer(bench.t.Context(), job, attempt, result); err != nil {
		bench.t.Fatal(err)
	}
}

// segments are a file's segments, as type provider start-end confidence.
func (bench *repeatsBench) segments(fileID int64) string {
	bench.t.Helper()
	return strings.Join(storetest.QueryRaw(bench.t, bench.database.Path(),
		"SELECT type || ' ' || provider || ' ' || start_ms || '-' || end_ms || ' ' || confidence FROM media_segment "+
			"WHERE file_id = "+itoa(fileID)+" ORDER BY provider, type, start_ms"), "; ")
}

// measured is how a file's bytes were measured, or "" when they were not.
func (bench *repeatsBench) measured(fileID int64) string {
	bench.t.Helper()
	return strings.Join(storetest.QueryRaw(bench.t, bench.database.Path(),
		"SELECT outcome || coalesce(' ' || failure, '') FROM repeat_measurement JOIN file "+
			"ON file.payload_hash = repeat_measurement.payload_hash WHERE file.id = "+itoa(fileID)), "; ")
}

// span is a repeat heard in some of the videos it was compared with.
func span(startMs, endMs int64, heardIn, comparedWith int) *nodeproto.RepeatedSpan {
	return &nodeproto.RepeatedSpan{StartMs: startMs, EndMs: endMs, HeardIn: heardIn, ComparedWith: comparedWith}
}

// The sweep queues one comparison for each season with a video not measured yet, aimed at its lowest
// such video, naming one video of each of its episodes with their sound track and length, and the
// opening and closing minutes the detector compares; a season whose videos were all measured is
// asked for nothing, and nor is a season of one episode, which has nothing to be compared with (D101).
func TestQueueRepeats_QueuesEachSeasonNotMeasuredOnce(t *testing.T) {
	t.Parallel()
	bench := newRepeatsBench(t)
	first, second := bench.season(1, 3), bench.season(2, 2)
	bench.season(3, 1)
	queued := bench.sweep()
	if got, want := targets(queued), []int64{first[0], second[0]}; !slices.Equal(got, want) {
		t.Fatalf("the sweep queued comparisons aimed at %v; want one for each season of two episodes or more, %v",
			got, want)
	}
	sent := bench.run(queued[1])
	if sent == nil {
		t.Fatal("the second season's comparison gave its node nothing")
	}
	if got, want := videosOf(sent), second; !slices.Equal(got, want) {
		t.Errorf("the second season's comparison named %v; want its videos %v", got, want)
	}
	if sent.OpeningMs != 600_000 || sent.ClosingMs != 360_000 || sent.Videos[0].AudioStreamIndex != 1 ||
		sent.Videos[0].DurationMs != 3_600_000 {
		t.Errorf("the comparison sent %+v; want ten minutes' opening, six's closing, each video's sound and length",
			*sent)
	}
	bench.answer(queued[1], sent, nodeproto.VideoRepeats{FileID: second[0]}, nodeproto.VideoRepeats{FileID: second[1]})
	if got, want := targets(bench.sweep()), []int64{first[0]}; !slices.Equal(got, want) {
		t.Errorf("after the second season was measured, the sweep queued %v; want the first's alone, %v", got, want)
	}
}

// What the node hears repeated in a video's opening becomes its intro and in its closing its credits,
// the fingerprint provider's, each as sure as the share of the videos it was compared with it is heard
// in; and each video's bytes are kept as measured, found or none (D101, D115).
func TestRepeats_SpansBecomeIntroAndCreditsWithTheirConfidence(t *testing.T) {
	t.Parallel()
	bench := newRepeatsBench(t)
	files := bench.season(1, 3)
	job := bench.sweep()[0]
	sent := bench.run(job)
	bench.answer(job, sent,
		nodeproto.VideoRepeats{
			FileID: files[0], Opening: span(30_000, 120_000, 2, 2), Closing: span(3_300_000, 3_390_000, 1, 2),
		},
		nodeproto.VideoRepeats{FileID: files[1], Opening: span(45_000, 135_000, 2, 2)},
		nodeproto.VideoRepeats{FileID: files[2]})
	for _, each := range []struct {
		fileID             int64
		segments, measured string
	}{
		{files[0], "credits fingerprint 3300000-3390000 0.5; intro fingerprint 30000-120000 1.0", "found"},
		{files[1], "intro fingerprint 45000-135000 1.0", "found"},
		{files[2], "", "none"},
	} {
		if got := bench.segments(each.fileID); got != each.segments {
			t.Errorf("file %d's segments are %q; want %q", each.fileID, got, each.segments)
		}
		if got := bench.measured(each.fileID); got != each.measured {
			t.Errorf("file %d was measured %q; want %q", each.fileID, got, each.measured)
		}
	}
}

// A comparison done again replaces only what the fingerprint provider found: a person's intro stays,
// for a person's correction wins (D115), and the fingerprint's earlier one is gone.
func TestRepeats_APersonsSegmentSurvivesAFingerprintAgain(t *testing.T) {
	t.Parallel()
	bench := newRepeatsBench(t)
	files := bench.season(1, 2)
	storetest.ExecRaw(t, bench.database.Path(), fmt.Sprintf("INSERT INTO media_segment (file_id, type, "+
		"start_ms, end_ms, provider, confidence) VALUES (%d, 'intro', 1000, 2000, 'person', 1)", files[0]))
	job := bench.sweep()[0]
	sent := bench.run(job)
	for _, startMs := range []int64{30_000, 60_000} {
		bench.answer(job, sent, nodeproto.VideoRepeats{FileID: files[0], Opening: span(startMs, startMs+90_000, 1, 1)},
			nodeproto.VideoRepeats{FileID: files[1], Opening: span(startMs, startMs+90_000, 1, 1)})
	}
	want := "intro fingerprint 60000-150000 1.0; intro person 1000-2000 1.0"
	if got := bench.segments(files[0]); got != want {
		t.Errorf("after two comparisons, file %d's segments are %q; want %q", files[0], got, want)
	}
}

// A video whose sound could not be fingerprinted is kept as measured with FFmpeg's words, and is not
// compared again, a fact about its bytes; one the node refused, changed or gone since the scan, is
// left unmeasured, and the next sweep asks of it again (D101).
func TestRepeats_AFailedVideoIsNotAskedAgainAndARefusedOneIs(t *testing.T) {
	t.Parallel()
	bench := newRepeatsBench(t)
	files := bench.season(1, 4)
	job := bench.sweep()[0]
	sent := bench.run(job)
	bench.answer(job, sent,
		nodeproto.VideoRepeats{FileID: files[0], Failure: "no audio frames"},
		nodeproto.VideoRepeats{FileID: files[1], Refusal: "changed since the scan"},
		nodeproto.VideoRepeats{FileID: files[2]}, nodeproto.VideoRepeats{FileID: files[3]})
	if got, want := bench.measured(files[0]), "failed no audio frames"; got != want {
		t.Errorf("the video that failed was measured %q; want %q", got, want)
	}
	if got := bench.measured(files[1]); got != "" {
		t.Errorf("the video refused was measured %q; want it left for the next sweep", got)
	}
	queued := bench.sweep()
	if got, want := targets(queued), []int64{files[1]}; !slices.Equal(got, want) {
		t.Fatalf("the next sweep queued %v; want the refused video's season again, aimed at it, %v", got, want)
	}
	if got, want := videosOf(bench.run(queued[0])), files[1:]; !slices.Equal(got, want) {
		t.Errorf("the next comparison named %v; want all but the video that failed, %v", got, want)
	}
}

// An answer for a video no longer in the season, or one never in it, is not kept: neither its
// segments nor its measurement; the season's others' are (D101).
func TestRepeats_AnswersForAVideoGoneFromTheSeasonAreIgnored(t *testing.T) {
	t.Parallel()
	bench := newRepeatsBench(t)
	files := bench.season(1, 3)
	job := bench.sweep()[0]
	sent := bench.run(job)
	var gone int64
	for title, file := range bench.episodes {
		if file == files[2] {
			gone = title
		}
	}
	storetest.ExecRaw(t, bench.database.Path(), fmt.Sprintf("DELETE FROM position WHERE work_id = %d", gone))
	bench.answer(job, sent,
		nodeproto.VideoRepeats{FileID: files[0], Opening: span(30_000, 120_000, 2, 2)},
		nodeproto.VideoRepeats{FileID: files[1], Opening: span(30_000, 120_000, 2, 2)},
		nodeproto.VideoRepeats{FileID: files[2], Opening: span(30_000, 120_000, 2, 2)},
		nodeproto.VideoRepeats{FileID: 999_999, Opening: span(30_000, 120_000, 2, 2)})
	if got, want := bench.segments(files[0]), "intro fingerprint 30000-120000 1.0"; got != want {
		t.Errorf("a video still in the season has segments %q; want %q", got, want)
	}
	if got, measured := bench.segments(files[2]), bench.measured(files[2]); got != "" || measured != "" {
		t.Errorf("the video gone from the season has segments %q and was measured %q; want neither kept",
			got, measured)
	}
}
