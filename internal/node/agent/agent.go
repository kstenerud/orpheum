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
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/prepared"
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

// Scanner lists under a media root and examines files there; scan.Scanner is the real one.
type Scanner interface {
	List(ctx context.Context, root *confine.MediaRoot, start string, depth int) ([]nodeproto.ScanRecord, error)
	Examine(ctx context.Context, root *confine.MediaRoot, files []nodeproto.ExamineFile, between func() error) (
		[]nodeproto.ScanRecord, error)
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
	// ThroughReader is nil on a node that does not read videos through.
	ThroughReader ThroughReader
	// ChunkMaker is nil on a node that does not make chunks.
	ChunkMaker ChunkMaker
	// PreparedShelf keeps the prepared directory within its budget; nil on a node that does not
	// prepare.
	PreparedShelf PreparedShelf
	// PreparedDirectory is where renditions are written, whose disk the status reports; empty on
	// a node that does not prepare.
	PreparedDirectory string
	// ProgressEvery is how often, at most, a running prepare tells the core how far it is:
	// node.progress_every_seconds.
	ProgressEvery time.Duration
	// EndedWorkKept is how long the node remembers an attempt whose result the core acknowledged,
	// so the same command sent again is not run again: node.ended_work_kept_hours.
	EndedWorkKept time.Duration
	// HeldResults are the results too large to post, which the byte socket serves; New makes them
	// when nil.
	HeldResults *HeldResults
	// FindDevices finds what the roots and prepared directory are on, for each hello (D240 §5):
	// hardware.Devices over them. Nil finds none.
	FindDevices func() ([]nodeproto.Device, error)
	// Stuck is the work the node gave up on that has not ended, for its status (D233 §10):
	// StuckOf the runner's. Nil has none.
	Stuck func() []nodeproto.Stuck
	// ReadStallWindow is how long a listing's or an examining's own reading may go without moving
	// before the node answers it as stalled on its disk (D233 §10): ReadStallWindowFor the probe's
	// timeout. Zero is defaultReadStallWindow.
	ReadStallWindow time.Duration
}

// defaultReadStallWindow is the least time a reading in the node's own process may go without
// moving: a listing lists each entry and an examining reads a hash in chunks, so minutes without
// either is a disk not answering, not a slow one.
const defaultReadStallWindow = 5 * time.Minute

// ReadStallWindowFor is the read window of a node whose ffprobe may take up to probeTimeout on one
// file, during which an examining does not move: defaultReadStallWindow past it.
func ReadStallWindowFor(probeTimeout time.Duration) time.Duration {
	return defaultReadStallWindow + probeTimeout
}

// PreparedShelf keeps the prepared directory within its budget: prepared.Shelf (D217 §5). It lists
// the renditions it holds by their manifests for the hello, and evicts those the core does not keep
// (D238 §2).
type PreparedShelf interface {
	Evict() (prepared.Usage, error)
	LastUsage() (prepared.Usage, bool)
	Inventory() ([]nodeproto.RenditionManifest, error)
	Reconcile(listed, kept []string) (evicted []string, err error)
}

// Agent is the node's loop (D207): it dials the core's socket, says hello, long-polls for
// commands and holds the work they give it, and posts its status every tick. When the poll or a
// status breaks it waits for the next tick and says hello again.
type Agent struct {
	client  *http.Client
	hello   nodeproto.Hello
	roots   map[string]*confine.MediaRoot
	workers Workers
	clock   clock.Clock
	logger  *slog.Logger

	work  *workList
	ended *endedAttempts
	// stuckReads are the readings the node answered as stalled that have not ended.
	stuckReads stuckReads
	// takenCommandSeq is the last command this session took, which the status says.
	takenCommandSeq atomic.Int64
	answerRetry     answerRetry

	mutex sync.Mutex
	// devices are what the roots are on, as the last hello found them. Guarded by mutex.
	devices nodeproto.Devices
}

