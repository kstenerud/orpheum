// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// report numbers records and hands them to the stream. Numbering continues from the last record
// the core ingested, which the hello reply names, so a reconnected node never reuses a number.
type report struct {
	mutex   sync.Mutex
	lastSeq int64
	records chan nodeproto.Record
}

func newReport(lastSeq int64) *report {
	return &report{lastSeq: lastSeq, records: make(chan nodeproto.Record)}
}

// emit numbers record and waits until the stream has taken it. Numbering and sending happen
// under one lock, so records reach the core in the order of their numbers.
func (stream *report) emit(ctx context.Context, record nodeproto.Record) error {
	stream.mutex.Lock()
	defer stream.mutex.Unlock()
	record.Seq = stream.lastSeq + 1
	select {
	case stream.records <- record:
		stream.lastSeq = record.Seq
		return nil
	case <-ctx.Done():
		return fmt.Errorf("record not sent: %w", ctx.Err())
	}
}
