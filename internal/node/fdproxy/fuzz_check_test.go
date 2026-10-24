// SPDX-License-Identifier: AGPL-3.0-or-later

package fdproxy

import (
	"bytes"
	"errors"
	"testing"
)

// classOf is a failure as the model classes it.
func classOf(failure error) failureClass {
	switch {
	case failure == nil:
		return failedNot
	case errors.Is(failure, ErrViolation):
		return failedViolation
	case errors.Is(failure, ErrReadFailed):
		return failedRead
	case errors.Is(failure, ErrOverBudget):
		return failedBudget
	}
	return failedOtherwise
}

// check holds what the run ended with to the model.
func (run *fuzzRun) check(account Account) {
	t := run.t
	t.Helper()
	run.source.mutex.Lock()
	outside := run.source.outside
	run.source.mutex.Unlock()
	if outside != nil {
		t.Errorf("the node read outside the film: %v", outside)
	}
	run.drainer.mutex.Lock()
	problem, isTruncated := run.drainer.problem, run.drainer.truncated
	run.drainer.mutex.Unlock()
	if problem != nil {
		t.Error(problem)
	}
	got := run.drainer.received()
	if run.setup.mode == fuzzBurst {
		run.checkBurst(account, got)
		return
	}
	run.checkServed(got, isTruncated)
	run.checkAccount(account)
}

// checkServed holds what the node sent and read to the model: exactly, or, once FFmpeg stopped
// reading its DATA, a prefix of it.
func (run *fuzzRun) checkServed(got []nodeFrame, isTruncated bool) {
	t, model := run.t, run.model
	t.Helper()
	if model.isStalled {
		// A writer blocked on the pipe FFmpeg stopped reading is cut off by Finish, mid-frame perhaps.
		if len(got) < run.framesStalled || len(got) > len(model.frames) {
			t.Errorf("the node sent %d frames; want %d to %d", len(got), run.framesStalled, len(model.frames))
		}
		checkFrames(t, got, model.frames[:min(len(got), len(model.frames))])
	} else {
		if isTruncated {
			t.Error("the node sent a frame cut off")
		}
		checkFrames(t, got, model.frames)
		if reads := run.source.readCount(); reads != model.reads {
			t.Errorf("the film was read %d times; want %d", reads, model.reads)
		}
	}
}

// checkAccount holds the account and the kill to the model.
func (run *fuzzRun) checkAccount(account Account) {
	t, model := run.t, run.model
	t.Helper()
	class, isKilled := classOf(account.Failure), run.killed.Load()
	cutOff := model.failedStalled && class == failedNot
	if class != model.failure && !cutOff {
		t.Errorf("the account's failure is %v (%v); want %v", class, account.Failure, model.failure)
	}
	if isKilled != model.isKilled && (!cutOff || isKilled) {
		t.Errorf("killed %v; want %v, the failure %v", isKilled, model.isKilled, account.Failure)
	}
	checkDone(t, account, model)
	if model.isStalled {
		if account.ServedBytes < run.spentStalled || account.ServedBytes > model.spent {
			t.Errorf("served %d bytes; want %d to %d", account.ServedBytes, run.spentStalled, model.spent)
		}
	} else if account.ServedBytes != model.spent {
		t.Errorf("served %d bytes; want %d", account.ServedBytes, model.spent)
	}
}

// checkBurst holds a burst's run to what the reader's order alone says: the opening exchange the
// model's, each DATA the next READ's, the breach the model found failing the run, and the DONE
// and the late frames the model's when nothing ended the reading first.
func (run *fuzzRun) checkBurst(account Account, got []nodeFrame) {
	t, model := run.t, run.model
	t.Helper()
	opening := model.openingFrames()
	if len(got) < opening {
		t.Fatalf("the node sent %d frames; want at least the opening exchange's %d", len(got), opening)
	}
	checkFrames(t, got[:opening], model.frames[:opening])
	data := got[opening:]
	if len(data) > len(model.accepted) {
		t.Fatalf("the node sent %d DATA for %d READs", len(data), len(model.accepted))
	}
	want := make([]nodeFrame, 0, len(data))
	served := int64(0)
	for _, read := range model.accepted[:len(data)] {
		end := read.offset + int64(read.length)
		want = append(want, nodeFrame{kind: typeData, sequence: read.sequence, body: model.film[read.offset:end]})
		served += int64(read.length)
	}
	checkFrames(t, data, want)
	if model.isBreach && account.Failure == nil {
		t.Error("the stream broke the protocol, and the run did not fail")
	}
	if account.Failure == nil {
		checkDone(t, account, model)
	} else if account.Done != nil && (model.done == nil || *account.Done != *model.done) {
		t.Errorf("the account's DONE is %+v; want %+v", account.Done, model.done)
	}
	if account.ServedBytes < served || account.ServedBytes > served+int64(model.terms.LongestRead) {
		t.Errorf("served %d bytes, of which DATA carried %d", account.ServedBytes, served)
	}
}

func checkDone(t *testing.T, account Account, model *fuzzModel) {
	t.Helper()
	if (account.Done == nil) != (model.done == nil) || (account.Done != nil && *account.Done != *model.done) {
		t.Errorf("the account's DONE is %+v; want %+v", account.Done, model.done)
	}
	if account.LateFrames != model.late {
		t.Errorf("%d late frames; want %d", account.LateFrames, model.late)
	}
}

func checkFrames(t *testing.T, got, want []nodeFrame) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("the node sent %d frames; want %d", len(got), len(want))
	}
	for index := range min(len(got), len(want)) {
		if got[index].kind != want[index].kind || got[index].sequence != want[index].sequence ||
			!bytes.Equal(got[index].body, want[index].body) {
			t.Errorf("frame %d: type %d, number %d, %d bytes; want type %d, number %d, %d bytes", index,
				got[index].kind, got[index].sequence, len(got[index].body), want[index].kind, want[index].sequence,
				len(want[index].body))
			return
		}
	}
}
