// SPDX-License-Identifier: AGPL-3.0-or-later

package fdproxy

import (
	"encoding/binary"
)

// fuzzModel is what the specification says the node does with the bytes FFmpeg sends and the run's
// other events, worked out apart from the channel, so the fuzz can hold the channel to it: the frames
// it sends, the reads it makes of the film, and its account. It judges FFmpeg's bytes as they come,
// a frame at a time, as the node's reader does, and plays the node's writer against the fuzzed
// film.
type fuzzModel struct {
	run, budget int64
	most        int8
	film        []byte
	behaviours  uint32
	// ignoresOutstanding is a burst's: there the writer races the reader, so how many READs wait when
	// one comes is the scheduler's, and the model counts none.
	ignoresOutstanding bool

	pending  []byte
	phase    modelPhase
	expected int8
	terms    Terms
	waiting  []readRequest
	writer   modelWriter
	reading  readRequest
	isHeld   bool
	// isClosing is Finish begun: FFmpeg has exited, the writer is gone, and nothing is killed.
	isClosing bool

	done      *Done
	late      int
	failure   failureClass
	isKilled  bool
	spent     int64
	reads     int
	frames    []nodeFrame
	accepted  []readRequest
	isBreach  bool
	isStalled bool
	// failedStalled is a failure the writer met after FFmpeg stopped reading its DATA, which a writer
	// blocked on the full pipe never reaches.
	failedStalled bool
}

type modelPhase int

const (
	phaseHello modelPhase = iota
	phaseCapability
	phaseServing
	phaseStopped
)

type modelWriter int

const (
	writerNone modelWriter = iota // before the terms
	writerIdle
	writerReading // in a read of the film the fuzz holds
	writerGone
)

type failureClass int

const (
	failedNot failureClass = iota
	failedViolation
	failedRead
	failedBudget
	failedOtherwise
)

func (class failureClass) String() string {
	return [...]string{"none", "a violation", "a failed read", "over budget", "another failure"}[class]
}

// The film's behaviours, two bits for each read in turn: answer at once (twice as likely), wait for
// the fuzz to release it, or fail.
const (
	behaviourAnswer   = 0
	behaviourGated    = 2
	behaviourFails    = 3
	behaviourBits     = 2
	behavioursInInput = 16
)

func behaviourOf(behaviours uint32, read int) uint32 {
	behaviour := behaviours >> (behaviourBits * (read % behavioursInInput)) & 3
	if behaviour == 1 {
		return behaviourAnswer
	}
	return behaviour
}

// nodeFrame is one frame the node sent.
type nodeFrame struct {
	kind, sequence int8
	body           []byte
}

// feed judges the bytes FFmpeg sent, every frame they complete.
func (model *fuzzModel) feed(raw []byte) {
	if model.phase == phaseStopped {
		return
	}
	model.pending = append(model.pending, raw...)
	for model.judgeNext() {
	}
}

// judgeNext judges the next frame in what FFmpeg sent: its header as soon as it is whole, and the
// frame once its body is. It says whether it took a frame.
func (model *fuzzModel) judgeNext() bool {
	if model.phase == phaseStopped || len(model.pending) < headerBytes {
		return false
	}
	kind, sequence := int8(model.pending[0]), int8(model.pending[1])       //nolint:gosec // G115: signed bytes
	length := int64(int32(binary.LittleEndian.Uint32(model.pending[2:6]))) //nolint:gosec // G115: as above
	if !model.headerKeepsTheRules(kind, sequence, length) {
		model.breakFraming()
		return false
	}
	if int64(len(model.pending)-headerBytes) < length {
		return false
	}
	body := model.pending[headerBytes : headerBytes+length]
	model.pending = model.pending[headerBytes+length:]
	model.expected = int8((int(model.expected) + 1) % 128)
	model.judge(kind, body)
	return true
}

// headerKeepsTheRules is the Frames section's: no negative field, the number expected, and a body
// within its type's bounds, of a type FFmpeg's side sends.
func (model *fuzzModel) headerKeepsTheRules(kind, sequence int8, length int64) bool {
	if sequence < 0 || length < 0 || sequence != model.expected {
		return false
	}
	switch kind {
	case typeHello:
		return length == 4
	case typeCapability:
		return length == 9
	case typeRead:
		return length == 12
	case typeDone:
		return length >= 9 && length <= 16+1024
	}
	return false
}

