// SPDX-License-Identifier: AGPL-3.0-or-later

package fdproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/testutil"
)

// fuzzFilm is the fuzzed run's film: each read answers at once, fails, or waits until the fuzz
// releases it or Finish stops it, as the input says.
type fuzzFilm struct {
	bytes      []byte
	behaviours uint32

	mutex sync.Mutex
	reads int
	// gate is the waiting read's, closed to release it. Guarded by mutex.
	gate chan struct{}
	// outside is a read the channel asked for outside the film. Guarded by mutex.
	outside error
}

var errFuzzedStorage = errors.New("the fuzzed storage failed")

func newFuzzFilm(size int, behaviours uint32) *fuzzFilm {
	contents := make([]byte, size)
	for index := range contents {
		contents[index] = byte(index*7) ^ byte(index>>8)
	}
	return &fuzzFilm{bytes: contents, behaviours: behaviours}
}

func (source *fuzzFilm) Size() int64 { return int64(len(source.bytes)) }

func (source *fuzzFilm) ReadAt(ctx context.Context, buffer []byte, offset int64) error {
	source.mutex.Lock()
	behaviour := behaviourOf(source.behaviours, source.reads)
	source.reads++
	if offset < 0 || offset > int64(len(source.bytes)) || int64(len(buffer)) > int64(len(source.bytes))-offset {
		source.outside = fmt.Errorf("a read of %d bytes at %d of a film of %d", len(buffer), offset, len(source.bytes))
		source.mutex.Unlock()
		return source.outside
	}
	var gate chan struct{}
	if behaviour == behaviourGated {
		source.gate = make(chan struct{})
		gate = source.gate
	}
	source.mutex.Unlock()
	switch behaviour {
	case behaviourFails:
		return errFuzzedStorage
	case behaviourGated:
		select {
		case <-gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	copy(buffer, source.bytes[offset:])
	return nil
}

// release ends the read waiting, if one is.
func (source *fuzzFilm) release() {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	if source.gate != nil {
		close(source.gate)
		source.gate = nil
	}
}

func (source *fuzzFilm) readCount() int {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	return source.reads
}

// fuzzDrainer is FFmpeg reading the node's frames: it keeps each, and can be stalled, as an FFmpeg
// that stops reading its DATA.
type fuzzDrainer struct {
	mutex  sync.Mutex
	frames []nodeFrame
	// problem is a frame no node may send; truncated, a frame cut off by the pipe's end. Guarded by mutex.
	problem   error
	truncated bool

	isStalled atomic.Bool
	resume    chan struct{}
	done      chan struct{}
}

func startFuzzDrainer(ctx context.Context, from *os.File) *fuzzDrainer {
	drainer := &fuzzDrainer{resume: make(chan struct{}), done: make(chan struct{})}
	go drainer.drain(ctx, from)
	return drainer
}

func (drainer *fuzzDrainer) drain(ctx context.Context, from *os.File) {
	defer close(drainer.done)
	defer func() { _ = from.Close() }() //nolint:errcheck // the fuzz's own end
	for {
		if drainer.isStalled.Load() {
			select {
			case <-drainer.resume:
			case <-ctx.Done():
				return
			}
		}
		raw := make([]byte, headerBytes)
		if count, err := io.ReadFull(from, raw); err != nil {
			drainer.record(nodeFrame{}, nil, count > 0)
			return
		}
		got := decodeHeader(raw)
		if got.length < 0 || got.length > largestBlockBytes {
			drainer.record(nodeFrame{}, fmt.Errorf("the node sent a frame of type %d with a body of %d bytes", got.kind,
				got.length), false)
			return
		}
		body := make([]byte, got.length)
		if _, err := io.ReadFull(from, body); err != nil {
			drainer.record(nodeFrame{}, nil, true)
			return
		}
		drainer.record(nodeFrame{kind: got.kind, sequence: got.sequence, body: body}, nil, false)
	}
}

func (drainer *fuzzDrainer) record(got nodeFrame, problem error, isTruncated bool) {
	drainer.mutex.Lock()
	defer drainer.mutex.Unlock()
	switch {
	case problem != nil:
		drainer.problem = problem
	case isTruncated:
		drainer.truncated = true
	case got.kind != 0:
		drainer.frames = append(drainer.frames, got)
	}
}

func (drainer *fuzzDrainer) received() []nodeFrame {
	drainer.mutex.Lock()
	defer drainer.mutex.Unlock()
	return append([]nodeFrame(nil), drainer.frames...)
}

// fuzzRun is one fuzzed run: the channel, FFmpeg's ends, and the model it is held to.
type fuzzRun struct {
	t       *testing.T
	channel *Channel
	source  *fuzzFilm
	setup   fuzzSetup
	model   *fuzzModel
	writing *os.File
	drainer *fuzzDrainer
	killed  atomic.Bool
	// finished has Finish's account once it is begun, and is nil before.
	finished chan Account
	sequence int8
	sent     int
	// framesStalled and spentStalled are what the model said had been served when FFmpeg stopped
	// reading.
	framesStalled int
	spentStalled  int64
}

func startFuzzRun(t *testing.T, channel *Channel, source *fuzzFilm, setup fuzzSetup) *fuzzRun {
	t.Helper()
	ends := channel.ChildFiles()
	reading, writing := duplicate(t, ends[0]), duplicate(t, ends[1])
	run := &fuzzRun{
		t: t, channel: channel, source: source, setup: setup, writing: writing,
		model: &fuzzModel{
			run: setup.config.Run, budget: setup.config.BudgetBytes, most: setup.config.MostOutstanding,
			film: source.bytes, behaviours: setup.behaviours, ignoresOutstanding: setup.mode == fuzzBurst,
		},
	}
	channel.Start(t.Context(), func() { run.killed.Store(true) })
	run.drainer = startFuzzDrainer(t.Context(), reading)
	return run
}

// burst writes FFmpeg's bytes at once and exits, as the first form of this fuzz did.
func (run *fuzzRun) burst(stream []byte) Account {
	stream = stream[:min(len(stream), mostFuzzedBytes)]
	_, _ = run.writing.Write(stream) //nolint:errcheck // a stream the node stopped reading is still fine
	run.model.feed(stream)
	return run.end()
}

// paced writes FFmpeg's bytes a chunk at a time, waiting after each for the node to judge it.
func (run *fuzzRun) paced(stream []byte) Account {
	stream = stream[:min(len(stream), mostFuzzedBytes)]
	chunk := run.setup.chunk
	if chunk == 0 {
		chunk = headerBytes
	}
	for len(stream) > 0 && run.model.phase != phaseStopped {
		piece := stream[:min(chunk, len(stream))]
		stream = stream[len(piece):]
		_, _ = run.writing.Write(piece) //nolint:errcheck // the node may have stopped reading
		run.model.feed(piece)
		run.settle()
	}
	return run.end()
}

// structured makes FFmpeg's opening exchange and then up to mostFuzzedSteps steps from the input.
func (run *fuzzRun) structured(input *fuzzInput) Account {
	versions := versionRange{lowest: 1, highest: 1}
	if input.nextByte()%4 == 0 {
		versions = versionRange{lowest: int16(input.next16()), highest: int16(input.next16())} //nolint:gosec // signed
	}
	run.sendFrame(typeHello, run.nextSequence(), helloBody(versions))
	run.settle()
	var block, largest int32
	var outstanding int8
	if input.nextByte()%4 == 0 {
		block, largest = int32(input.next32()), int32(input.next32()) //nolint:gosec // signed
		outstanding = int8(input.nextByte())                          //nolint:gosec // signed
	} else {
		block = 1 + int32(input.next16()%largestFuzzedFilm)
		largest, outstanding = block+int32(input.next16()), 1+int8(input.nextByte()%mostOutstanding)
	}
	run.sendFrame(typeCapability, run.nextSequence(), put(put(put([]byte(nil), block), largest), outstanding))
	run.settle()
	for step := 0; step < mostFuzzedSteps && !input.exhausted() && run.sent < mostFuzzedBytes; step++ {
		run.step(input)
	}
	return run.end()
}

// The steps of a structured run.
const (
	stepRead = iota
	stepReadAgain
	stepReadFuzzed
	stepReadMisnumbered
	stepDone
	stepOtherFrame
	stepGarbage
	stepReleaseFilm
	stepHold
	stepRelease
	stepFinish
	stepStall
	steps
)

func (run *fuzzRun) step(input *fuzzInput) {
	switch step := input.nextByte() % steps; step {
	case stepRead, stepReadAgain:
		run.sendFrame(typeRead, run.nextSequence(), run.readWithin(input))
	case stepReadFuzzed:
		offset, length := int64(input.next64()), int32(input.next32()) //nolint:gosec // signed
		run.sendFrame(typeRead, run.nextSequence(), readBody(offset, length))
	case stepReadMisnumbered:
		body := run.readWithin(input)
		sequence := int8(input.nextByte()) //nolint:gosec // signed
		if sequence == run.sequence {
			sequence = -1
		}
		run.nextSequence()
		run.sendFrame(typeRead, sequence, body)
	case stepDone:
		run.sendFrame(typeDone, run.nextSequence(), run.doneBody(input))
	case stepOtherFrame:
		kind := int8(input.nextByte()) //nolint:gosec // signed
		run.sendFrame(kind, run.nextSequence(), input.padded(int(input.next16()%1100)))
	case stepGarbage:
		run.send(input.bytes(1 + int(input.nextByte()%7)))
	default:
		run.control(step)
	}
	run.settle()
}

// control is a step that sends nothing: the film released, Hold, Release, Finish begun, or FFmpeg
// ceasing to read its DATA.
func (run *fuzzRun) control(step byte) {
	switch step {
	case stepReleaseFilm:
		if run.finished == nil {
			run.source.release()
			run.model.releaseFilm()
		}
	case stepHold:
		run.channel.Hold()
		run.model.hold()
	case stepRelease:
		run.channel.Release()
		run.model.release()
	case stepFinish:
		if run.finished == nil {
			run.beginFinish()
		}
	case stepStall:
		if !run.model.isStalled {
			run.drainer.isStalled.Store(true)
			run.framesStalled, run.spentStalled = len(run.model.frames), run.model.spent
			run.model.stall()
		}
	}
}

// readWithin is a READ's body within the terms, but for a film of no bytes left past its offset.
func (run *fuzzRun) readWithin(input *fuzzInput) []byte {
	size, longest := int64(run.setup.filmSize), int64(max(run.model.terms.LongestRead, 1))
	offset := int64(input.next16()) % (size + 1)
	room := max(min(longest, size-offset), 1)
	return readBody(offset, int32(1+int64(input.next16())%room)) //nolint:gosec // within the film
}

// doneBody is a DONE: for the run, 0 or another; OK, ERR or another result; and for ERR, any kind,
// a code and a message whose length field mostly says its length.
func (run *fuzzRun) doneBody(input *fuzzInput) []byte {
	choice := input.nextByte()
	id := run.setup.config.Run
	switch choice & 3 {
	case 2:
		id = 0
	case 3:
		id = int64(input.next64()) //nolint:gosec // signed
	}
	body := put([]byte(nil), id)
	result := (choice >> 2) & 3
	if result < 2 {
		body = put(body, int8(resultOK))
		if choice&0x40 != 0 {
			body = append(body, 0)
		}
		return body
	}
	resultByte := int8(resultErr)
	if result == 3 {
		resultByte = int8(input.nextByte()) //nolint:gosec // signed
	}
	kind := int8(input.nextByte() % 6)
	code := int32(input.next16())
	if choice&0x80 != 0 {
		code = int32(input.next32()) //nolint:gosec // signed
	}
	length := int(input.nextByte())
	if choice&0x10 != 0 {
		length = int(input.next16() % 1100)
	}
	message := input.padded(length)
	lengthField := int16(length)
	if choice&0x20 != 0 {
		lengthField = int16(input.next16()) //nolint:gosec // signed
	}
	return append(put(put(put(put(body, resultByte), kind), code), lengthField), message...)
}

func (run *fuzzRun) nextSequence() int8 {
	sequence := run.sequence
	run.sequence = nextSequence(sequence)
	return sequence
}

func (run *fuzzRun) sendFrame(kind, sequence int8, body []byte) {
	run.send(frame(kind, sequence, body))
}

// send writes what FFmpeg sends, in chunks when the run says so; nothing once the node has stopped
// reading, or FFmpeg has stopped reading its DATA, after which it falls silent.
func (run *fuzzRun) send(raw []byte) {
	if run.model.phase == phaseStopped || run.model.isStalled {
		return
	}
	run.sent += len(raw)
	for rest := raw; len(rest) > 0; {
		piece := rest
		if run.setup.chunk > 0 {
			piece = rest[:min(run.setup.chunk, len(rest))]
		}
		rest = rest[len(piece):]
		_, _ = run.writing.Write(piece) //nolint:errcheck // the node may have stopped reading
	}
	run.model.feed(raw)
}

// beginFinish is FFmpeg exiting: Finish begins while FFmpeg's pipe still holds what it sent, and may
// be sent more before it closes.
func (run *fuzzRun) beginFinish() {
	run.finished = make(chan Account, 1)
	go func() { run.finished <- run.channel.Finish() }()
	run.waitFor("Finish to begin", func() bool {
		run.channel.mutex.Lock()
		defer run.channel.mutex.Unlock()
		return run.channel.isClosing
	})
	run.model.beginFinish()
}

// end ends the run: FFmpeg's pipe closes, Finish runs, and its account and what the node sent are
// held to the model; it returns the account.
func (run *fuzzRun) end() Account {
	_ = run.writing.Close() //nolint:errcheck // FFmpeg exits
	run.model.end()
	if run.finished == nil {
		// The node reads all FFmpeg sent before Finish begins, once FFmpeg has exited, kills nothing
		// (Stopping); so its frames are judged first, as a run's are while it runs.
		testutil.Receive(run.t, run.channel.readerDone, "the node to read the whole stream")
		run.settle()
		run.beginFinish()
	}
	var account Account
	select {
	case account = <-run.finished:
	case <-time.After(3 * drainWait):
		run.t.Fatalf("Finish did not return for a run that sent %d bytes", run.sent)
	}
	close(run.drainer.resume)
	testutil.Receive(run.t, run.drainer.done, "the node to close what it sends")
	run.check(account)
	return account
}

// fuzzState is what the fuzz can see of the node while it runs, which the model says too.
type fuzzState struct {
	frames, reads, waiting, late int
	failed, isDone, killed       bool
	spent                        int64
}

func (run *fuzzRun) observed() fuzzState {
	run.channel.mutex.Lock()
	defer run.channel.mutex.Unlock()
	return fuzzState{
		frames: len(run.drainer.received()), reads: run.source.readCount(), waiting: len(run.channel.waiting),
		late: run.channel.account.LateFrames, failed: run.channel.account.Failure != nil,
		isDone: run.channel.account.Done != nil, killed: run.killed.Load(), spent: run.channel.account.ServedBytes,
	}
}

func (model *fuzzModel) state() fuzzState {
	return fuzzState{
		frames: len(model.frames), reads: model.reads, waiting: len(model.waiting), late: model.late,
		failed: model.failure != failedNot, isDone: model.done != nil, killed: model.isKilled, spent: model.spent,
	}
}

// settle waits for the node to reach the state the model says, but in a burst, whose timing is the
// scheduler's, and once FFmpeg has stopped reading, when the writer may wait on the pipe.
func (run *fuzzRun) settle() {
	if run.setup.mode == fuzzBurst || run.model.isStalled {
		return
	}
	want := run.model.state()
	run.waitFor(fmt.Sprintf("the node to reach %+v", want), func() bool { return run.observed() == want })
}

// waitFor waits for condition, yielding between looks, and fails the run after fuzzSettleLimit.
func (run *fuzzRun) waitFor(what string, condition func() bool) {
	run.t.Helper()
	limit := time.After(fuzzSettleLimit)
	for {
		if condition() {
			return
		}
		select {
		case <-limit:
			run.t.Fatalf("waited %s for %s; the node is at %+v", fuzzSettleLimit, what, run.observed())
		default:
			runtime.Gosched()
		}
	}
}
