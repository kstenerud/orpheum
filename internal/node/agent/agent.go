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
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/node/scan"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/diskspace"
)

// nodeHeader names the node on every request after its hello; the core's nodes.NodeHeader.
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
	Scanner Scanner
	Artwork Artworker
	// Preparer is nil on a node that does not prepare.
	Preparer Preparer
	// Subtitles is nil on a node that does not extract subtitles.
	Subtitles Subtitler
	// Loudness is nil on a node that does not measure loudness.
	Loudness Measurer
	// Keeper keeps the prepared directory within its budget; nil on a node that does not prepare.
	Keeper Keeper
	// PreparedDirectory is where renditions are written, whose disk the status reports; empty on
	// a node that does not prepare.
	PreparedDirectory string
}

// Keeper keeps the prepared directory within its budget: prepared.Shelf (D217 §5).
type Keeper interface {
	Evict() (prepared.Usage, error)
	LastUsage() (prepared.Usage, bool)
}

// Agent is the node's loop (D207): it dials the core's socket, says hello, long-polls for
// commands and runs them one at a time, prepares beside them, and streams its report — scan records and a status
// record every tick — as one JSONL request. When anything breaks it waits for the next tick and
// says hello again.
type Agent struct {
	client  *http.Client
	hello   nodeproto.Hello
	roots   map[string]*confine.MediaRoot
	workers Workers
	clock   clock.Clock
	logger  *slog.Logger

	slots   slots
	working working

	mutex sync.Mutex
	doing string
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
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &Agent{
		client: &http.Client{Transport: transport}, hello: hello, roots: byPath, workers: workers,
		clock: clockSource, logger: logger, doing: "idle", slots: newSlots(),
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

// session is one hello and everything until something fails.
func (agent *Agent) session(ctx context.Context, tick <-chan time.Time) error {
	reply, err := agent.sayHello(ctx)
	if err != nil {
		return err
	}
	agent.logger.InfoContext(ctx, "said hello to the core", "nodeId", reply.NodeID, "lastReportSeq",
		reply.LastIngestedReportSeq)
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	report := newReport(reply.LastIngestedReportSeq)
	work := make(chan nodeproto.Command, 16)
	var running sync.WaitGroup
	running.Go(func() { cancel(agent.stream(ctx, reply.NodeID, report)) })
	running.Go(func() { cancel(agent.poll(ctx, reply.NodeID, work)) })
	running.Go(func() { cancel(agent.work(ctx, reply.NodeID, work, report)) })
	running.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick:
				if err := report.emit(ctx, agent.status()); err != nil {
					cancel(err)
					return
				}
				for _, job := range agent.working.waitingJobs() {
					agent.sayWaiting(ctx, reply.NodeID, job)
				}
			}
		}
	})
	running.Wait()
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

// poll long-polls for commands and queues them for the worker.
func (agent *Agent) poll(ctx context.Context, nodeID int64, work chan<- nodeproto.Command) error {
	var after int64
	for {
		var answer nodeproto.Commands
		path := nodeproto.PathCommands + "?" + nodeproto.QueryAfterCommandSeq + "=" + strconv.FormatInt(after, 10)
		if err := agent.exchange(ctx, http.MethodGet, path, nodeID, http.NoBody, &answer); err != nil {
			return err
		}
		for _, command := range answer.Commands {
			// A cancel is carried out as it arrives, not queued behind the work it stops.
			if command.Type == nodeproto.CommandCancel && command.Cancel != nil {
				if !agent.working.cancel(command.Cancel.JobID) {
					agent.logger.InfoContext(ctx, "a job was cancelled before it started", "jobId",
						command.Cancel.JobID)
				}
				after = command.CommandSeq
				continue
			}
			select {
			case work <- command:
			case <-ctx.Done():
				return fmt.Errorf("stopped while queueing command %d: %w", command.CommandSeq, ctx.Err())
			}
			after = command.CommandSeq
		}
	}
}

// work runs commands one at a time.
func (agent *Agent) work(ctx context.Context, nodeID int64, work <-chan nodeproto.Command, report *report) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("worker stopped: %w", ctx.Err())
		case command := <-work:
			if err := agent.run(ctx, nodeID, command, report); err != nil {
				return err
			}
		}
	}
}

// run carries out one command. A scan of a root the node was not configured with is refused,
// whatever the core asks (D107).
func (agent *Agent) run(ctx context.Context, nodeID int64, command nodeproto.Command, report *report) error {
	if command.Type == nodeproto.CommandArtwork && command.EncodeArtwork != nil {
		return agent.artwork(ctx, nodeID, *command.EncodeArtwork)
	}
	if command.Type == nodeproto.CommandPrepare && command.Prepare != nil {
		agent.prepare(ctx, nodeID, *command.Prepare)
		return nil
	}
	if command.Type == nodeproto.CommandExtractSubtitle && command.ExtractSubtitle != nil {
		agent.extractSubtitle(ctx, nodeID, *command.ExtractSubtitle)
		return nil
	}
	if command.Type == nodeproto.CommandMeasureLoudness && command.MeasureLoudness != nil {
		agent.measureLoudness(ctx, nodeID, *command.MeasureLoudness)
		return nil
	}
	if command.Type == nodeproto.CommandScan {
		root, known := agent.roots[command.Scan.RootPath]
		if !known {
			agent.logger.WarnContext(ctx, "refused to scan a root this node was not given", "root",
				command.Scan.RootPath)
			return nil
		}
		agent.setDoing("scanning " + root.Path())
		defer agent.setDoing("idle")
		emit := func(record nodeproto.Record) error {
			// Once a scan says it is complete, the node is idle: a status record sent after the
			// completion must not still say it is scanning.
			if record.Type == nodeproto.RecordScanComplete {
				agent.setDoing("idle")
			}
			return report.emit(ctx, record)
		}
		err := agent.workers.Scanner.Scan(ctx, root, emit)
		if err != nil && ctx.Err() == nil {
			agent.logger.WarnContext(ctx, "a scan ended early", "root", root.Path(), "err", err)
		}
		return nil
	}
	agent.logger.WarnContext(ctx, "ignored a command this node does not know", "type", command.Type)
	return nil
}

