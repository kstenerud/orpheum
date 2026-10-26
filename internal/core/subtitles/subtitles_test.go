// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitles

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/nodes/nodestest"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakeNodeID is the fake node every bench's fixture holds the subtitle's file on.
const fakeNodeID = 1

// bareVideoFileID is the video file every bench's fixture makes, stream 1's.
const bareVideoFileID = 1

// errOffline is why the fake node refuses a command while a test holds it disconnected.
var errOffline = errors.New("the fake node is not connected")

// fakeQueue is the job queue as a list, which also answers KeepWanted (jobs.Queue's two methods
// the subtitles service uses), since jobstest.Queue alone does not (finding 10, a conformance
// fake narrower than its interface is worse than none).
type fakeQueue struct {
	mutex sync.Mutex
	// requests are the jobs asked for, in order; guarded by mutex.
	requests []jobs.Request
	// keptWanted counts, by kind and target, how often a play asked to keep a job wanted.
	keptWanted map[jobs.Target]int
}

func newFakeQueue() *fakeQueue { return &fakeQueue{keptWanted: map[jobs.Target]int{}} }

// Enqueue lists every job asked for, in order: the service never reads what Enqueue answers
// (subtitles.go's wantText and convertWhileWaiting both discard it), so the fake need not model
// the real queue's merge of a repeat request into the open job, only record the asking.
func (queue *fakeQueue) Enqueue(_ context.Context, job jobs.Request) (bool, error) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.requests = append(queue.requests, job)
	return true, nil
}

func (queue *fakeQueue) KeepWanted(_ context.Context, _ jobs.Kind, target jobs.Target) error {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.keptWanted[target]++
	return nil
}

func (queue *fakeQueue) requestCount(kind jobs.Kind, target jobs.Target) int {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	count := 0
	for _, request := range queue.requests {
		if request.Kind == kind && request.Target == target {
			count++
		}
	}
	return count
}

// fakeNode is the one node a bench's fixture holds the file on, connected unless a test says
// otherwise, which answers each extraction it is sent as its answer func says, once set.
type fakeNode struct {
	service     *Service
	ctx         context.Context //nolint:containedctx // the test's, for the answers it sends
	isConnected bool
	// answer is called for each ExtractSubtitle command the node is sent; nil answers nothing, so
	// the job hangs until the test delivers one itself.
	answer func(nodeproto.ExtractSubtitle) nodeproto.SubtitleText

	mutex sync.Mutex
	sent  []nodeproto.ExtractSubtitle
}

func (node *fakeNode) NodeConnection(context.Context, int64) (nodes.Connection, time.Time) {
	if node.isConnected {
		return nodes.Connected, time.Time{}
	}
	return nodes.AwaitingHello, time.Time{}
}

func (node *fakeNode) IsConnected(context.Context, int64) bool { return node.isConnected }

func (node *fakeNode) Send(ctx context.Context, nodeID int64, command nodeproto.Command) error {
	if err := node.ParkUnlessConnected(ctx, nodeID); err != nil {
		return err
	}
	node.Enqueue(nodeID, command)
	return nil
}

func (node *fakeNode) ParkUnlessConnected(_ context.Context, nodeID int64) error {
	if !node.isConnected {
		return jobs.ParkOnNode(nodeID, time.Time{}, errOffline)
	}
	return nil
}

func (*fakeNode) Withdraw(int64, string) bool { return false }

func (*fakeNode) StartOf(int64) string { return "" }

func (*fakeNode) Unanswered(_ context.Context, _ int64, err error) error { return err }

func (*fakeNode) Unpark(context.Context, jobs.Destination) {}

func (*fakeNode) Resources(context.Context, int64) (nodes.Resources, bool, error) {
	return nodes.Resources{}, false, nil
}

