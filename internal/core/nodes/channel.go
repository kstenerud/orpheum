// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Bounds on a report: one record per line, and a line no longer than a probe of a hostile file
// can make it (the node bounds its own records; this bounds the channel).
const (
	maxRecordBytes = 32 << 20
	batchRecords   = 256
	batchAge       = time.Second
)

// commands answers a long-poll: the commands numbered after the one the node names, as soon as
// there are any, or none once pollHold has passed. Naming a number acknowledges every command up
// to it, and those are dropped.
func (service *Service) commands(writer http.ResponseWriter, request *http.Request) {
	nodeID, ok := service.nodeID(writer, request)
	if !ok {
		return
	}
	after, err := strconv.ParseInt(request.URL.Query().Get("after"), 10, 64)
	if err != nil {
		http.Error(writer, "commands?after=<the last command's seq, or 0>", http.StatusBadRequest)
		return
	}
	service.heard(nodeID)
	timer := time.NewTimer(service.pollHold)
	defer timer.Stop()
	for {
		pending, wake := service.pending(nodeID, after)
		if len(pending) > 0 {
			writeJSON(writer, nodeproto.Commands{Commands: pending})
			return
		}
		select {
		case <-wake:
		case <-timer.C:
			writeJSON(writer, nodeproto.Commands{Commands: []nodeproto.Command{}})
			return
		case <-request.Context().Done():
			return
		}
	}
}

// pending drops the commands a node has acknowledged and returns those after it, with the
// channel that is closed when another is queued.
func (service *Service) pending(nodeID, after int64) ([]nodeproto.Command, <-chan struct{}) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	queue := service.channels[nodeID]
	kept := queue.commands[:0]
	for _, command := range queue.commands {
		if command.Seq > after {
			kept = append(kept, command)
		}
	}
	queue.commands = kept
	return append([]nodeproto.Command(nil), kept...), queue.wake
}

// report reads a node's JSONL report as it arrives — the core may read a report before it is
// complete (scanner.md) — and hands it to the sink in batches: when a batch is full, is old, or
// the stream has gone quiet, so the library shows what a scan found as it finds it. It records
// the last sequence number ingested and the node's latest status.
func (service *Service) report(writer http.ResponseWriter, request *http.Request) {
	nodeID, ok := service.nodeID(writer, request)
	if !ok {
		return
	}
	ctx := request.Context()
	reader := &reportReader{service: service, nodeID: nodeID}
	body := bufio.NewReaderSize(request.Body, 64<<10)
	for {
		line, readErr := readLine(body, maxRecordBytes)
		if len(line) > 0 {
			if err := reader.line(ctx, line); err != nil {
				service.logger.WarnContext(ctx, "a node's report was refused", "nodeId", nodeID, "err", err)
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			if body.Buffered() == 0 {
				readErr = errors.Join(readErr, reader.flush(ctx))
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			// A node that hung up mid-line reconnects and continues; what was ingested stays.
			service.logger.InfoContext(ctx, "a node's report ended early", "nodeId", nodeID, "err",
				errors.Join(readErr, reader.flush(ctx)))
			return
		}
	}
	if err := reader.flush(ctx); err != nil {
		service.fail(ctx, writer, "ingest the report's end", err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

// maxArtworkBytes bounds an artwork answer: seven sizes of our own WebP, the largest 3840×2160,
// in base64. Real ones are a few megabytes.
const maxArtworkBytes = 64 << 20

// artworkAnswer takes a node's answer to an artwork command to the artwork store.
func (service *Service) artworkAnswer(writer http.ResponseWriter, request *http.Request) {
	nodeID, ok := service.nodeID(writer, request)
	if !ok {
		return
	}
	service.heard(nodeID)
	if service.artwork == nil {
		http.Error(writer, "this core takes no artwork", http.StatusServiceUnavailable)
		return
	}
	var result nodeproto.ArtworkResult
	body := http.MaxBytesReader(writer, request.Body, maxArtworkBytes)
	if err := json.NewDecoder(body).Decode(&result); err != nil {
		http.Error(writer, "an artwork answer is JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	service.artwork.Answered(request.Context(), nodeID, result)
	writeJSON(writer, struct{}{}) // the node reads a JSON answer
}

// readLine reads one line without its newline, refusing one longer than limit. At the end of the
// stream it returns what is left with io.EOF.
func readLine(reader *bufio.Reader, limit int) ([]byte, error) {
	var line []byte
	for {
		chunk, err := reader.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > limit {
			return nil, fmt.Errorf("a report line is over %d bytes", limit)
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return bytes.TrimSuffix(line, []byte("\n")), err
		}
	}
}

// reportReader is one report stream in progress.
type reportReader struct {
	service   *Service
	nodeID    int64
	lastSeq   int64
	batch     []nodeproto.Record
	batchFrom time.Time
	status    *nodeproto.Status
}

// line decodes one record, refusing one out of sequence, and flushes the batch when it is full
// or old enough that the library should show what it holds.
func (reader *reportReader) line(ctx context.Context, line []byte) error {
	if len(line) == 0 {
		return nil
	}
	var record nodeproto.Record
	if err := json.Unmarshal(line, &record); err != nil {
		return fmt.Errorf("a report line is not a record: %w", err)
	}
	if reader.lastSeq != 0 && record.Seq <= reader.lastSeq {
		return fmt.Errorf("record %d came after %d; a report's sequence only rises", record.Seq, reader.lastSeq)
	}
	reader.lastSeq = record.Seq
	reader.service.heard(reader.nodeID)
	if record.Type == nodeproto.RecordStatus && record.Status != nil {
		reader.status = record.Status
	}
	now := reader.service.clock.Now()
	if len(reader.batch) == 0 {
		reader.batchFrom = now
	}
	reader.batch = append(reader.batch, record)
	if len(reader.batch) >= batchRecords || now.Sub(reader.batchFrom) >= batchAge {
		return reader.flush(ctx)
	}
	return nil
}

// flush hands the batch to the sink, then records how far the core has ingested and the node's
// latest status.
func (reader *reportReader) flush(ctx context.Context) error {
	if len(reader.batch) == 0 {
		return nil
	}
	if err := reader.service.sink.Records(ctx, reader.nodeID, reader.batch); err != nil {
		return fmt.Errorf("ingest: %w", err)
	}
	last := reader.batch[len(reader.batch)-1].Seq
	reader.batch = reader.batch[:0]
	status := reader.status
	reader.status = nil
	return reader.service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		err := queries.UpdateNodeReportSeq(ctx, sqlcgen.UpdateNodeReportSeqParams{ReportSeq: last, ID: reader.nodeID})
		if err != nil || status == nil {
			return err
		}
		encoded, err := json.Marshal(status)
		if err != nil {
			return fmt.Errorf("encode the status: %w", err)
		}
		text := string(encoded)
		at := reader.service.clock.Now().UnixMilli()
		return queries.UpdateNodeStatus(ctx, sqlcgen.UpdateNodeStatusParams{
			Status: &text, StatusAt: &at, ID: reader.nodeID,
		})
	})
}
