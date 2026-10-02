// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/node/scan"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/diskspace"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
)

// nodeHeader names the node on every request after its hello; the core's nodes.NodeIDHeader.
const nodeHeader = "Orpheum-Node-ID"

// coreURL is the base of every request. The host is a placeholder: the dialer ignores it and
// connects to the core's socket.
const coreURL = "http://orpheum-core"

// Scanner walks one media root; scan.Scanner is the real one.
type Scanner interface {
	Scan(ctx context.Context, root *confine.MediaRoot, emit scan.Emit) error
}

// Artworker re-encodes an image; artwork.Deriver is the real one (D212).
type Artworker interface {
	Derive(ctx context.Context, command nodeproto.Artwork) nodeproto.ArtworkResult
}

// Workers are what the agent runs commands with.
type Workers struct {
	Scanner   Scanner
	Artworker Artworker
	// Preparer is nil on a node that does not prepare.
	Preparer Preparer
	// Subtitler is nil on a node that does not extract subtitles.
	Subtitler Subtitler
	// LoudnessMeasurer is nil on a node that does not measure loudness.
	LoudnessMeasurer Measurer
	// PreparedShelf keeps the prepared directory within its budget; nil on a node that does not
	// prepare.
	PreparedShelf PreparedShelf
	// PreparedDirectory is where renditions are written, whose disk the status reports; empty on
	// a node that does not prepare.
	PreparedDirectory string
}

// PreparedShelf keeps the prepared directory within its budget: prepared.Shelf (D217 §5).
type PreparedShelf interface {
	Evict() (prepared.Usage, error)
	LastUsage() (prepared.Usage, bool)
}

// Agent is the node's loop (D207): it dials the core's socket, says hello, long-polls for
// commands and runs them one at a time, prepares, extractions and measurements beside them, and
// streams its report — scan records and a status record every tick — as one JSONL request. When
// the report or the poll breaks it waits for the next tick and says hello again.
type Agent struct {
	client  *http.Client
	hello   nodeproto.Hello
	roots   map[string]*confine.MediaRoot
	workers Workers
	clock   clock.Clock
	logger  *slog.Logger

	slots       slots
	jobs        jobTracker
	answerRetry answerRetry

	mutex    sync.Mutex
	activity string
}

// New returns an agent that talks to the core at socketPath. The hello is sent as given, with
// the roots' paths filled in.
func New(
	socketPath string, hello nodeproto.Hello, roots []*confine.MediaRoot, workers Workers, clockSource clock.Clock,
	logger *slog.Logger,
) *Agent {
	byPath := make(map[string]*confine.MediaRoot, len(roots))
	hello.RootPaths = make([]string, 0, len(roots))
	for _, root := range roots {
		byPath[root.Path()] = root
		hello.RootPaths = append(hello.RootPaths, root.Path())
	}
	return &Agent{
		client: &http.Client{Transport: unixsocket.Transport(socketPath)}, hello: hello, roots: byPath,
		workers: workers,
		clock:   clockSource, logger: logger, activity: "idle", slots: newSlots(hello.Capacity),
		answerRetry: defaultAnswerRetry(),
	}
}

// Run keeps a session with the core until ctx ends. Each tick sends a status record, and after
// a failed session the next tick starts another.
func (agent *Agent) Run(ctx context.Context, tick <-chan time.Time) error {
	for {
		err := agent.session(ctx, tick)
		if ctx.Err() != nil {
			return nil //nolint:nilerr // a session always fails when the node is told to stop
		}
		agent.logger.WarnContext(ctx, "lost the core; saying hello again at the next tick", "err", err)
		select {
		case <-ctx.Done():
			return nil
		case <-tick:
		}
	}
}

