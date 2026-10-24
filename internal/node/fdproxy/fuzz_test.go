// SPDX-License-Identifier: AGPL-3.0-or-later

package fdproxy

import (
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/testutil"
)

// fuzzBudget is the budget a fuzzed run has: three times the fuzzed film.
const fuzzBudget = 3000

// mostFuzzedBytes keeps a fuzzed stream inside a pipe's default 64 KiB, so writing it never waits
// on the node.
const mostFuzzedBytes = 60 << 10

// FFmpeg may have been taken over by the film it reads, so whatever it sends, the node must judge it
// and end: Finish returns, no more than the budget is served, and the node kills the run exactly
// when it records a reason to while the run runs (Serving). The seeds run with the package's tests; `go test -fuzz
// FuzzChannel_JudgesWhateverFFmpegSends ./internal/node/fdproxy/` explores beyond them.
func FuzzChannel_JudgesWhateverFFmpegSends(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, stream []byte) {
		if len(stream) > mostFuzzedBytes {
			stream = stream[:mostFuzzedBytes]
		}
		channel, err := Open(Config{Run: 7, Source: newFilm(1000), MostOutstanding: 2, BudgetBytes: fuzzBudget})
		if err != nil {
			t.Fatal(err)
		}
		ends := channel.ChildFiles()
		reading, writing := duplicate(t, ends[0]), duplicate(t, ends[1])
		var killed atomic.Bool
		channel.Start(t.Context(), func() { killed.Store(true) })
		drained := make(chan struct{})
		go func() {
			_, _ = io.Copy(io.Discard, reading) //nolint:errcheck // whatever the node sends is drained
			close(drained)
		}()
		_, _ = writing.Write(stream) //nolint:errcheck // a stream the node stopped reading is still fine
		_ = writing.Close()          //nolint:errcheck // FFmpeg exits
		// The node reads all FFmpeg sent before Finish begins, once FFmpeg has exited, kills nothing
		// (Stopping); so its frames are judged first, as a run's are while it runs.
		testutil.Receive(t, channel.readerDone, "the node to read the whole stream")
		finished := make(chan Account, 1)
		go func() { finished <- channel.Finish() }()
		var account Account
		select {
		case account = <-finished:
		case <-time.After(3 * drainWait):
			t.Fatalf("Finish did not return for a stream of %d bytes", len(stream))
		}
		testutil.Receive(t, drained, "the node to close what it sends")
		_ = reading.Close() //nolint:errcheck // the fuzz's own end
		if account.ServedBytes > fuzzBudget {
			t.Errorf("served %d bytes past the budget of %d", account.ServedBytes, fuzzBudget)
		}
		if killed.Load() != (account.Failure != nil) {
			t.Errorf("killed %v, but the account's failure is %v", killed.Load(), account.Failure)
		}
	})
}

// fuzzSeeds are streams FFmpeg's side might send: a whole session that keeps to the protocol, and
// one of each way to break it, from which the fuzzer mutates.
func fuzzSeeds() [][]byte {
	hello := frame(typeHello, 0, helloBody(versionRange{lowest: 1, highest: 1}))
	offer := frame(typeCapability, 1, put(put(put([]byte(nil), int32(256)), int32(1024)), int8(2)))
	opening := append(append([]byte(nil), hello...), offer...)
	session := append([]byte(nil), opening...)
	sequence := int8(2)
	for offset := int64(0); offset < 1000; offset += 256 {
		length := int32(min(256, 1000-offset))
		session = append(session, frame(typeRead, sequence, put(put([]byte(nil), offset), length))...)
		sequence++
	}
	session = append(session, frame(typeDone, sequence, put(put([]byte(nil), int64(7)), int8(resultOK)))...)
	return [][]byte{
		session,
		opening,
		nil,
		hello[:3],
		append(append([]byte(nil), opening...), frame(typeRead, 2, put(put([]byte(nil), int64(900)), int32(200)))...),
		append(append([]byte(nil), opening...), frame(typeRead, 9, put(put([]byte(nil), int64(0)), int32(256)))...),
		append(append([]byte(nil), opening...), frame(typeDone, 2, []byte{7, 0, 0, 0, 0, 0, 0, 0, 1, 2})...),
		frame(typeHello, 0, helloBody(versionRange{lowest: 2, highest: 3})),
	}
}
