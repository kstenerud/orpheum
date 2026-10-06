// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakeChunkMaker makes every chunk of the run in turn, telling each, and ends; a run of a store
// named "...failing..." fails after its first chunk, as FFmpeg does on bytes it cannot read, and
// one named "...panicking..." panics, as the node's own code reading FFmpeg's output might.
type fakeChunkMaker struct{}

func (fakeChunkMaker) Make(
	_ context.Context, _ string, command nodeproto.MakeChunks, onChunk func(n int),
) (int, error) {
	if strings.Contains(command.StoreDir, "panicking") {
		panic("a box shorter than its header")
	}
	last := command.FirstChunk + len(command.BoundsMs) - 2
	if strings.Contains(command.StoreDir, "failing") {
		last = command.FirstChunk
	}
	for n := command.FirstChunk; n <= last; n++ {
		onChunk(n)
	}
	if strings.Contains(command.StoreDir, "failing") {
		return last, errors.New("ffmpeg: Invalid data found when processing input")
	}
	return last, nil
}

// sendMakeChunks sends the core's command to make chunks as job.
func (core *fakeCore) sendMakeChunks(job int64, command nodeproto.MakeChunks) {
	core.commands <- nodeproto.Command{
		CommandSeq: job, JobID: job,
		Work: nodeproto.Work{Type: nodeproto.CommandMakeChunks, MakeChunks: &command},
	}
}

// receiveChunksReport is the next run's report the node posts.
func receiveChunksReport(t *testing.T, core *fakeCore) nodeproto.MakeChunksReport {
	t.Helper()
	select {
	case result := <-core.chunkRuns:
		return *result.MakeChunks
	case <-t.Context().Done():
	}
	t.Fatal("no run of chunks reached the core")
	return nodeproto.MakeChunksReport{}
}

// The node makes chunks only of the video the core named, tells the core each chunk as it is
// made, and says how far a run got however it ended (D247 §10).
func TestAgent_MakesChunksOfOnlyTheVideoTheCoreNamedAndSaysEachAsItIsMade(t *testing.T) {
	t.Parallel()
	video, _, core := startExtractionAgent(t)
	core.sendMakeChunks(1, nodeproto.MakeChunks{
		Video: video, StoreDir: "1/chunks-picture/0123456789abcdef", FirstChunk: 4,
		BoundsMs: []int64{8000, 10000, 12000}, Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
	})
	var told []int
	for len(told) < 2 {
		select {
		case progress := <-core.progress:
			if progress.ChunksMadeThrough != nil {
				told = append(told, *progress.ChunksMadeThrough)
			}
		case <-t.Context().Done():
			t.Fatalf("the core was told chunks %v; want 4 and 5", told)
		}
	}
	if told[0] != 4 || told[1] != 5 {
		t.Errorf("the core was told chunks %v; want 4 then 5, each as it is made", told)
	}
	if done := receiveChunksReport(t, core); done.State != nodeproto.PrepareDone || done.LastChunk != 5 {
		t.Errorf("the run ended %+v; want done through chunk 5", done)
	}
	failing := nodeproto.MakeChunks{
		Video: video, StoreDir: "1/chunks-failing/0123456789abcdef", FirstChunk: 0,
		BoundsMs: []int64{0, 2000, 4000}, Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
	}
	core.sendMakeChunks(2, failing)
	if failed := receiveChunksReport(t, core); failed.State != nodeproto.PrepareFailed || failed.LastChunk != 0 ||
		!strings.Contains(failed.Failure, "Invalid data") {
		t.Errorf("a run FFmpeg failed ended %+v; want failed through chunk 0, in FFmpeg's words", failed)
	}
	changed := failing
	changed.Video.PayloadHash = strings.Repeat("0", 64)
	core.sendMakeChunks(3, changed)
	if refused := receiveChunksReport(t, core); refused.State != nodeproto.PrepareRefused ||
		refused.RefusalCode != nodeproto.RefusedChanged || refused.LastChunk != -1 {
		t.Errorf("a run of a changed video ended %+v; want refused untried (D100)", refused)
	}
}

// A job whose work panics in the node's own code ends alone and says why to the core; the node goes
// on, and makes the next run (D248 §2).
func TestAgent_AJobThatPanicsSaysSoAndTheNodeGoesOn(t *testing.T) {
	t.Parallel()
	video, _, core := startExtractionAgent(t)
	core.sendMakeChunks(1, nodeproto.MakeChunks{
		Video: video, StoreDir: "1/chunks-panicking/0123456789abcdef", FirstChunk: 0,
		BoundsMs: []int64{0, 2000}, Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
	})
	select {
	case crashed := <-core.crashes:
		if crashed.JobID != 1 || !strings.Contains(crashed.Crashed, "a box shorter than its header") {
			t.Errorf("the node said %+v; want job 1's crash, with the panic", crashed)
		}
	case <-t.Context().Done():
		t.Fatal("the node said nothing of the job that panicked")
	}
	core.sendMakeChunks(2, nodeproto.MakeChunks{
		Video: video, StoreDir: "1/chunks-picture/0123456789abcdef", FirstChunk: 0,
		BoundsMs: []int64{0, 2000}, Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
	})
	if done := receiveChunksReport(t, core); done.State != nodeproto.PrepareDone {
		t.Errorf("the next run ended %+v; want it made, the node going on", done)
	}
}
