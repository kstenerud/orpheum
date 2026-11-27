// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// fakeSpeedTrials say when they are started and each run of chunks' start and end.
type fakeSpeedTrials struct {
	started chan struct{}
	runs    chan string
	going   atomic.Int64
	results []nodeproto.SpeedTrial
}

func newFakeSpeedTrials() *fakeSpeedTrials {
	return &fakeSpeedTrials{started: make(chan struct{}, 4), runs: make(chan string, 8)}
}

func (fake *fakeSpeedTrials) Start(context.Context) { fake.started <- struct{}{} }

func (fake *fakeSpeedTrials) ChunksRunStarts() func() {
	fake.going.Add(1)
	fake.runs <- "started"
	return func() {
		fake.going.Add(-1)
		fake.runs <- "ended"
	}
}

func (fake *fakeSpeedTrials) Results() []nodeproto.SpeedTrial { return fake.results }

// The speed trials start once the node has said hello, and each run of chunks tells them it started
// and, once over, that it ended, so no trial goes beside it.
func TestAgent_StartsTheSpeedTrialsAfterHelloAndTellsThemOfEachRunOfChunks(t *testing.T) {
	t.Parallel()
	trials := newFakeSpeedTrials()
	core, socket := startFakeCore(t)
	root, _ := startAgentWithWorkers(t, socket, Workers{
		Scanner: fakeScanner{}, Artworker: fakeArtworker{}, Subtitler: fakeSubtitler{},
		ThroughReader: fakeThroughReader{}, ChunkMaker: fakeChunkMaker{}, SpeedTrials: trials,
	})
	select {
	case <-trials.started:
	case <-t.Context().Done():
		t.Fatal("the speed trials were not started after hello")
	}
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	video := nodeproto.BytesFile{
		RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: nodetest.PayloadHashHex(t, "a film"),
	}
	core.sendMakeChunks(1, nodeproto.MakeChunks{
		Video: video, StoreDir: "1/chunks-picture/0123456789abcdef", FirstChunk: 4,
		BoundsMs: []int64{8000, 10000, 12000}, Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
	})
	receiveChunksReport(t, core)
	var said []string
	for len(said) < 2 {
		select {
		case event := <-trials.runs:
			said = append(said, event)
		case <-t.Context().Done():
			t.Fatalf("the trials were told %v of the run; want started, then ended", said)
		}
	}
	if !slices.Equal(said, []string{"started", "ended"}) || trials.going.Load() != 0 {
		t.Errorf("the trials were told %v of the run; want started, then ended", said)
	}
}

// The status carries the speed trials' results, and none on a node without trials.
func TestStatus_CarriesTheSpeedTrials(t *testing.T) {
	t.Parallel()
	trials := newFakeSpeedTrials()
	trials.results = []nodeproto.SpeedTrial{{
		Codec: "h264", BitDepth: 8, Height: 1080, MadeKbps: 8000,
		MadeHeight: 1080, SpeedPercent: 240, DecodedOn: "cpu",
	}}
	now := clocktest.NewFake(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC))
	node := New("/nowhere", nodeproto.Hello{Name: "local"}, nil, Workers{SpeedTrials: trials}, now,
		slog.New(slog.DiscardHandler))
	if got := node.status(t.Context()).SpeedTrials; !slices.Equal(got, trials.results) {
		t.Errorf("the status's speed trials are %+v; want %+v", got, trials.results)
	}
	bare := New("/nowhere", nodeproto.Hello{Name: "local"}, nil, Workers{}, now, slog.New(slog.DiscardHandler))
	if got := bare.status(t.Context()).SpeedTrials; got != nil {
		t.Errorf("a node without trials says %+v; want none", got)
	}
}