// artwork re-encodes an image and posts the answer — the sizes, or why there are none — to the
// core, whose artwork store takes images from nowhere else (D212 §8).
func (agent *Agent) artwork(ctx context.Context, nodeID int64, command nodeproto.Artwork) error {
	agent.setDoing("re-encoding artwork")
	defer agent.setDoing("idle")
	var result nodeproto.ArtworkResult
	sourceHash := ""
	if command.AbsolutePath != nil {
		source, err := agent.readImage(*command.AbsolutePath)
		if err == nil {
			sum := sha256.Sum256(source)
			command.SourceImage, command.AbsolutePath, sourceHash = source, nil, hex.EncodeToString(sum[:])
		} else {
			result = nodeproto.ArtworkResult{JobID: command.JobID, Refusal: err.Error()}
		}
	}
	if result.Refusal == "" {
		result = agent.workers.Artwork.Derive(ctx, command)
		result.SourceSha256 = sourceHash
	}
	if result.Refusal != "" {
		agent.logger.WarnContext(ctx, "refused an image", "jobId", command.JobID, "refusal", result.Refusal)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode artwork %d: %w", command.JobID, err)
	}
	var accepted struct{}
	body := bytes.NewReader(encoded)
	return agent.exchange(ctx, http.MethodPost, nodeproto.PathArtwork, nodeID, body, &accepted)
}

// maxImageBytes bounds an image beside the media the node reads: posters are megabytes.
const maxImageBytes = 64 << 20

// readImage reads an image beside the media through the root it is under, so no path the core
// names can reach outside the node's roots (D107).
func (agent *Agent) readImage(named nodeproto.Path) ([]byte, error) {
	absolute := named.Text
	if named.Bytes != nil {
		absolute = string(named.Bytes)
	}
	for rootPath, root := range agent.roots {
		rel, under := strings.CutPrefix(absolute, rootPath+"/")
		if !under {
			continue
		}
		return readBounded(root, rel, maxImageBytes)
	}
	return nil, errors.New("the image is under none of this node's roots")
}

// readBounded reads one file under a root, refusing one past maxBytes.
func readBounded(root *confine.MediaRoot, rel string, maxBytes int) ([]byte, error) {
	file, err := root.Open(rel)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", rel, err)
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // read-only
	contents, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rel, err)
	}
	if len(contents) > maxBytes {
		return nil, fmt.Errorf("%s is larger than %d bytes", rel, maxBytes)
	}
	return contents, nil
}

func (agent *Agent) setDoing(doing string) {
	agent.mutex.Lock()
	defer agent.mutex.Unlock()
	agent.doing = doing
}

func (agent *Agent) status() nodeproto.Record {
	disks := agent.disks()
	agent.mutex.Lock()
	defer agent.mutex.Unlock()
	status := &nodeproto.Status{SentAt: agent.clock.Now().UnixMilli(), Activity: agent.doing, Disks: disks}
	if agent.workers.Keeper != nil {
		if usage, known := agent.workers.Keeper.LastUsage(); known {
			status.PreparedUsage = &nodeproto.PreparedUsage{
				UsedBytes: usage.Bytes, BudgetBytes: usage.BudgetBytes, DiskUsedPercent: usage.DiskPercent,
				DiskUsedLimitPercent: usage.MaxDiskPercent,
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
			disk.TotalBytes, disk.AvailableBytes = space.TotalBytes, space.FreeBytes
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
		text, _ := io.ReadAll(io.LimitReader(response.Body, 4096)) //nolint:errcheck // best effort
		return fmt.Errorf("%s %s: %s: %s", method, path, response.Status, bytes.TrimSpace(text))
	}
	if err := json.NewDecoder(response.Body).Decode(into); err != nil {
		return fmt.Errorf("%s %s: the answer is not JSON: %w", method, path, err)
	}
	return nil
}

// stream sends the report as one request whose body is written as records arrive. The client
// sends each write as its own chunk, so the core reads a record as soon as it is written.
func (agent *Agent) stream(ctx context.Context, nodeID int64, report *report) error {
	reader, writer := io.Pipe()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, coreURL+nodeproto.PathReport, reader)
	if err != nil {
		return fmt.Errorf("build the report request: %w", err)
	}
	request.Header.Set(nodeHeader, strconv.FormatInt(nodeID, 10))
	request.Header.Set("Content-Type", "application/jsonl")
	answered := make(chan error, 1)
	go func() {
		response, err := agent.client.Do(request)
		if err == nil {
			err = fmt.Errorf("the core ended the report: %s", response.Status)
			_ = response.Body.Close() //nolint:errcheck // nothing to read
		}
		answered <- err
		_ = reader.CloseWithError(err)
	}()
	encoder := json.NewEncoder(writer)
	for {
		select {
		case <-ctx.Done():
			_ = writer.Close() //nolint:errcheck // always nil
			return errors.Join(fmt.Errorf("report stopped: %w", ctx.Err()), <-answered)
		case record := <-report.records:
			if err := encoder.Encode(record); err != nil {
				return fmt.Errorf("write record %d: %w", record.ReportSeq, errors.Join(err, <-answered))
			}
		}
	}
}
