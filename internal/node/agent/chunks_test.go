// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/namedfile"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakeChunkMaker makes every chunk of the run in turn, telling each, and ends; a run of a store
// named "...failing..." fails after its first chunk, as FFmpeg does on bytes it cannot read, and
// one named "...panicking..." panics, as the node's own code reading FFmpeg's output might. One named
// "...slow..." makes its first chunk and runs until it is stopped, one named "...full..." finds the
// disk with no room, and one named "...stalled..." is stopped by the watchdog.
type fakeChunkMaker struct{}

func (fakeChunkMaker) Make(
	ctx context.Context, _ execx.Film, command nodeproto.MakeChunks, onChunk func(n int),
) (int, encode.Decoding, error) {
	switch {
	case strings.Contains(command.StoreDir, "panicking"):
		panic("a box shorter than its header")
	case strings.Contains(command.StoreDir, "slow"):
		onChunk(command.FirstChunk)
		<-ctx.Done()
		return command.FirstChunk, "", ctx.Err()
	case strings.Contains(command.StoreDir, "full"):
		return -1, "", fmt.Errorf("make %s: %w", command.StoreDir, prepared.ErrNoRoom)
	case strings.Contains(command.StoreDir, "stalled"):
		return -1, "", fmt.Errorf("make %s: %w", command.StoreDir, errStalled)
	}
	last := command.FirstChunk + len(command.BoundsMs) - 2
	if strings.Contains(command.StoreDir, "failing") {
		last = command.FirstChunk
	}
	for n := command.FirstChunk; n <= last; n++ {
		onChunk(n)
	}
	if strings.Contains(command.StoreDir, "failing") {
		return last, "", errors.New("ffmpeg: Invalid data found when processing input")
	}
	return last, "", nil
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
	if done := receiveChunksReport(t, core); done.State != nodeproto.RunDone || done.LastChunk != 5 {
		t.Errorf("the run ended %+v; want done through chunk 5", done)
	}
	failing := nodeproto.MakeChunks{
		Video: video, StoreDir: "1/chunks-failing/0123456789abcdef", FirstChunk: 0,
		BoundsMs: []int64{0, 2000, 4000}, Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
	}
	core.sendMakeChunks(2, failing)
	if failed := receiveChunksReport(t, core); failed.State != nodeproto.RunFailed || failed.LastChunk != 0 ||
		!strings.Contains(failed.Failure, "Invalid data") {
		t.Errorf("a run FFmpeg failed ended %+v; want failed through chunk 0, in FFmpeg's words", failed)
	}
	changed := failing
	changed.Video.PayloadHash = strings.Repeat("0", 64)
	core.sendMakeChunks(3, changed)
	if refused := receiveChunksReport(t, core); refused.State != nodeproto.RunRefused ||
		refused.RefusalCode != nodeproto.RefusedChanged || refused.LastChunk != -1 {
		t.Errorf("a run of a changed video ended %+v; want refused untried (D100)", refused)
	}
}

// A run of a video renamed away from its path is refused untried and named gone, so the core reads
// another place of it (D363 §8).
func TestAgent_ARunOfAVideoGoneFromItsPathIsRefusedAsGone(t *testing.T) {
	t.Parallel()
	video, _, core := startExtractionAgent(t)
	video.Path = []byte("renamed-away.mkv")
	core.sendMakeChunks(1, nodeproto.MakeChunks{
		Video: video, StoreDir: "1/chunks-picture/0123456789abcdef", FirstChunk: 0,
		BoundsMs: []int64{0, 2000}, Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
	})
	if refused := receiveChunksReport(t, core); refused.State != nodeproto.RunRefused ||
		refused.RefusalCode != nodeproto.RefusedGone || refused.LastChunk != -1 {
		t.Errorf("a run of a video gone from its path ended %+v; want refused untried, gone", refused)
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
	if done := receiveChunksReport(t, core); done.State != nodeproto.RunDone {
		t.Errorf("the next run ended %+v; want it made, the node going on", done)
	}
}

// pacedChunkMaker makes every chunk of the run in turn, each taking chunkTakes of its clock, and
// ends; a run of a store named "...failing..." fails after its first chunk, one named
// "...waiting..." waits after its first chunk until it is stopped, and one named "...gpu..." says its
// picture was decoded on the GPU (decodedOnOf).
type pacedChunkMaker struct {
	clock      *clocktest.Fake
	chunkTakes time.Duration
}

func (maker pacedChunkMaker) Make(
	ctx context.Context, _ execx.Film, command nodeproto.MakeChunks, onChunk func(n int),
) (int, encode.Decoding, error) {
	last := command.FirstChunk + len(command.BoundsMs) - 2
	for chunk := command.FirstChunk; chunk <= last; chunk++ {
		maker.clock.Advance(maker.chunkTakes)
		onChunk(chunk)
		switch {
		case strings.Contains(command.StoreDir, "failing"):
			return chunk, "", errors.New("ffmpeg: Invalid data found when processing input")
		case strings.Contains(command.StoreDir, "waiting"):
			<-ctx.Done()
			return chunk, "", ctx.Err()
		}
	}
	return last, decodedOnOf(command), nil
}

// decodedOnOf is where a paced run says its picture was decoded: on the GPU for a store named
// "...gpu...", and nowhere, as a run decoding no picture says, otherwise.
func decodedOnOf(command nodeproto.MakeChunks) encode.Decoding {
	if strings.Contains(command.StoreDir, "gpu") {
		return encode.DecodedOnGPU
	}
	return ""
}

// checkLogLine fails the test unless line holds every one of parts.
func checkLogLine(t *testing.T, line string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(line, part) {
			t.Errorf("logged %s; want it to hold %s", line, part)
		}
	}
}