// session is one hello and everything until something fails. The work it is given runs on ctx,
// the node's own lifetime, not the session's: a session that ends, the core gone or restarting,
// stops no prepare, extraction, measurement or re-encode (D233 §2), and each posts its result when
// a core takes it.
func (agent *Agent) session(ctx context.Context, tick <-chan time.Time) error {
	nodeCtx := ctx
	reply, err := agent.sayHello(ctx)
	if err != nil {
		return err
	}
	agent.logger.InfoContext(ctx, "said hello to the core", "nodeId", reply.NodeID, "lastIngestedReportSeq",
		reply.LastIngestedReportSeq)
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	stream := newRecordStream(reply.LastIngestedReportSeq)
	// The long-poll hands over a batch of commands at a time; a buffer of a batch or so lets it go
	// back for the next while the session starts the last. Only its size is chosen: a full buffer
	// holds the poll, never drops a command.
	commandQueue := make(chan nodeproto.Command, 16)
	var sessionTasks sync.WaitGroup
	sessionTasks.Go(func() { cancel(agent.streamReport(ctx, reply.NodeID, stream)) })
	sessionTasks.Go(func() { cancel(agent.pollCommands(ctx, reply.NodeID, commandQueue)) })
	sessionTasks.Go(func() {
		cancel(agent.runCommandsInOrder(ctx, nodeCtx, reply.NodeID, commandQueue, stream))
	})
	sessionTasks.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick:
				if err := stream.emit(ctx, agent.status()); err != nil {
					cancel(err)
					return
				}
				for _, jobID := range agent.jobs.waitingJobIDs() {
					agent.sayWaiting(ctx, reply.NodeID, jobID)
				}
			}
		}
	})
	sessionTasks.Wait()
	return context.Cause(ctx)
}

func (agent *Agent) sayHello(ctx context.Context) (nodeproto.HelloReply, error) {
	encoded, err := json.Marshal(agent.hello)
	if err != nil {
		return nodeproto.HelloReply{}, fmt.Errorf("encode the hello: %w", err)
	}
	var reply nodeproto.HelloReply
	err = agent.exchange(ctx, http.MethodPost, nodeproto.PathHello, 0, bytes.NewReader(encoded), &reply)
	return reply, err
}

// pollCommands long-polls for commands and queues them for the worker.
func (agent *Agent) pollCommands(ctx context.Context, nodeID int64, commandQueue chan<- nodeproto.Command) error {
	var lastTakenCommandSeq int64
	for {
		var answer nodeproto.Commands
		path := nodeproto.PathCommands + "?" + nodeproto.QueryAfterCommandSeq + "=" +
			strconv.FormatInt(lastTakenCommandSeq, 10)
		if err := agent.exchange(ctx, http.MethodGet, path, nodeID, http.NoBody, &answer); err != nil {
			return err
		}
		for _, command := range answer.Commands {
			// A cancel is carried out as it arrives, not queued behind the work it stops.
			if command.Type == nodeproto.CommandCancel {
				if !agent.jobs.cancel(command.JobID) {
					agent.logger.InfoContext(ctx, "a job was cancelled before it started", "jobId", command.JobID)
				}
				lastTakenCommandSeq = command.CommandSeq
				continue
			}
			select {
			case commandQueue <- command:
			case <-ctx.Done():
				return fmt.Errorf("stopped while queueing command %d: %w", command.CommandSeq, ctx.Err())
			}
			lastTakenCommandSeq = command.CommandSeq
		}
	}
}

// runCommandsInOrder runs commands one at a time, until the session's ctx ends; the work they start
// runs on nodeCtx, the node's lifetime.
func (agent *Agent) runCommandsInOrder(
	ctx, nodeCtx context.Context, nodeID int64, commandQueue <-chan nodeproto.Command, stream *recordStream,
) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("worker stopped: %w", ctx.Err())
		case command := <-commandQueue:
			agent.runCommand(ctx, nodeCtx, nodeID, command, stream)
		}
	}
}