// Enqueue takes the command as the fake node reading its queue would: it records an extraction
// asked of it and, once the test has given an answer func, answers at once.
func (node *fakeNode) Enqueue(_ int64, command nodeproto.Command) {
	if command.Type != nodeproto.CommandExtractSubtitle {
		return
	}
	extract := *command.ExtractSubtitle
	node.mutex.Lock()
	node.sent = append(node.sent, extract)
	node.mutex.Unlock()
	if node.answer == nil {
		return
	}
	made := node.answer(extract)
	echoed := command.WithoutBytes()
	go func() {
		result := nodeproto.Result{
			JobID: command.JobID, AttemptToken: command.AttemptToken, Work: &echoed, Subtitle: &made,
		}
		// An answer not taken leaves nothing stored, which the test's own checks find.
		_ = node.service.exchanges.Deliver(node.ctx, fakeNodeID, result) //nolint:errcheck // as above
	}()
}

// bench is a subtitles service over one video, stream 1, its ASS track, on the fake node.
type bench struct {
	service  *Service
	database *store.Database
	node     *fakeNode
	queue    *fakeQueue
	clock    *clocktest.Fake
	resolver *nodestest.Resolver
	streamID int64
}

// newBench returns a bench whose node is connected unless told otherwise before a job runs.
func newBench(t *testing.T) *bench {
	t.Helper()
	database := storetest.OpenMigrated(t)
	nodeID, libraryID := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/m"})
	storetest.ExecRaw(t, database.Path(),
		"INSERT INTO file (id, library_id, node_id, path, reported_path_raw, payload_hash, filing, "+
			"first_seen_at, last_seen_at) VALUES (1, "+strconv.FormatInt(libraryID, 10)+", "+
			strconv.FormatInt(nodeID, 10)+", '/m/f1.mkv', CAST('/m/f1.mkv' AS BLOB), X'01', 'unidentified', 1, 1)",
		"INSERT INTO stream (id, bytes_file, video_file, stream_index, kind, codec, language, disposition_flags) "+
			"VALUES (1, 1, 1, 2, 'subtitle', 'ass', 'en', 0)")
	node := &fakeNode{ctx: t.Context(), isConnected: true}
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC))
	queue := newFakeQueue()
	cache := storetest.OpenCache(t)
	config := settingstest.Seeded(t, database)
	service := NewService(Parts{
		Database: database, Queue: queue, Nodes: node,
		Exchanges: nodes.NewExchanges(nodes.ExchangeParts{
			Sender: node, Rooms: node, Clock: fakeClock, Random: rand.Reader, Logger: slog.New(slog.DiscardHandler),
			Cache: cache, Settings: config, MaxSilence: time.Minute,
		}),
		Clock: fakeClock, MaxSilence: time.Minute,
	})
	resolver := nodestest.NewResolver()
	service.exchanges.Handle(jobs.KindExtractSubtitle, service.Assignee(resolver.Of(nil)))
	node.service = service
	return &bench{
		service: service, database: database, node: node, queue: queue, clock: fakeClock, resolver: resolver,
		streamID: 1,
	}
}

// extractJobID is the one extraction job every bench test runs.
const extractJobID = 1

// extractJob is the extraction job of the bench's one file: one job per file the tracks are in,
// a video's own from one read of it (D340).
func (bench *bench) extractJob() jobs.Job {
	return jobs.Job{
		ID: extractJobID, Kind: jobs.KindExtractSubtitle, Target: jobs.Target{FileID: bareVideoFileID},
		Priority: jobs.PriorityPlayNow,
	}
}

// tracksAnswer answers every track extract's command named alike: each with ass and vtt, or, when
// failure is set, each as its own failure, never the whole command's (D340).
func tracksAnswer(extract nodeproto.ExtractSubtitle, ass, vtt []byte, failure string) nodeproto.SubtitleText {
	tracks := make([]nodeproto.TrackText, len(extract.StreamIndexes))
	for index, streamIndex := range extract.StreamIndexes {
		tracks[index] = nodeproto.TrackText{StreamIndex: streamIndex, ASS: ass, VTT: vtt, Failure: failure}
	}
	return nodeproto.SubtitleText{Tracks: tracks}
}