// A run of chunks, however it ends, is logged with the chunks it made, the film they span, the wall
// time it took, its speed, the time to open its film and to its first chunk: whether a run keeps
// ahead of a player who seeks into it (playback-in-practice.md §3), how much of its start is
// FFmpeg's (D512), and, for a quality, where its picture was decoded.
func TestAgent_ARunOfChunksIsLoggedWithTheFilmItMadeAndHowFast(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	clock := clocktest.NewFake(time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC))
	logs := &testutil.Log{}
	root, _ := startAgentWithWorkers(t, socket, Workers{
		Scanner: fakeScanner{}, Artworker: fakeArtworker{}, Subtitler: fakeSubtitler{},
		ThroughReader: fakeThroughReader{},
		ChunkMaker:    pacedChunkMaker{clock: clock, chunkTakes: 600 * time.Millisecond},
	}, func(node *Agent) {
		node.clock = clock
		node.logger = slog.New(slog.NewJSONHandler(logs, nil))
	})
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	video := nodeproto.BytesFile{
		RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: nodetest.PayloadHashHex(t, "a film"),
	}
	run := func(storeDir string) nodeproto.MakeChunks {
		return nodeproto.MakeChunks{
			Video: video, StoreDir: storeDir, FirstChunk: 4, BoundsMs: []int64{8000, 10000, 12000, 14000},
			Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
		}
	}
	// Three chunks of two seconds each in 1.8 s: 3.3 times real time.
	core.sendMakeChunks(1, run("1/chunks-picture/0123456789abcdef"))
	receiveChunksReport(t, core)
	checkLogLine(t, logs.AwaitEvent(t, "chunkRunFinished"), `"level":"INFO"`, `"jobId":1,`,
		`"storeDir":"1/chunks-picture/0123456789abcdef"`, `"firstChunk":4,`, `"lastChunk":6,`, `"chunksMade":3,`,
		`"filmMadeMs":6000,`, `"wallTimeMs":1800,`, `"speed":3.3,`, `"filmOpenedAfterMs":0,`,
		`"firstChunkAfterMs":600`)
	if line := logs.AwaitEvent(t, "chunkRunFinished"); strings.Contains(line, "decodedOn") {
		t.Errorf("a run decoding no picture logged %s; want no decodedOn", line)
	}
	core.sendMakeChunks(2, run("1/chunks-failing/0123456789abcdef"))
	receiveChunksReport(t, core)
	checkLogLine(t, logs.AwaitEvent(t, "chunkRunFailed"), `"level":"WARN"`,
		`"jobId":2,`, `"lastChunk":4,`, `"why":"ffmpeg: Invalid data`, `"chunksMade":1,`, `"filmMadeMs":2000,`,
		`"wallTimeMs":600,`, `"speed":3.3,`, `"firstChunkAfterMs":600`)
	core.sendMakeChunks(3, run("1/chunks-waiting/0123456789abcdef"))
	for {
		if progress := testutil.Receive(t, core.progress, "job 3's first chunk"); progress.JobID == 3 &&
			progress.ChunksMadeThrough != nil {
			break
		}
	}
	core.commands <- nodeproto.Command{CommandSeq: 4, JobID: 3, Work: nodeproto.Work{Type: nodeproto.CommandCancel}}
	checkLogLine(t, logs.AwaitEvent(t, "chunkRunCancelled"), `"jobId":3,`,
		`"storeDir":"1/chunks-waiting/0123456789abcdef"`, `"lastChunk":4,`, `"chunksMade":1,`,
		`"filmMadeMs":2000,`, `"wallTimeMs":600,`, `"firstChunkAfterMs":600`)
	// A quality says where its picture was decoded.
	core.sendMakeChunks(5, run("1/chunks-gpu/0123456789abcdef"))
	receiveChunksReport(t, core)
	logs.AwaitEvent(t, "chunkRunFinished", `"jobId":5,`, `"decodedOn":"gpu"`)
}