// New returns an agent that talks to the core at socketPath. The hello is sent as given, with
// the roots' paths and this build's protocol version filled in.
func New(
	socketPath string, hello nodeproto.Hello, roots []*confine.MediaRoot, workers Workers, clockSource clock.Clock,
	logger *slog.Logger,
) *Agent {
	if workers.HeldResults == nil {
		workers.HeldResults = NewHeldResults()
	}
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
		clock:   clockSource, logger: logger, work: newWorkList(hello.Capacity, clockSource),
		ended:       newEndedAttempts(),
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
	hello := agent.helloNow(ctx)
	reply, err := agent.sayHello(ctx, hello)
	if err != nil {
		return err
	}
	agent.logger.InfoContext(ctx, "said hello to the core", "nodeId", reply.NodeID)
	agent.evictUnkept(ctx, hello.Renditions, reply.KeptRenditionDirs)
	agent.work.setReaderLimits(reply.DeviceReaderLimits)
	agent.ended.saidHello()
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var sessionTasks sync.WaitGroup
	sessionTasks.Go(func() { cancel(agent.pollCommands(ctx, nodeCtx, reply.NodeID)) })
	sessionTasks.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick:
				agent.work.stallStillReads(agent.readStallWindow())
				if err := agent.postStatus(ctx, reply.NodeID); err != nil {
					cancel(err)
					return
				}
				agent.sayHeld(ctx, reply.NodeID)
			}
		}
	})
	sessionTasks.Wait()
	return context.Cause(ctx)
}

// readStallWindow is how long the node's own reading may go without moving (Workers.ReadStallWindow).
func (agent *Agent) readStallWindow() time.Duration {
	if agent.workers.ReadStallWindow > 0 {
		return agent.workers.ReadStallWindow
	}
	return defaultReadStallWindow
}

// helloNow is the node's hello with the devices its roots are on and the renditions its prepared
// directory holds, now: a disk may have come or gone since the last.
func (agent *Agent) helloNow(ctx context.Context) nodeproto.Hello {
	hello := agent.hello
	if agent.workers.FindDevices != nil {
		devices, err := agent.workers.FindDevices()
		if err != nil {
			// The core counts every root as one reader's device until the next hello says more.
			agent.logger.WarnContext(ctx, "could not find the devices the roots are on", "err", err)
		}
		hello.Devices = devices
		agent.mutex.Lock()
		agent.devices = devices
		agent.mutex.Unlock()
	}
	if agent.workers.PreparedShelf == nil {
		return hello
	}
	renditions, err := agent.workers.PreparedShelf.Inventory()
	if err != nil {
		// The core adopts none of them this time, and none is evicted for it.
		agent.logger.WarnContext(ctx, "could not list the prepared renditions for the hello", "err", err)
	}
	hello.Renditions = renditions
	return hello
}

// evictUnkept evicts the renditions the hello listed that the core's reply does not keep, and the
// makings that never finished (D238 §2).
func (agent *Agent) evictUnkept(ctx context.Context, listed []nodeproto.RenditionManifest, kept []string) {
	if agent.workers.PreparedShelf == nil {
		return
	}
	listedDirs := make([]string, 0, len(listed))
	for _, manifest := range listed {
		listedDirs = append(listedDirs, manifest.Prepare.RenditionDir)
	}
	evicted, err := agent.workers.PreparedShelf.Reconcile(listedDirs, kept)
	if err != nil {
		agent.logger.WarnContext(ctx, "could not evict the renditions the core does not keep", "err", err)
	}
	if len(evicted) > 0 {
		agent.logger.InfoContext(ctx, "evicted the renditions the core does not keep", "renditionDirs", evicted)
	}
}

func (agent *Agent) sayHello(ctx context.Context, hello nodeproto.Hello) (nodeproto.HelloReply, error) {
	encoded, err := json.Marshal(hello)
	if err != nil {
		return nodeproto.HelloReply{}, fmt.Errorf("encode the hello: %w", err)
	}
	var reply nodeproto.HelloReply
	err = agent.exchange(ctx, http.MethodPost, nodeproto.PathHello, 0, bytes.NewReader(encoded), &reply)
	return reply, err
}