func (model *fuzzModel) judge(kind int8, body []byte) {
	switch model.phase {
	case phaseHello:
		if kind != typeHello {
			model.violate()
			return
		}
		lowest := int16(binary.LittleEndian.Uint16(body[0:2]))  //nolint:gosec // G115: signed
		highest := int16(binary.LittleEndian.Uint16(body[2:4])) //nolint:gosec // G115: signed
		// The node speaks version 1 alone, so FFmpeg's range must be valid and take in 1 (HELLO).
		if lowest < 1 || highest < lowest || lowest > 1 {
			model.violate()
			return
		}
		model.send(typeHello, 0, []byte{1, 0, 1, 0})
		model.phase = phaseCapability
	case phaseCapability:
		if kind != typeCapability {
			model.violate()
			return
		}
		block := int32(binary.LittleEndian.Uint32(body[0:4]))   //nolint:gosec // G115: signed
		largest := int32(binary.LittleEndian.Uint32(body[4:8])) //nolint:gosec // G115: signed
		outstanding := int8(body[8])                            //nolint:gosec // G115: signed
		if block < 1 || largest < block || outstanding < 1 || outstanding > 16 {
			model.violate()
			return
		}
		model.terms = Terms{
			Version: 1, Run: model.run, FileSize: int64(len(model.film)), BlockSize: min(block, 4<<20),
			LongestRead: min(block, 4<<20), Outstanding: min(outstanding, model.most),
		}
		model.send(typeRequest, 1, requestBody(model.terms))
		model.phase, model.writer = phaseServing, writerIdle
		model.serve()
	case phaseServing:
		model.take(kind, body)
	case phaseStopped: // judgeNext judges nothing once the reader has stopped
	}
}

// take is a frame after the opening exchange: a READ, a DONE, or a breach; and after DONE, a late
// frame that changes nothing (DONE).
func (model *fuzzModel) take(kind int8, body []byte) {
	if model.done != nil {
		model.late++
		return
	}
	switch kind {
	case typeDone:
		done, isValid := modelDone(body)
		if !isValid {
			model.violate()
			return
		}
		model.done, model.waiting = &done, nil
		if model.writer != writerNone {
			model.writer = writerGone // DONE stops the read under way, and nothing more is served
		}
	case typeRead:
		read := readRequest{
			sequence: int8((int(model.expected) + 127) % 128),
			offset:   int64(binary.LittleEndian.Uint64(body[0:8])),  //nolint:gosec // G115: signed
			length:   int32(binary.LittleEndian.Uint32(body[8:12])), //nolint:gosec // G115: signed
		}
		size := int64(len(model.film))
		// Checked as Checks Without Overflow says: offset first, so size − offset cannot overflow.
		if read.offset < 0 || read.offset > size || read.length <= 0 || read.length > model.terms.LongestRead ||
			int64(read.length) > size-read.offset {
			model.violate()
			return
		}
		if !model.ignoresOutstanding && len(model.waiting) >= int(model.terms.Outstanding) {
			model.violate()
			return
		}
		model.accepted = append(model.accepted, read)
		model.waiting = append(model.waiting, read)
		model.serve()
	default:
		model.violate()
	}
}

// modelDone is a DONE's body judged by its section: OK of exactly 9 bytes, or ERR with a kind of
// version 1, a code not negative, and a message of its length, 0 to 1,024 bytes.
func modelDone(body []byte) (Done, bool) {
	done := Done{Run: int64(binary.LittleEndian.Uint64(body[0:8]))} //nolint:gosec // G115: signed
	if done.Run < 0 {
		return Done{}, false
	}
	switch body[8] {
	case 0:
		done.IsOK = true
		return done, len(body) == 9
	case 1:
		if len(body) < 16 {
			return Done{}, false
		}
		done.Kind = Kind(int8(body[9]))                               //nolint:gosec // G115: signed
		done.Code = int32(binary.LittleEndian.Uint32(body[10:14]))    //nolint:gosec // G115: signed
		length := int(int16(binary.LittleEndian.Uint16(body[14:16]))) //nolint:gosec // G115: signed
		if done.Kind < 1 || done.Kind > 4 || done.Code < 0 || length < 0 || length > 1024 || len(body) != 16+length {
			return Done{}, false
		}
		done.Message = Sanitised(body[16:])
		return done, true
	}
	return Done{}, false
}