// A run of a quality tells the core with its report how fast it made its film, how much it made,
// where its picture was decoded and whether another run went beside it at any time while it did; a
// run of another stream carries none of it (start-quality-by-node-speed.md §3).
func TestAgent_ARunOfAQualityReportsItsSpeedAndWhetherAnotherRunWentBesideIt(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	clock := clocktest.NewFake(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC))
	root, _ := startAgentWithWorkers(t, socket, Workers{
		ChunkMaker: pacedChunkMaker{clock: clock, chunkTakes: 600 * time.Millisecond},
	}, func(node *Agent) { node.clock = clock })
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	video := nodeproto.BytesFile{
		RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: nodetest.PayloadHashHex(t, "a film"),
	}
	run := func(storeDir string, kind nodeproto.ChunkStreamKind) nodeproto.MakeChunks {
		return nodeproto.MakeChunks{
			Video: video, StoreDir: storeDir, FirstChunk: 4, BoundsMs: []int64{8000, 10000, 12000, 14000},
			Stream: nodeproto.ChunkStream{Kind: kind, BitRateKbps: 3000, Height: 720},
		}
	}
	// Three chunks of two seconds each in 1.8 s: 333 % of real time.
	core.sendMakeChunks(1, run("1/chunks-q3000k720p-gpu/0123456789abcdef", nodeproto.StreamQuality))
	alone := nodeproto.MakeChunksReport{
		State: nodeproto.RunDone, LastChunk: 6, SpeedPercent: 333, FilmMadeMs: 6000, DecodedOn: "gpu",
	}
	if report := receiveChunksReport(t, core); report != alone {
		t.Errorf("a quality's run alone reported %+v; want %+v", report, alone)
	}
	core.sendMakeChunks(2, run("1/chunks-picture/0123456789abcdef", nodeproto.StreamPicture))
	if report := receiveChunksReport(t, core); report != (nodeproto.MakeChunksReport{
		State: nodeproto.RunDone, LastChunk: 6,
	}) {
		t.Errorf("a copied picture's run reported %+v; want no figures", report)
	}
	core.sendMakeChunks(3, run("1/chunks-waiting/0123456789abcdef", nodeproto.StreamPicture))
	for {
		if progress := testutil.Receive(t, core.progress, "job 3's first chunk"); progress.JobID == 3 &&
			progress.ChunksMadeThrough != nil {
			break
		}
	}
	core.sendMakeChunks(4, run("1/chunks-q3000k720p/0123456789abcdef", nodeproto.StreamQuality))
	if report := receiveChunksReport(t, core); !report.WasBesideAnotherRun || report.SpeedPercent != 333 {
		t.Errorf("a quality's run beside another reported %+v; want it beside another, at 333 %%", report)
	}
	core.commands <- nodeproto.Command{CommandSeq: 5, JobID: 3, Work: nodeproto.Work{Type: nodeproto.CommandCancel}}
}

// Each run of chunks after a seek opens the same unchanged film; the payload hashes the node shares
// with its byte socket remember its hash, so it is hashed once, not at every run (audit N4).
func TestAgent_RunsOfAnUnchangedFilmHashItOnceInTheHashesTheNodeShares(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	shared := namedfile.NewHashes()
	root, _ := startAgentWithWorkers(t, socket, Workers{ChunkMaker: fakeChunkMaker{}, Hashes: shared})
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	video := nodeproto.BytesFile{
		RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: nodetest.PayloadHashHex(t, "a film"),
	}
	for job := range int64(3) {
		core.sendMakeChunks(job+1, nodeproto.MakeChunks{
			Video: video, StoreDir: "1/chunks-picture/0123456789abcdef", FirstChunk: int(job),
			BoundsMs: []int64{job * 2000, job*2000 + 2000},
			Stream:   nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
		})
		if done := receiveChunksReport(t, core); done.State != nodeproto.RunDone {
			t.Fatalf("run %d ended %+v; want done", job+1, done)
		}
	}
	if computed := shared.Computed(); computed != 1 {
		t.Errorf("three runs of one unchanged film computed %d payload hashes; want 1", computed)
	}
}

func (fakeChunkMaker) StretchOf(nodeproto.MakeChunks) execx.Stretch { return execx.Stretch{} }

func (pacedChunkMaker) StretchOf(nodeproto.MakeChunks) execx.Stretch { return execx.Stretch{} }