// pollCommands long-polls for commands. Work is held by the node; a cancel or a reprioritize is
// carried out as it arrives.
func (agent *Agent) pollCommands(ctx, nodeCtx context.Context, nodeID int64) error {
	var lastTakenCommandSeq int64
	agent.takenCommandSeq.Store(0)
	for {
		var answer nodeproto.Commands
		path := nodeproto.PathCommands + "?" + nodeproto.QueryAfterCommandSeq + "=" +
			strconv.FormatInt(lastTakenCommandSeq, 10)
		if err := agent.exchange(ctx, http.MethodGet, path, nodeID, http.NoBody, &answer); err != nil {
			return err
		}
		if answer.DeviceReaderLimits != nil {
			agent.work.setReaderLimits(answer.DeviceReaderLimits)
		}
		for _, command := range answer.Commands {
			if !agent.takeOrder(ctx, command) {
				agent.holdCommand(ctx, nodeCtx, nodeID, command)
			}
			lastTakenCommandSeq = command.CommandSeq
			agent.takenCommandSeq.Store(lastTakenCommandSeq)
		}
	}
}

// takeOrder carries out a command that orders the work the node holds, a cancel or a reprioritize, and
// reports whether it was one.
func (agent *Agent) takeOrder(ctx context.Context, command nodeproto.Command) bool {
	if command.Type == nodeproto.CommandCancel {
		if !agent.work.cancel(command.AttemptToken) {
			agent.logger.InfoContext(ctx, "a cancel came for an attempt this node does not hold", "jobId",
				command.JobID, "attemptToken", command.AttemptToken)
		}
		return true
	}
	if command.Type == nodeproto.CommandReprioritize {
		agent.work.reprioritize(command.AttemptToken, command.Priority, command.IsPlayDriven)
		return true
	}
	return false
}

// holdCommand holds a command's work until a slot of its kind is free (workList), on nodeCtx, the
// node's lifetime: a session that ends, the core gone or restarting, stops no work (D233 §2), and
// each posts its result when a core takes it. A command the node cannot read is refused.
func (agent *Agent) holdCommand(ctx, nodeCtx context.Context, nodeID int64, command nodeproto.Command) {
	held := attemptOf(command)
	work, isKnown := agent.workFor(nodeCtx, nodeID, held, command)
	if !isKnown || held.token == "" {
		agent.refuseUnknown(ctx, nodeID, command)
		return
	}
	work.attempt, work.priority, work.isPlayDriven, work.slot = held, command.Priority, command.IsPlayDriven,
		command.Slot()
	if file, isMedia := command.MediaFile(); isMedia {
		work.device = agent.deviceOf(filepath.Join(file.RootPath, string(file.Path)))
	}
	if agent.ended.hasEnded(held.token) {
		agent.logger.InfoContext(ctx, "the core sent again an attempt that ended here; it is not run again",
			"jobId", held.jobID, "attemptToken", held.token)
		return
	}
	isHeld, isWaiting := agent.work.hold(nodeCtx, work)
	if !isHeld {
		agent.logger.InfoContext(ctx, "the core sent again an attempt this node holds", "jobId", held.jobID,
			"attemptToken", held.token)
		return
	}
	if isWaiting {
		go agent.sayProgress(nodeCtx, nodeID, held.progress(nodeproto.ProgressWaiting))
	}
}

// deviceOf is the key of the device an absolute path is on, as the last hello found them, or ""
// when it is on none of them.
func (agent *Agent) deviceOf(path string) string {
	agent.mutex.Lock()
	defer agent.mutex.Unlock()
	return agent.devices.KeyOf(path)
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
	case command.Type == nodeproto.CommandReadThrough && command.ReadThrough != nil:
		return agent.readThroughWork(nodeCtx, nodeID, held, *command.ReadThrough), true
	case command.Type == nodeproto.CommandMakeChunks && command.MakeChunks != nil:
		return agent.chunksWork(nodeCtx, nodeID, held, *command.MakeChunks), true
	case command.Type == nodeproto.CommandEncodeArtwork && command.EncodeArtwork != nil:
		return agent.artworkWork(nodeCtx, nodeID, held, *command.EncodeArtwork), true
	case command.Type == nodeproto.CommandList && command.List != nil:
		return agent.listWork(nodeCtx, nodeID, held, *command.List), true
	case command.Type == nodeproto.CommandExamine && command.Examine != nil:
		return agent.examineWork(nodeCtx, nodeID, held, *command.Examine), true
	}
	return heldWork{}, false
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
	refused := attemptOf(command).named(nodeproto.Result{Refusal: refusal})
	agent.postResult(ctx, nodeID, "a refusal", refused)
}