// runCommand carries out one command. A scan of a root the node was not configured with is
// refused, whatever the core asks (D107).
func (agent *Agent) runCommand(
	ctx, nodeCtx context.Context, nodeID int64, command nodeproto.Command, stream *recordStream,
) {
	if command.Type == nodeproto.CommandEncodeArtwork && command.EncodeArtwork != nil {
		agent.deriveAndPostArtwork(nodeCtx, nodeID, command.JobID, *command.EncodeArtwork)
		return
	}
	if command.Type == nodeproto.CommandPrepare && command.Prepare != nil {
		agent.startPrepare(nodeCtx, nodeID, command.JobID, *command.Prepare)
		return
	}
	if command.Type == nodeproto.CommandExtractSubtitle && command.ExtractSubtitle != nil {
		agent.extractSubtitle(nodeCtx, nodeID, command.JobID, *command.ExtractSubtitle)
		return
	}
	if command.Type == nodeproto.CommandMeasureLoudness && command.MeasureLoudness != nil {
		agent.measureLoudness(nodeCtx, nodeID, command.JobID, *command.MeasureLoudness)
		return
	}
	if command.Type == nodeproto.CommandScan {
		root, known := agent.roots[command.Scan.RootPath]
		if !known {
			agent.logger.WarnContext(ctx, "refused to scan a root this node was not given", "rootPath",
				command.Scan.RootPath)
			return
		}
		agent.setActivity("scanning " + root.Path())
		defer agent.setActivity("idle")
		emit := func(record nodeproto.Record) error {
			// Once a scan says it is complete, the node is idle: a status record sent after the
			// completion must not still say it is scanning.
			if record.Type == nodeproto.RecordScanComplete {
				agent.setActivity("idle")
			}
			return stream.emit(ctx, record)
		}
		err := agent.workers.Scanner.Scan(ctx, root, emit)
		if err != nil && ctx.Err() == nil {
			agent.logger.WarnContext(ctx, "a scan ended early", "rootPath", root.Path(), "err", err)
		}
		return
	}
	agent.logger.WarnContext(ctx, "ignored a command this node does not know", "commandType", command.Type)
}

// deriveAndPostArtwork re-encodes an image and posts the answer — the sizes, or why there are
// none — to the core, whose artwork store takes images from nowhere else (D212 §8). The answer is
// posted beside the node's other work, so a core slow to take it holds up no other command.
func (agent *Agent) deriveAndPostArtwork(ctx context.Context, nodeID, jobID int64, command nodeproto.Artwork) {
	agent.setActivity("re-encoding artwork")
	defer agent.setActivity("idle")
	jobCtx, finishJob := agent.jobs.startJob(ctx, jobID)
	defer finishJob()
	var result nodeproto.ArtworkResult
	sourceHash := ""
	if command.AbsolutePath != nil {
		source, err := agent.readImage(*command.AbsolutePath)
		if err == nil {
			sum := sha256.Sum256(source)
			command.SourceImage, command.AbsolutePath, sourceHash = source, nil, hex.EncodeToString(sum[:])
		} else {
			result = nodeproto.ArtworkResult{Refusal: err.Error()}
		}
	}
	if result.Refusal == "" {
		result = agent.workers.Artworker.Derive(jobCtx, command)
		result.SourceSha256 = sourceHash
	}
	if result.Refusal != "" {
		agent.logger.WarnContext(ctx, "refused an image", "jobId", jobID, "refusal", result.Refusal)
	}
	go agent.postResult(ctx, nodeID, "an artwork re-encode", nodeproto.Result{JobID: jobID, Artwork: &result})
}

// maxImageBytes bounds an image beside the media the node reads: posters are megabytes.
const maxImageBytes = 64 << 20

// readImage reads an image beside the media through the root it is under, so no path the core
// names can reach outside the node's roots (D107).
func (agent *Agent) readImage(named nodeproto.Path) ([]byte, error) {
	absolute := named.Text
	if named.Raw != nil {
		absolute = string(named.Raw)
	}
	var holder *confine.MediaRoot
	holderRelativePath := ""
	for _, root := range agent.roots {
		relativePath, inside := root.RelativeIfInside(absolute)
		// The deepest root holds it when roots nest, whatever order the map gives them in.
		if inside && relativePath != "." && (holder == nil || len(relativePath) < len(holderRelativePath)) {
			holder, holderRelativePath = root, relativePath
		}
	}
	if holder == nil {
		return nil, errors.New("the image is under none of this node's roots")
	}
	return holder.ReadFile(holderRelativePath, maxImageBytes)
}

func (agent *Agent) setActivity(activity string) {
	agent.mutex.Lock()
	defer agent.mutex.Unlock()
	agent.activity = activity
}

