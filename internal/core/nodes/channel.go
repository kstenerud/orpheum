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
	maxRecordBytes  = 32 << 20
	maxBatchRecords = 256
	maxBatchAge     = time.Second
)

// serveCommands answers a long-poll: the commands numbered after the one the node names, as soon
// as there are any, or none once pollHold has passed. Naming a number acknowledges every command up
// to it, and those are dropped.
func (service *Service) serveCommands(writer http.ResponseWriter, request *http.Request) {
	nodeID, ok := service.knownNodeIDOf(writer, request)
	if !ok {
		return
	}
	ackedSeq, err := strconv.ParseInt(request.URL.Query().Get(nodeproto.QueryAfterCommandSeq), 10, 64)
	if err != nil {
		http.Error(writer, "commands?"+nodeproto.QueryAfterCommandSeq+"=<the last command's seq, or 0>",
			http.StatusBadRequest)
		return
	}
	service.notePoll(nodeID, 1)
	defer service.notePoll(nodeID, -1)
	hold := service.clock.NewTimer(service.pollHold)
	defer hold.Stop()
	answer := func(ctx context.Context, pending []nodeproto.Command) {
		// Each answer carries the reader limits as the settings say now, so a change reaches the
		// node within a poll's hold (D240 §5).
		limits, err := service.readerLimits(ctx, service.devicesOf(nodeID))
		if err != nil {
			service.logger.WarnContext(ctx, "could not read the devices' reader limits; the node keeps its last",
				"nodeId", nodeID, "err", err)
		}
		writeJSON(writer, nodeproto.Commands{Commands: pending, DeviceReaderLimits: limits})
	}
	for {
		pending, wake := service.dropAckedAndListPending(nodeID, ackedSeq)
		if len(pending) > 0 {
			answer(request.Context(), pending)
			return
		}
		select {
		case <-wake:
		case <-hold.C():
			answer(request.Context(), []nodeproto.Command{})
			return
		case <-request.Context().Done():
			return
		}
	}
}

// dropAckedAndListPending drops the commands a node has acknowledged and returns those after
// ackedSeq, with the channel that is closed when another is queued.
func (service *Service) dropAckedAndListPending(nodeID, ackedSeq int64) ([]nodeproto.Command, <-chan struct{}) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	node := service.connectedNodes[nodeID]
	kept := node.commands[:0]
	for _, command := range node.commands {
		if command.CommandSeq > ackedSeq {
			kept = append(kept, command)
		}
	}
	node.commands = kept
	return append([]nodeproto.Command(nil), kept...), node.wake
}

