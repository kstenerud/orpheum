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

	work        *workList
	answerRetry answerRetry

	mutex    sync.Mutex
	activity string
}

// New returns an agent that talks to the core at socketPath. The hello is sent as given, with
// the roots' paths and this build's protocol version filled in.
func New(
	socketPath string, hello nodeproto.Hello, roots []*confine.MediaRoot, workers Workers, clockSource clock.Clock,
	logger *slog.Logger,
) *Agent {
	byPath := make(map[string]*confine.MediaRoot, len(roots))
	hello.ProtocolVersion = nodeproto.ProtocolVersion
	hello.RootPaths = make([]string, 0, len(roots))
	for _, root := range roots {
		byPath[root.Path()] = root
		hello.RootPaths = append(hello.RootPaths, root.Path())
	}
	return &Agent{
		client: &http.Client{Transport: unixsocket.Transport(socketPath)}, hello: hello, roots: byPath,
		workers: workers,
		clock:   clockSource, logger: logger, activity: "idle", work: newWorkList(hello.Capacity),
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
	// back for the next while the session starts the last scan. Only its size is chosen: a full
	// buffer holds the poll, never drops a command.
	scanQueue := make(chan nodeproto.Scan, 16)
	var sessionTasks sync.WaitGroup
	sessionTasks.Go(func() { cancel(agent.streamReport(ctx, reply.NodeID, stream)) })
	sessionTasks.Go(func() { cancel(agent.pollCommands(ctx, nodeCtx, reply.NodeID, scanQueue)) })
	sessionTasks.Go(func() { cancel(agent.runScansInOrder(ctx, scanQueue, stream)) })
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
				for _, waiting := range agent.work.waitingPrepares() {
					agent.sayWaiting(ctx, reply.NodeID, waiting)
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

// pollCommands long-polls for commands. Work is held by the node, beside its scans, which are
// queued for the session to run one at a time; a cancel is carried out as it arrives.
func (agent *Agent) pollCommands(
	ctx, nodeCtx context.Context, nodeID int64, scanQueue chan<- nodeproto.Scan,
) error {
	var lastTakenCommandSeq int64
	for {
		var answer nodeproto.Commands
		path := nodeproto.PathCommands + "?" + nodeproto.QueryAfterCommandSeq + "=" +
			strconv.FormatInt(lastTakenCommandSeq, 10)
		if err := agent.exchange(ctx, http.MethodGet, path, nodeID, http.NoBody, &answer); err != nil {
			return err
		}
		for _, command := range answer.Commands {
			switch {
			case command.Type == nodeproto.CommandCancel:
				if !agent.work.cancel(command.AttemptToken) {
					agent.logger.InfoContext(ctx, "a cancel came for an attempt this node does not hold", "jobId",
						command.JobID, "attemptToken", command.AttemptToken)
				}
			case command.Type == nodeproto.CommandScan && command.Scan != nil:
				select {
				case scanQueue <- *command.Scan:
				case <-ctx.Done():
					return fmt.Errorf("stopped while queueing command %d: %w", command.CommandSeq, ctx.Err())
				}
			default:
				agent.holdCommand(ctx, nodeCtx, nodeID, command)
			}
			lastTakenCommandSeq = command.CommandSeq
		}
	}
}

// holdCommand holds a command's work until a slot of its kind is free (workList), on nodeCtx, the
// node's lifetime: a session that ends, the core gone or restarting, stops no work (D233 §2), and
// each posts its result when a core takes it. A command the node cannot read is refused.
func (agent *Agent) holdCommand(ctx, nodeCtx context.Context, nodeID int64, command nodeproto.Command) {
	held := attempt{jobID: command.JobID, token: command.AttemptToken}
	work, isKnown := agent.workFor(nodeCtx, nodeID, held, command)
	if !isKnown || held.token == "" {
		agent.refuseUnknown(ctx, nodeID, command)
		return
	}
	work.attempt, work.priority = held, command.Priority
	isHeld, isWaiting := agent.work.hold(nodeCtx, work)
	if !isHeld {
		agent.logger.InfoContext(ctx, "the core sent again an attempt this node holds", "jobId", held.jobID,
			"attemptToken", held.token)
		return
	}
	if isWaiting && work.saysWaiting {
		go agent.sayWaiting(nodeCtx, nodeID, held)
	}
}

// workFor is the work a command asks for, and false for one this node cannot read: unknown, or
// without its payload. One without an attempt token is refused too, by holdCommand.
func (agent *Agent) workFor(
	nodeCtx context.Context, nodeID int64, held attempt, command nodeproto.Command,
) (heldWork, bool) {
	switch {
	case command.Type == nodeproto.CommandPrepare && command.Prepare != nil:
		return agent.prepareWork(nodeCtx, nodeID, held, *command.Prepare), true
	case command.Type == nodeproto.CommandExtractSubtitle && command.ExtractSubtitle != nil:
		return agent.subtitleWork(nodeCtx, nodeID, held, *command.ExtractSubtitle), true
	case command.Type == nodeproto.CommandMeasureLoudness && command.MeasureLoudness != nil:
		return agent.loudnessWork(nodeCtx, nodeID, held, *command.MeasureLoudness), true
	case command.Type == nodeproto.CommandEncodeArtwork && command.EncodeArtwork != nil:
		return agent.artworkWork(nodeCtx, nodeID, held, *command.EncodeArtwork), true
	}
	return heldWork{}, false
}

// runScansInOrder runs scans one at a time, until the session's ctx ends.
func (agent *Agent) runScansInOrder(ctx context.Context, scanQueue <-chan nodeproto.Scan, stream *recordStream) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("the scans stopped: %w", ctx.Err())
		case queued := <-scanQueue:
			agent.runScan(ctx, queued, stream)
		}
	}
}

