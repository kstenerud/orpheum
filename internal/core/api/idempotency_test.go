// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"
	"time"
)

// scopeOf is a distinct key's scope.
func scopeOf(index int) [sha256.Size]byte {
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], uint64(index)) //nolint:gosec // a test's small index
	return sha256.Sum256(raw[:])
}

// The answers kept for retries are bounded, more tightly for requests no one signed in made; past a
// bound a request is done without keeping its answer, and a repeat of a kept one still gets it (the
// second audit's K4, 2026-10-13: every POST's answer was kept, with no bound, and every POST walked
// them all).
func TestIdempotentAnswers_AreBounded(t *testing.T) {
	t.Parallel()
	answers := &idempotentAnswers{}
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	body := sha256.Sum256([]byte("body"))
	for index := range idempotentAnswersSignedOutAtMost {
		if _, isFirst, isKept := answers.take(scopeOf(index), body, now, true); !isFirst || !isKept {
			t.Fatalf("signed-out answer %d: first %v, kept %v", index, isFirst, isKept)
		}
	}
	if _, _, isKept := answers.take(scopeOf(-1), body, now, true); isKept {
		t.Error("a signed-out request past its bound was kept; want it done, not kept")
	}
	if _, isFirst, isKept := answers.take(scopeOf(-2), body, now, false); !isFirst || !isKept {
		t.Errorf("a signed-in request below the whole bound: first %v, kept %v; want kept", isFirst, isKept)
	}
	if kept, isFirst, isKept := answers.take(scopeOf(0), body, now, true); kept == nil || isFirst || !isKept {
		t.Errorf("a repeat of a kept answer: %v, first %v, kept %v; want the kept one", kept, isFirst, isKept)
	}
	if kept, _, isKept := answers.take(scopeOf(0), sha256.Sum256([]byte("other")), now, true); kept != nil ||
		!isKept {
		t.Error("the key with another body was not refused as a conflict")
	}
}

// An answer past its time is swept out, making room again (the second audit's K4).
func TestIdempotentAnswers_PastTheirTimeAreSweptOut(t *testing.T) {
	t.Parallel()
	answers := &idempotentAnswers{}
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	body := sha256.Sum256([]byte("body"))
	for index := range idempotentAnswersSignedOutAtMost {
		answers.take(scopeOf(index), body, now, true)
	}
	later := now.Add(idempotentAnswerKept + idempotentSweepEvery + time.Second)
	if _, isFirst, isKept := answers.take(scopeOf(-1), body, later, true); !isFirst || !isKept {
		t.Errorf("past every answer's time: first %v, kept %v; want room again", isFirst, isKept)
	}
	if len(answers.byScope) != 1 || answers.signedOut != 1 {
		t.Errorf("after the sweep %d answers are kept, %d signed out; want the one new one",
			len(answers.byScope), answers.signedOut)
	}
}
