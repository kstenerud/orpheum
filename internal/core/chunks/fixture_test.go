// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// fakeQueue keeps what the chunks ask of the job queue.
type fakeQueue struct {
	mutex sync.Mutex
	// requests are the runs asked for, in order; guarded by mutex.
	requests []jobs.Request
	// isTaken says whether a request is queued, as the queue would: false is one folded into another
	// of the film's jobs already queued. Nil queues every one. Guarded by mutex.
	isTaken func(request jobs.Request) bool
	// progress is each job's share done, and keptWanted how often a job was kept wanted; guarded by
	// mutex.
	progress   map[int64]float64
	keptWanted int
	// cancelledFirsts are the first chunks of the runs cancelled, in order, and cancelledJobs the
	// jobs cancelled by id; guarded by mutex.
	cancelledFirsts []int
	cancelledJobs   []int64
	// released are the jobs said to be no longer wanted, in order; guarded by mutex.
	released []jobs.Target
}

func (queue *fakeQueue) ReleaseWanted(_ context.Context, _ jobs.Kind, target jobs.Target) error {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.released = append(queue.released, target)
	return nil
}

func (queue *fakeQueue) Cancel(_ context.Context, id int64) error {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.cancelledJobs = append(queue.cancelledJobs, id)
	return nil
}

func newFakeQueue() *fakeQueue { return &fakeQueue{progress: map[int64]float64{}} }

func (queue *fakeQueue) Enqueue(_ context.Context, request jobs.Request) (bool, error) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.requests = append(queue.requests, request)
	return queue.isTaken == nil || queue.isTaken(request), nil
}

func (queue *fakeQueue) SetProgress(_ context.Context, id int64, fraction float64) error {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.progress[id] = fraction
	return nil
}

func (queue *fakeQueue) KeepWanted(context.Context, jobs.Kind, jobs.Target) error {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.keptWanted++
	return nil
}

// CancelMatching records the first chunk of each queued run match says to cancel, as the queue
// would cancel it.
func (queue *fakeQueue) CancelMatching(
	_ context.Context, kind jobs.Kind, target jobs.Target, match func(argsJSON string) bool,
) (int, error) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	cancelled := 0
	for _, request := range queue.requests {
		if request.Kind != kind || request.Target != target || !match(request.ArgsJSON) {
			continue
		}
		var args runArgs
		if err := json.Unmarshal([]byte(request.ArgsJSON), &args); err != nil {
			return cancelled, err
		}
		queue.cancelledFirsts = append(queue.cancelledFirsts, args.FirstChunk)
		cancelled++
	}
	return cancelled, nil
}

// askedRun is a run asked for: the stream its store is of, and its chunks.
type askedRun struct {
	stream                string
	firstChunk, lastChunk int
}

// runs are the runs asked of the queue, each by its store's stream and its arguments.
func (queue *fakeQueue) runs(t *testing.T, service *Service) []askedRun {
	t.Helper()
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	asked := make([]askedRun, 0, len(queue.requests))
	for _, request := range queue.requests {
		var args runArgs
		if err := json.Unmarshal([]byte(request.ArgsJSON), &args); err != nil {
			t.Fatal(err)
		}
		row, err := service.cache.Read().GetChunkStore(t.Context(), request.ChunkStoreID)
		if err != nil {
			t.Fatalf("a run's target, chunk store %d: %v", request.ChunkStoreID, err)
		}
		asked = append(asked, askedRun{stream: row.Stream, firstChunk: args.FirstChunk, lastChunk: args.LastChunk})
	}
	return asked
}

// fakeNode is node 1, connected, which takes the commands it is sent and does nothing with them.
type fakeNode struct {
	sent chan nodeproto.Command
}

func (node fakeNode) Enqueue(_ int64, command nodeproto.Command) { node.sent <- command }

func (fakeNode) ParkUnlessConnected(context.Context, int64) error { return nil }

func (fakeNode) Withdraw(int64, string) bool { return false }

func (fakeNode) StartOf(int64) string { return "" }

func (node fakeNode) Send(_ context.Context, nodeID int64, command nodeproto.Command) error {
	node.Enqueue(nodeID, command)
	return nil
}

func (fakeNode) Unanswered(_ context.Context, _ int64, err error) error { return err }

func (fakeNode) Unpark(context.Context, jobs.Destination) {}

func (fakeNode) Resources(context.Context, int64) (nodes.Resources, bool, error) {
	return nodes.Resources{}, false, nil
}

func (fakeNode) NodeConnection(context.Context, int64) (nodes.Connection, time.Time) {
	return nodes.Connected, time.Time{}
}

// fakeLoudness levels film 1's first audio track, stream 1, as stereo by -3.5 dB.
type fakeLoudness struct{}

func (fakeLoudness) Levels(context.Context, []byte) (decide.Levels, error) {
	return decide.Levels{{StreamIndex: 1, Mix: decide.MixStereo}: {IsMeasured: true, GainDB: -3.5}}, nil
}

