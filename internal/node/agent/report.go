// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// recordStream numbers records and hands them to the stream. Numbering continues from the last record
// the core ingested, which the hello reply names, so a reconnected node never reuses a number.
type recordStream struct {
	mutex         sync.Mutex
	lastReportSeq int64
	records       chan nodeproto.Record
}

func newRecordStream(lastReportSeq int64) *recordStream {
	return &recordStream{lastReportSeq: lastReportSeq, records: make(chan nodeproto.Record)}
}

// emit numbers record and waits until the stream has taken it. Numbering and sending happen
// under one lock, so records reach the core in the order of their numbers.
func (stream *recordStream) emit(ctx context.Context, record nodeproto.Record) error {
	stream.mutex.Lock()
	defer stream.mutex.Unlock()
	record.ReportSeq = stream.lastReportSeq + 1
	select {
	case stream.records <- record:
		stream.lastReportSeq = record.ReportSeq
		return nil
	case <-ctx.Done():
		return fmt.Errorf("record not sent: %w", ctx.Err())
	}
}
