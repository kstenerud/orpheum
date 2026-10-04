// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"sync"
	"time"
)

// IdempotencyKeyHeader names one action a client asks for, the same on every retry of it: a POST
// carrying one is done once, and a repeat with the same key is given the first one's answer
// (D244 §4).
const IdempotencyKeyHeader = "Idempotency-Key"

// idempotentAnswerKept is how long a POST's answer is kept for a repeat of its key: longer than any
// wait a client retries after, a Retry-After or a lost connection. An internal bound: past it, a
// repeat is a new action, which is what a person who waited that long means.
const idempotentAnswerKept = 10 * time.Minute

// maxIdempotentBodyBytes bounds the request body hashed to tell a repeat from another action under
// the same key: larger than any body the API takes.
const maxIdempotentBodyBytes = 1 << 20

// keptAnswer is the answer to the first POST of a key, or, until done closes, its making.
type keptAnswer struct {
	done        chan struct{}
	requestHash [sha256.Size]byte
	keptUntil   time.Time
	status      int
	header      http.Header
	body        []byte
}

// idempotentAnswers keeps each key's first answer, by the key's scope: who asked, the method, the
// path and the key.
type idempotentAnswers struct {
	mutex sync.Mutex
	// byScope is each key's answer; guarded by mutex.
	byScope map[[sha256.Size]byte]*keptAnswer
}

// idempotent answers a POST that carries an Idempotency-Key once: a repeat of the key by the same
// caller, for the same path and the same body, waits for the first and is given its answer; one with
// another body under the key is refused as a conflict. Every other request goes to next as it is.
func (server *Server) idempotent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key := request.Header.Get(IdempotencyKeyHeader)
		dependencies := server.dependencies.Load()
		if request.Method != http.MethodPost || key == "" || dependencies == nil || dependencies.Clock == nil {
			next.ServeHTTP(writer, request)
			return
		}
		body, err := io.ReadAll(io.LimitReader(request.Body, maxIdempotentBodyBytes))
		if err != nil {
			writeErrorAnswer(writer,
				invalidRequestAt(http.StatusBadRequest, "body", "the request's body could not be read"))
			return
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		scope := sha256.Sum256([]byte(request.Header.Get("Authorization") + "\x00" + request.Method + "\x00" +
			request.URL.Path + "\x00" + key))
		now := dependencies.Clock.Now()
		kept, isFirst := server.answers.take(scope, sha256.Sum256(body), now)
		if kept == nil {
			writeErrorAnswer(writer, newError(http.StatusConflict, CodeConflict,
				"this Idempotency-Key was used for another request; make a new key for a new action"))
			return
		}
		if isFirst {
			recorded := &recordedAnswer{header: http.Header{}, status: http.StatusOK}
			next.ServeHTTP(recorded, request)
			kept.status, kept.header, kept.body = recorded.status, recorded.header, recorded.body.Bytes()
			close(kept.done)
		} else {
			select {
			case <-kept.done:
			case <-request.Context().Done():
				return
			}
		}
		maps.Copy(writer.Header(), kept.header)
		writer.WriteHeader(kept.status)
		_, _ = writer.Write(kept.body) //nolint:errcheck // the client went; nothing to tell
	})
}

// take is the kept answer of a key's scope and whether this request is its first, making one when
// none is kept or the kept one is past its time; nil when the key is kept for another body.
func (answers *idempotentAnswers) take(
	scope, requestHash [sha256.Size]byte, now time.Time,
) (*keptAnswer, bool) {
	answers.mutex.Lock()
	defer answers.mutex.Unlock()
	if answers.byScope == nil {
		answers.byScope = map[[sha256.Size]byte]*keptAnswer{}
	}
	for each, kept := range answers.byScope {
		if now.After(kept.keptUntil) {
			delete(answers.byScope, each)
		}
	}
	if kept, isKept := answers.byScope[scope]; isKept {
		if kept.requestHash != requestHash {
			return nil, false
		}
		return kept, false
	}
	kept := &keptAnswer{done: make(chan struct{}), requestHash: requestHash, keptUntil: now.Add(idempotentAnswerKept)}
	answers.byScope[scope] = kept
	return kept, true
}

// recordedAnswer is a response written to memory, to be kept and given to each repeat.
type recordedAnswer struct {
	header http.Header
	status int
	body   bytes.Buffer
}

// Header is the answer's header, as the handler sets it.
func (recorded *recordedAnswer) Header() http.Header { return recorded.header }

// WriteHeader records the answer's status.
func (recorded *recordedAnswer) WriteHeader(status int) { recorded.status = status }

// Write records the answer's body.
func (recorded *recordedAnswer) Write(content []byte) (int, error) {
	written, err := recorded.body.Write(content)
	if err != nil {
		return written, err //nolint:wrapcheck // a bytes.Buffer's own; it never fails
	}
	return written, nil
}

// writeErrorAnswer writes an error in the API's one shape, outside an operation (cut-a §5.2).
func writeErrorAnswer(writer http.ResponseWriter, failure *Error) {
	encoded, err := json.Marshal(failure.ErrorBody)
	if err != nil {
		http.Error(writer, failure.Error(), failure.status)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(failure.status)
	_, _ = writer.Write(encoded) //nolint:errcheck // the client went; nothing to tell
}