// fakeReadThrough counts the read-throughs asked for at play priority.
type fakeReadThrough struct {
	mutex sync.Mutex
	asked []int64
	// failedBeforePlay are the files whose read-through failed before a play, which it tries again.
	failedBeforePlay map[int64]bool
	retried          []int64
	// released are the files whose read-through a player let go of.
	released []int64
	// noIndex are the payload hashes whose files a read from the index found none in; readFirst, the
	// files read through ahead of the sweep for their keyframes.
	noIndex   [][]byte
	readFirst []int64
}

func (reader *fakeReadThrough) HasNoIndex(payloadHash []byte, _ int) bool {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	return slices.ContainsFunc(reader.noIndex, func(each []byte) bool { return bytes.Equal(each, payloadHash) })
}

func (reader *fakeReadThrough) ReadFirst(_ context.Context, fileID int64) (bool, error) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	reader.readFirst = append(reader.readFirst, fileID)
	return true, nil
}

func (reader *fakeReadThrough) RetryFailedForPlay(
	_ context.Context, fileID int64, _ []byte, _ int, _ time.Time,
) (bool, error) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	if !reader.failedBeforePlay[fileID] {
		return false, nil
	}
	delete(reader.failedBeforePlay, fileID)
	reader.retried = append(reader.retried, fileID)
	return true, nil
}

func (reader *fakeReadThrough) ReleaseForPlay(_ context.Context, fileID int64) error {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	reader.released = append(reader.released, fileID)
	return nil
}

func (reader *fakeReadThrough) ReadForPlay(_ context.Context, fileID int64) (bool, error) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	reader.asked = append(reader.asked, fileID)
	return true, nil
}

// filmMinutes is film 1's length: twenty minutes, a keyframe every ten seconds, so a chunk every ten
// seconds, chunks 0 to 119.
const filmMinutes = 20

// chunksFixture is a chunks service over two films on node 1, under /m, its runs queued in queue and
// given to node: film 1, whose 1920x1080 H.264 picture at four megabits a second is stream 0, after
// it a 5.1 English track and a stereo one with no language, its timeline kept, a keyframe every ten
// seconds and one busy second of 9.6 megabits; and film 2, the same with no timeline yet. The
// segment length is two seconds.
func chunksFixture(t *testing.T, queue Queue, node fakeNode) (*Service, *fakeReadThrough) {
	t.Helper()
	database := storetest.OpenMigrated(t)
	nodeID, libraryID := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/m"})
	durationMs := int64(filmMinutes * 60_000)
	probe := fmt.Sprintf(`{"format":{"durationMs":%d,"bitRateBps":4000000},"streams":[`+
		`{"kind":"video","codec":"h264","video":{"width":1920,"height":1080}},`+
		`{"streamIndex":1,"kind":"audio","codec":"ac3","language":"en","audio":{"channelCount":6}},`+
		`{"streamIndex":2,"kind":"audio","codec":"aac","audio":{"channelCount":2}}]}`, durationMs)
	for fileID := range 2 {
		storetest.ExecRaw(t, database.Path(), fmt.Sprintf("INSERT INTO file (id, library_id, node_id, path, "+
			"reported_path_raw, payload_hash, filing, first_seen_at, last_seen_at) "+
			"VALUES (%d, %d, %d, '/m/f%d.mkv', CAST('/m/f%d.mkv' AS BLOB), X'0%d', 'unidentified', 1, 1)",
			fileID+1, libraryID, nodeID, fileID+1, fileID+1, fileID+1),
			fmt.Sprintf("INSERT INTO file_probe (file_id, probe_json, duration_ms) VALUES (%d, '%s', %d)",
				fileID+1, probe, durationMs))
	}
	keyframes, seconds := []int64{}, []int64{}
	for at := int64(0); at < durationMs; at += 10_000 {
		keyframes = append(keyframes, at)
	}
	for range durationMs / 1000 {
		seconds = append(seconds, 500_000)
	}
	seconds[30] = 1_200_000
	keyframesJSON, err := json.Marshal(keyframes)
	if err != nil {
		t.Fatal(err)
	}
	secondsJSON, err := json.Marshal(seconds)
	if err != nil {
		t.Fatal(err)
	}
	err = database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.UpsertVideoTimeline(t.Context(), sqlcgen.UpsertVideoTimelineParams{
			PayloadHash: []byte{0x01}, KeyframesMsJSON: string(keyframesJSON),
			BytesInEachSecondJSON: string(secondsJSON),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	seeded := settingstest.Seeded(t, database)
	settingstest.Set(t, seeded, settings.HLSSegmentLengthSeconds, "2")
	clockSource := clocktest.NewFake(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC))
	reader := &fakeReadThrough{}
	cache := storetest.OpenCache(t)
	service := NewService(Parts{
		Database: database, Cache: cache, Settings: seeded, Queue: queue, Nodes: node, Loudness: fakeLoudness{},
		ReadThrough: reader, Logger: slog.New(slog.DiscardHandler), Clock: clockSource,
		Exchanges: nodes.NewExchanges(nodes.ExchangeParts{
			Sender: node, Rooms: node, Clock: clockSource, Random: rand.Reader, Logger: slog.New(slog.DiscardHandler),
			Cache: cache, Settings: seeded, MaxSilence: time.Hour,
		}),
	})
	return service, reader
}