// artworkWork re-encodes an artwork job's images, each in turn, and posts one answer for all — each
// image's sizes, or why there are none — to the core, whose artwork store takes images from nowhere
// else (D212 §8).
func (agent *Agent) artworkWork(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.ArtworkBatch,
) heldWork {
	return heldWork{run: func(jobCtx context.Context) {
		agent.sayProgress(ctx, nodeID, held.progress(nodeproto.ProgressRunning))
		results := make([]nodeproto.ArtworkResult, 0, len(command.Images))
		for _, image := range command.Images {
			results = append(results, agent.reEncode(jobCtx, image))
			if jobCtx.Err() != nil {
				break
			}
		}
		if jobCtx.Err() != nil && ctx.Err() == nil {
			agent.logger.InfoContext(ctx, "an artwork re-encode was cancelled", "jobId", held.jobID)
			return
		}
		for _, result := range results {
			if result.Refusal != "" {
				agent.logger.WarnContext(ctx, "refused an image", "jobId", held.jobID, "key", result.Key,
					"refusal", result.Refusal)
			}
		}
		agent.postResult(ctx, nodeID, "an artwork re-encode",
			held.named(nodeproto.Result{Artwork: &nodeproto.ArtworkBatchResult{Images: results}}))
	}}
}

// reEncode re-encodes one image, read from beside the media when it names its path there, and
// names the image's key in its answer.
func (agent *Agent) reEncode(ctx context.Context, image nodeproto.Artwork) nodeproto.ArtworkResult {
	sourceHash := ""
	if image.AbsolutePath != nil {
		source, err := agent.readImage(*image.AbsolutePath)
		if err != nil {
			return nodeproto.ArtworkResult{Key: image.Key, Refusal: err.Error()}
		}
		sum := sha256.Sum256(source)
		image.SourceImage, image.AbsolutePath, sourceHash = source, nil, hex.EncodeToString(sum[:])
	}
	result := agent.workers.Artworker.Derive(ctx, image)
	result.Key, result.SourceSha256 = image.Key, sourceHash
	return result
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

// postStatus posts the node's status, now (D234 §6).
func (agent *Agent) postStatus(ctx context.Context, nodeID int64) error {
	encoded, err := json.Marshal(agent.status())
	if err != nil {
		return fmt.Errorf("encode the status: %w", err)
	}
	var answer struct{}
	return agent.exchange(ctx, http.MethodPost, nodeproto.PathStatus, nodeID, bytes.NewReader(encoded), &answer)
}

// status is what the node is doing and holds, and its disks, now.
func (agent *Agent) status() nodeproto.Status {
	disks := agent.disks()
	agent.mutex.Lock()
	defer agent.mutex.Unlock()
	status := nodeproto.Status{
		SentAt: agent.clock.Now().UnixMilli(), Disks: disks, Held: agent.held(),
		TakenCommandSeq: agent.takenCommandSeq.Load(),
	}
	if agent.workers.Stuck != nil {
		status.Stuck = agent.workers.Stuck()
	}
	status.Stuck = append(status.Stuck, agent.stuckReads.all()...)
	if agent.workers.PreparedShelf != nil {
		if usage, known := agent.workers.PreparedShelf.LastUsage(); known {
			status.PreparedUsage = &nodeproto.PreparedUsage{
				UsedBytes: usage.UsedBytes, BudgetBytes: usage.BudgetBytes, DiskUsedPercent: usage.DiskUsedPercent,
				DiskUsedLimitPercent: usage.DiskUsedLimitPercent, DiskAvailableBytes: usage.DiskAvailableBytes,
				DiskMinFreeBytes: usage.DiskMinFreeBytes,
			}
		}
	}
	return status
}

// held is every attempt the node holds, and every one whose result the core has not acknowledged,
// for the core to reconcile with what it gave the node (D234 §6).
func (agent *Agent) held() []nodeproto.HeldAttempt {
	states := agent.work.states()
	held := make([]nodeproto.HeldAttempt, 0, len(states))
	for _, each := range states {
		state := nodeproto.HeldWaiting
		switch {
		case each.isPaused:
			state = nodeproto.HeldPaused
		case each.isRunning:
			state = nodeproto.HeldRunning
		}
		held = append(held, nodeproto.HeldAttempt{
			JobID: each.jobID, AttemptToken: each.token, State: state, SinceMovedMs: each.sinceMoved.Milliseconds(),
		})
	}
	return append(held, agent.ended.awaitingAcknowledgement()...)
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