// serveReport reads a node's JSONL report as it arrives — the core may read a report before it is
// complete (scanner.md) — and hands it to the sink in batches: when a batch is full, is old, or
// the stream has gone quiet, so the library shows what a scan found as it finds it. It records
// the last sequence number ingested and the node's latest status.
func (service *Service) serveReport(writer http.ResponseWriter, request *http.Request) {
	nodeID, ok := service.knownNodeIDOf(writer, request)
	if !ok {
		return
	}
	ctx := request.Context()
	reader := &reportReader{service: service, nodeID: nodeID}
	// A record is a line of JSON, most of them a file's, well under 64 KiB: the buffer reads many at
	// a time, and readLine takes a longer one, up to maxRecordBytes, across reads.
	body := bufio.NewReaderSize(request.Body, 64<<10)
	for {
		line, readErr := readLine(body, maxRecordBytes)
		if len(line) > 0 {
			if err := reader.takeLine(ctx, line); err != nil {
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

// maxResultBytes bounds a result. The largest is an artwork re-encode's: seven sizes of our own
// WebP, the largest 3840×2160, in base64; real ones are a few megabytes. A subtitle's is its text
// twice, each at most the node's own bound of 16 MiB, in base64; a prepare report and a loudness
// measurement are a few numbers.
const maxResultBytes = 64 << 20

// takeResult reads a known node's word on how a job ended, at most maxResultBytes of JSON, and hands
// it to the result sink, which records it; its answer is the node's acknowledgement (D234 §5).
// With nowhere to hand it, or a sink that could not record it, the result is refused, and the node
// sends it again.
func (service *Service) takeResult(writer http.ResponseWriter, request *http.Request) {
	nodeID, result, ok := readPost[nodeproto.Result](service, writer, request, maxResultBytes)
	if !ok {
		return
	}
	var err error
	if result.Held != nil {
		result, err = service.readHeld(request.Context(), nodeID, result)
	}
	if err == nil {
		err = service.resultSink.Deliver(request.Context(), nodeID, result)
	}
	switch {
	case errors.Is(err, ErrMalformedResult):
		http.Error(writer, err.Error(), http.StatusBadRequest)
	case err != nil:
		service.fail(request.Context(), writer, "record a node's result", err)
	default:
		writeJSON(writer, struct{}{}) // the node reads a JSON answer
	}
}

// readHeld fetches a result the node holds, too large to post, which its post named.
func (service *Service) readHeld(ctx context.Context, nodeID int64, posted nodeproto.Result) (nodeproto.Result, error) {
	if service.heldResults == nil {
		return nodeproto.Result{}, errors.New("this core cannot fetch a held result")
	}
	result, err := service.heldResults.ReadHeldResult(ctx, nodeID, posted)
	if err != nil {
		return nodeproto.Result{}, fmt.Errorf("take node %d's held result: %w", nodeID, err)
	}
	return result, nil
}

// maxProgressBytes bounds a progress post: a few numbers.
const maxProgressBytes = 4 << 10

// takeProgress reads a known node's word on where an attempt is, at most maxProgressBytes of JSON,
// and hands it to the job waiting for it. It is best effort: one nothing waits for is dropped.
func (service *Service) takeProgress(writer http.ResponseWriter, request *http.Request) {
	if nodeID, progress, ok := readPost[nodeproto.Progress](service, writer, request, maxProgressBytes); ok {
		service.resultSink.DeliverProgress(request.Context(), nodeID, progress)
		writeJSON(writer, struct{}{}) // the node reads a JSON answer
	}
}

// readPost reads a known node's post to the result sink, at most maxBytes of JSON, and notes that
// the node was heard. It answers the node itself, and reports false, for an unknown node, a core
// with no result sink, or a body that is not the JSON it should be.
func readPost[Body any](
	service *Service, writer http.ResponseWriter, request *http.Request, maxBytes int64,
) (int64, Body, bool) {
	var body Body
	nodeID, ok := service.knownNodeIDOf(writer, request)
	if !ok {
		return 0, body, false
	}
	service.noteHeard(nodeID)
	if service.resultSink == nil {
		http.Error(writer, "this core takes no results", http.StatusServiceUnavailable)
		return 0, body, false
	}
	if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxBytes)).Decode(&body); err != nil {
		http.Error(writer, "the body is not the JSON it should be: "+err.Error(), http.StatusBadRequest)
		return 0, body, false
	}
	return nodeID, body, true
}

// readLine reads one line without its newline, refusing one longer than maxBytes. At the end of the
// stream it returns what is left with io.EOF.
func readLine(reader *bufio.Reader, maxBytes int) ([]byte, error) {
	var line []byte
	for {
		chunk, err := reader.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > maxBytes {
			return nil, fmt.Errorf("a report line is over %d bytes", maxBytes)
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return bytes.TrimSuffix(line, []byte("\n")), err
		}
	}
}

// reportReader is one report stream in progress.
type reportReader struct {
	service        *Service
	nodeID         int64
	lastReportSeq  int64
	batch          []nodeproto.Record
	batchStartedAt time.Time
	status         *nodeproto.Status
}

// takeLine decodes one record, refusing one out of sequence, and flushes the batch when it is full
// or old enough that the library should show what it holds.
func (reader *reportReader) takeLine(ctx context.Context, line []byte) error {
	if len(line) == 0 {
		return nil
	}
	var record nodeproto.Record
	if err := json.Unmarshal(line, &record); err != nil {
		return fmt.Errorf("a report line is not a record: %w", err)
	}
	if reader.lastReportSeq != 0 && record.ReportSeq <= reader.lastReportSeq {
		return fmt.Errorf("record %d came after %d; a report's sequence only rises", record.ReportSeq,
			reader.lastReportSeq)
	}
	reader.lastReportSeq = record.ReportSeq
	reader.service.noteHeard(reader.nodeID)
	if record.Type == nodeproto.RecordStatus && record.Status != nil {
		reader.status = record.Status
		reader.service.keepStatus(ctx, reader.nodeID, record.Status)
	}
	now := reader.service.clock.Now()
	if len(reader.batch) == 0 {
		reader.batchStartedAt = now
	}
	reader.batch = append(reader.batch, record)
	if len(reader.batch) >= maxBatchRecords || now.Sub(reader.batchStartedAt) >= maxBatchAge {
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
	if err := reader.service.recordSink.IngestRecords(ctx, reader.nodeID, reader.batch); err != nil {
		return fmt.Errorf("ingest: %w", err)
	}
	lastBatchSeq := reader.batch[len(reader.batch)-1].ReportSeq
	reader.batch = reader.batch[:0]
	status := reader.status
	reader.status = nil
	return reader.service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		err := queries.UpdateNodeLastIngestedReportSeq(ctx, sqlcgen.UpdateNodeLastIngestedReportSeqParams{
			LastIngestedReportSeq: lastBatchSeq,
			ID:                    reader.nodeID,
		})
		if err != nil || status == nil {
			return err
		}
		encoded, err := json.Marshal(status)
		if err != nil {
			return fmt.Errorf("encode the status: %w", err)
		}
		text := string(encoded)
		statusAt := reader.service.clock.Now().UnixMilli()
		return queries.UpdateNodeStatus(ctx, sqlcgen.UpdateNodeStatusParams{
			LastStatusJSON: &text, LastStatusAt: &statusAt, ID: reader.nodeID,
		})
	})
}