func (agent *Agent) status() nodeproto.Record {
	disks := agent.disks()
	agent.mutex.Lock()
	defer agent.mutex.Unlock()
	status := &nodeproto.Status{SentAt: agent.clock.Now().UnixMilli(), Activity: agent.activity, Disks: disks}
	if agent.workers.PreparedShelf != nil {
		if usage, known := agent.workers.PreparedShelf.LastUsage(); known {
			status.PreparedUsage = &nodeproto.PreparedUsage{
				UsedBytes: usage.UsedBytes, BudgetBytes: usage.BudgetBytes, DiskUsedPercent: usage.DiskUsedPercent,
				DiskUsedLimitPercent: usage.DiskUsedLimitPercent,
			}
		}
	}
	return nodeproto.Record{Type: nodeproto.RecordStatus, Status: status}
}

// disks are the filesystems the node's roots and prepared directory are on, each measured now
// (D225 §3).
func (agent *Agent) disks() []nodeproto.Disk {
	paths := slices.Clone(agent.hello.RootPaths)
	if agent.workers.PreparedDirectory != "" {
		paths = append(paths, agent.workers.PreparedDirectory)
	}
	disks := make([]nodeproto.Disk, 0, len(paths))
	for _, path := range paths {
		disk := nodeproto.Disk{Path: path}
		if space, err := diskspace.Measure(path); err != nil {
			disk.Failure = err.Error()
		} else {
			disk.TotalBytes, disk.AvailableBytes = space.TotalBytes, space.AvailableBytes
		}
		disks = append(disks, disk)
	}
	return disks
}

// exchange makes one request and decodes its JSON answer.
func (agent *Agent) exchange(ctx context.Context, method, path string, nodeID int64, body io.Reader, into any) error {
	request, err := http.NewRequestWithContext(ctx, method, coreURL+path, body)
	if err != nil {
		return fmt.Errorf("build %s %s: %w", method, path, err)
	}
	if nodeID != 0 {
		request.Header.Set(nodeHeader, strconv.FormatInt(nodeID, 10))
	}
	response, err := agent.client.Do(request)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read-only, and decoded below
	if response.StatusCode != http.StatusOK {
		text, _ := io.ReadAll(io.LimitReader(response.Body, outbound.BodyExcerptBytes)) //nolint:errcheck // best effort
		return fmt.Errorf("%s %s: %s: %s", method, path, response.Status, bytes.TrimSpace(text))
	}
	if err := json.NewDecoder(response.Body).Decode(into); err != nil {
		return fmt.Errorf("%s %s: the answer is not JSON: %w", method, path, err)
	}
	return nil
}

// streamReport sends the report as one request whose body is written as records arrive. The client
// sends each write as its own chunk, so the core reads a record as soon as it is written.
func (agent *Agent) streamReport(ctx context.Context, nodeID int64, stream *recordStream) error {
	reader, writer := io.Pipe()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, coreURL+nodeproto.PathReport, reader)
	if err != nil {
		return fmt.Errorf("build the report request: %w", err)
	}
	request.Header.Set(nodeHeader, strconv.FormatInt(nodeID, 10))
	request.Header.Set("Content-Type", "application/jsonl")
	reportRequestEnded := make(chan error, 1)
	go func() {
		response, err := agent.client.Do(request)
		if err == nil {
			err = fmt.Errorf("the core ended the report: %s", response.Status)
			_ = response.Body.Close() //nolint:errcheck // nothing to read
		}
		reportRequestEnded <- err
		_ = reader.CloseWithError(err)
	}()
	encoder := json.NewEncoder(writer)
	for {
		select {
		case <-ctx.Done():
			_ = writer.Close() //nolint:errcheck // always nil
			return errors.Join(fmt.Errorf("report stopped: %w", ctx.Err()), <-reportRequestEnded)
		case record := <-stream.records:
			if err := encoder.Encode(record); err != nil {
				return fmt.Errorf("write record %d: %w", record.ReportSeq, errors.Join(err, <-reportRequestEnded))
			}
		}
	}
}