// handle runs the bench's extraction job by its handler, as a runner would, and returns its
// answer: the handler's own, once the node has none to give (not connected, say), or the one the
// node's word resolves it with, once the fake node's answer func has made one.
func (bench *bench) handle(t *testing.T) error {
	t.Helper()
	return testutil.Receive(t, bench.resolver.Start(t, bench.service.HandleExtractSubtitle, bench.extractJob()),
		"the extraction job's answer")
}

// text is the bench's one track's text, as a playback of the video asks for it.
func (bench *bench) text(t *testing.T) Text {
	t.Helper()
	text, err := bench.service.TextOf(t.Context(), preferences.TrackRef{StreamID: bench.streamID}, bareVideoFileID,
		FormatASS)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

// extractTarget is the job target of the bench's one file's extraction (D340).
func (bench *bench) extractTarget() jobs.Target { return jobs.Target{FileID: bareVideoFileID} }

// A video's text subtitle track is queued for extraction once, when it is first played: not
// queued twice for the same payload hash, 425 (preparing) until the node answers, and served as
// both ASS and WebVTT once it has (D217 §6).
func TestSubtitles_ATextTrackIsQueuedOnceAndServedAsASSAndWebVTTOnceExtracted(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	if _, err := bench.service.Want(t.Context(), bareVideoFileID); err != nil {
		t.Fatal(err)
	}
	if count := bench.queue.requestCount(jobs.KindExtractSubtitle, bench.extractTarget()); count != 1 {
		t.Fatalf("the first Want queued the extraction %d times; want once", count)
	}
	if text := bench.text(t); text.Readiness != jobs.ReadinessPreparing {
		t.Fatalf("a track not yet extracted: %+v; want preparing", text)
	}
	bench.node.answer = func(extract nodeproto.ExtractSubtitle) nodeproto.SubtitleText {
		return tracksAnswer(extract, []byte("[Script Info]"), []byte("WEBVTT"), "")
	}
	if err := bench.handle(t); err != nil {
		t.Fatalf("the extraction job: %v", err)
	}
	if text := bench.text(t); text.Readiness != jobs.ReadinessComplete || string(text.Text) != "[Script Info]" {
		t.Errorf("the extracted track's ASS: %+v; want its text, complete", text)
	}
	vtt, err := bench.service.TextOf(t.Context(), preferences.TrackRef{StreamID: bench.streamID}, bareVideoFileID,
		FormatWebVTT)
	if err != nil || vtt.Readiness != jobs.ReadinessComplete || string(vtt.Text) != "WEBVTT" {
		t.Errorf("the extracted track's WebVTT: %+v, %v; want its text, complete", vtt, err)
	}
	if _, err := bench.service.Want(t.Context(), bareVideoFileID); err != nil {
		t.Fatal(err)
	}
	if count := bench.queue.requestCount(jobs.KindExtractSubtitle, bench.extractTarget()); count != 1 {
		t.Errorf("Want after extraction queued it again: %d times; want still once", count)
	}
}

// An extraction FFmpeg could not make is kept as the track's own failure, shown to a playback, and
// is tried again the next time the video is played, a play being the request (D231 §4).
func TestSubtitles_AFailedExtractionSaysSoAndTheNextWantTriesItAgain(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	if _, err := bench.service.Want(t.Context(), bareVideoFileID); err != nil {
		t.Fatal(err)
	}
	bench.node.answer = func(extract nodeproto.ExtractSubtitle) nodeproto.SubtitleText {
		return tracksAnswer(extract, nil, nil, "a text FFmpeg could not read")
	}
	// FFmpeg's failure is a fact about the source's bytes, not retried: the job itself ends
	// permanently, which is this answer, not an error of the test's own (Assignee).
	if err := bench.handle(t); err == nil || !jobqueue.IsPermanent(err) {
		t.Fatalf("the extraction job answered %v; want FFmpeg's failure kept, permanent", err)
	}
	if text := bench.text(t); text.Readiness != jobs.ReadinessFailed || text.Failure != "a text FFmpeg could not read" {
		t.Fatalf("the failed track: %+v; want FFmpeg's failure kept and shown", text)
	}
	if _, err := bench.service.Want(t.Context(), bareVideoFileID); err != nil {
		t.Fatal(err)
	}
	if count := bench.queue.requestCount(jobs.KindExtractSubtitle, bench.extractTarget()); count != 2 {
		t.Errorf("the next Want queued it %d times; want a second attempt", count)
	}
}

// A node's refusal says nothing of the source's bytes, so it is not kept as the track's failure,
// and the job is tried again (Assignee).
func TestSubtitles_ARefusalIsNotKeptAndTheJobIsTriedAgain(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	bench.node.answer = func(nodeproto.ExtractSubtitle) nodeproto.SubtitleText {
		return nodeproto.SubtitleText{Refusal: "its file changed since the scan"}
	}
	err := bench.handle(t)
	if err == nil {
		t.Fatal("a refused extraction answered nil; want the refusal's error")
	}
	if jobqueue.IsPermanent(err) {
		t.Errorf("a refusal was kept as the track's own failure: %v; want it tried again, not permanent", err)
	}
	if text := bench.text(t); text.Readiness != jobs.ReadinessPreparing {
		t.Errorf("after a refusal the track: %+v; want still preparing, no failure kept", text)
	}
}

// A player still waiting for a text being made keeps its extraction's priority of play, as one
// waiting for a rendition does (D234 §3).
func TestSubtitles_APlayerStillWaitingKeepsItsExtractionWanted(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	if text := bench.text(t); text.Readiness != jobs.ReadinessPreparing {
		t.Fatalf("a track not yet extracted: %+v; want preparing", text)
	}
	if text := bench.text(t); text.Readiness != jobs.ReadinessPreparing {
		t.Fatalf("a track not yet extracted: %+v; want preparing", text)
	}
	if count := bench.queue.keptWanted[bench.extractTarget()]; count != 2 {
		t.Errorf("a player asking twice kept it wanted %d times; want twice", count)
	}
}

// A subtitle's text is kept in persistent.db, which outlives cache.db (the architecture map's F1,
// D231 §3): it is read straight from the database the extraction wrote, not through any cache.
func TestSubtitles_ExtractedTextIsKeptInPersistentDB(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	bench.node.answer = func(extract nodeproto.ExtractSubtitle) nodeproto.SubtitleText {
		return tracksAnswer(extract, []byte("kept"), []byte("kept"), "")
	}
	if err := bench.handle(t); err != nil {
		t.Fatalf("the extraction job: %v", err)
	}
	streamIndex := int64(2) // the fixture stream's own stream_index
	row, err := bench.database.Read().GetSubtitleText(t.Context(), textKey([]byte{0x01}, &streamIndex))
	if err != nil {
		t.Fatalf("read persistent.db directly: %v", err)
	}
	if string(row.ASS) != "kept" || string(row.VTT) != "kept" {
		t.Errorf("persistent.db holds %+v; want the extracted text", row)
	}
}

// An extraction, or a fetched subtitle's conversion, that is given up on is kept as the track's own
// failure, shown to a playback, and the next play of its video asks again (D231 §4, §10).
func TestSubtitles_RecordFailureKeepsAGivenUpExtractionAsItsTracksFailure(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	job := bench.extractJob()
	if err := bench.service.RecordFailure(t.Context(), job, "FFmpeg stalled twice"); err != nil {
		t.Fatal(err)
	}
	text := bench.text(t)
	if text.Readiness != jobs.ReadinessFailed || text.Failure != "given up: FFmpeg stalled twice" {
		t.Errorf("the given-up extraction: %+v; want its failure kept, prefixed \"given up: \"", text)
	}
}