// serve is the node's writer: while it is free, serving, unheld and a READ waits, it takes the
// oldest, counts it against the budget before it reads (Budget), and reads the film, which answers
// at once, fails, or waits for the fuzz.
func (model *fuzzModel) serve() {
	for model.writer == writerIdle && !model.isClosing && model.failure == failedNot && !model.isHeld &&
		len(model.waiting) > 0 {
		read := model.waiting[0]
		model.waiting = model.waiting[1:]
		if model.budget > 0 && int64(read.length) > model.budget-model.spent {
			model.writer = writerGone
			model.fail(failedBudget)
			return
		}
		model.spent += int64(read.length)
		behaviour := behaviourOf(model.behaviours, model.reads)
		model.reads++
		switch behaviour {
		case behaviourAnswer:
			model.answer(read)
		case behaviourFails:
			model.writer = writerGone
			model.fail(failedRead) // the DATA section: a read that failed is never answered
			return
		case behaviourGated:
			model.writer, model.reading = writerReading, read
			return
		}
	}
}

func (model *fuzzModel) answer(read readRequest) {
	end := read.offset + int64(read.length)
	model.send(typeData, read.sequence, model.film[read.offset:end])
}

// releaseFilm ends the read the fuzz holds: its DATA is sent, unless the run has failed meanwhile,
// after which the node sends nothing more (Serving).
func (model *fuzzModel) releaseFilm() {
	if model.writer != writerReading || model.isClosing {
		return
	}
	if model.failure != failedNot {
		model.writer = writerGone
		return
	}
	model.answer(model.reading)
	model.writer = writerIdle
	model.serve()
}

func (model *fuzzModel) hold() { model.isHeld = true }

func (model *fuzzModel) release() {
	model.isHeld = false
	model.serve()
}

// stall is FFmpeg ceasing to read its DATA: from then on the writer may block on the pipe, so what
// it serves is a prefix of what the model says.
func (model *fuzzModel) stall() { model.isStalled = true }

// beginFinish is Finish begun, FFmpeg having exited: the writer ends, the READs waiting are dropped,
// and nothing more is killed (End of a Run, Stopping).
func (model *fuzzModel) beginFinish() {
	model.isClosing, model.waiting = true, nil
	if model.writer != writerNone {
		model.writer = writerGone
	}
}

// end is the end of what FFmpeg sent: a frame it left cut off breaks the framing.
func (model *fuzzModel) end() {
	if model.phase != phaseStopped && len(model.pending) > 0 {
		model.breakFraming()
	}
	model.phase = phaseStopped
}

// breakFraming is a frame that breaks the framing, after which the reader stops: a breach, or after
// DONE a late frame that changes nothing (DONE).
func (model *fuzzModel) breakFraming() {
	if model.done == nil {
		model.violate()
		return
	}
	model.late++
	model.phase = phaseStopped
}

// violate is a breach: the run fails and is killed, and the reader stops (Serving).
func (model *fuzzModel) violate() {
	model.isBreach = true
	model.phase = phaseStopped
	model.fail(failedViolation)
}

// fail records the first reason to fail the run, drops the READs waiting, and kills the run unless
// Finish has begun.
func (model *fuzzModel) fail(class failureClass) {
	if model.failure == failedNot {
		model.failure = class
		model.failedStalled = model.isStalled
	}
	model.waiting = nil
	if !model.isClosing {
		model.isKilled = true
	}
	if model.writer == writerIdle {
		model.writer = writerGone
	}
}

func (model *fuzzModel) send(kind, sequence int8, body []byte) {
	model.frames = append(model.frames, nodeFrame{kind: kind, sequence: sequence, body: body})
}

// openingFrames is how many of the frames the model says are the opening exchange's.
func (model *fuzzModel) openingFrames() int {
	count := 0
	for _, sent := range model.frames {
		if sent.kind != typeData {
			count++
		}
	}
	return count
}