// runScan scans one root. A root the node was not configured with is refused, whatever the core
// asks (D107).
func (agent *Agent) runScan(ctx context.Context, command nodeproto.Scan, stream *recordStream) {
	root, known := agent.roots[command.RootPath]
	if !known {
		agent.logger.WarnContext(ctx, "refused to scan a root this node was not given", "rootPath", command.RootPath)
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
}

// refuseUnknown answers a command this node does not know, or whose payload or attempt token is
// missing, from a core of another build: the job it is for is refused, so the core sees the node needs updating rather
// than waiting on silence (D233 §9). A command for no job is logged.
func (agent *Agent) refuseUnknown(ctx context.Context, nodeID int64, command nodeproto.Command) {
	agent.logger.WarnContext(ctx, "refused a command this node does not know", "commandType", command.Type,
		"jobId", command.JobID)
	if command.JobID == 0 {
		return
	}
	refusal := fmt.Sprintf("this node cannot read the command %q, unknown or without its payload; "+
		"it speaks node protocol %d", command.Type, nodeproto.ProtocolVersion)
	refused := attempt{jobID: command.JobID, token: command.AttemptToken}.named(nodeproto.Result{Refusal: refusal})
	go agent.postResult(ctx, nodeID, "a refusal", refused)
}

// artworkWork re-encodes an image and posts the answer — the sizes, or why there are none — to
// the core, whose artwork store takes images from nowhere else (D212 §8).
func (agent *Agent) artworkWork(ctx context.Context, nodeID int64, held attempt, command nodeproto.Artwork) heldWork {
	return heldWork{slot: slotArtwork, run: func(jobCtx context.Context) {
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
		if jobCtx.Err() != nil && ctx.Err() == nil {
			agent.logger.InfoContext(ctx, "an artwork re-encode was cancelled", "jobId", held.jobID)
			return
		}
		if result.Refusal != "" {
			agent.logger.WarnContext(ctx, "refused an image", "jobId", held.jobID, "refusal", result.Refusal)
		}
		agent.postResult(ctx, nodeID, "an artwork re-encode", held.named(nodeproto.Result{Artwork: &result}))
	}}
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
