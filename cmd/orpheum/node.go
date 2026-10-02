// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/node/agent"
	"github.com/kstenerud/orpheum/internal/node/artwork"
	"github.com/kstenerud/orpheum/internal/node/byteserver"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/hardware"
	"github.com/kstenerud/orpheum/internal/node/loudness"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/node/probe"
	"github.com/kstenerud/orpheum/internal/node/scan"
	"github.com/kstenerud/orpheum/internal/node/subtitle"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/diskspace"
	"github.com/kstenerud/orpheum/internal/platform/host"
	"github.com/kstenerud/orpheum/internal/platform/randbytes"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
)

// coreSocketName is the core's socket's name in server.socket_dir (cut-a §2).
const coreSocketName = "core.sock"

// nodeSocketName is the node's byte socket's name in server.socket_dir, from which the core reads
// media bytes (cut-a §4, D193 §1).
const nodeSocketName = "node.sock"

// errConfinementRequired refuses to run FFmpeg without every confinement mechanism in force.
var errConfinementRequired = errors.New("node.confinement_policy is \"require\", and not every mechanism is in " +
	"force; fix what is missing, or set it to \"report\" to run with what there is and say so (D112)")

// commandNode runs a media node: the child `run` starts, or a node on its own.
func commandNode(ctx context.Context, call invocation) int {
	config, configReport, err := call.loadConfigFromFlags("node", nil)
	if err != nil {
		return call.fail(err)
	}
	logger, err := call.logger(config)
	if err != nil {
		return call.fail(err)
	}
	for _, note := range configReport.Notes() {
		logger.WarnContext(ctx, "the config file needs attention; doctor lists it too", "note", note)
	}
	if err = runNode(ctx, config, logger); err != nil {
		logger.ErrorContext(ctx, "the node stopped", "err", err)
		return exitFailure
	}
	return exitSuccess
}

// runNode opens the media roots, builds the scanner and runs the node's loop until ctx ends.
func runNode(ctx context.Context, config Config, logger *slog.Logger) error {
	return runNodeWith(ctx, config, logger, nodeSeams{})
}

// nodeSeams are where a test reaches into the node; the zero value is the node as it runs.
type nodeSeams struct {
	// wrapScanner wraps the scanner: the M2 acceptance test records a real scan through it and
	// replays one against a media root no process may read (plan-deviations P9).
	wrapScanner func(agent.Scanner) agent.Scanner
	// mechanismStates stands in for confine.MechanismStates, so a test can see what a machine without a mechanism does.
	mechanismStates func() []confine.State
	// isUnconfined runs the tools as they are, not through the shim: a test's fake FFmpeg is a
	// shell script that records each run in its own directory, which no confined run may do.
	isUnconfined bool
}

// checkConfinement is what the machine can confine FFmpeg with (D112): each mechanism missing is
// logged with its remedy, and refuses the node's start when the config requires them all.
func checkConfinement(
	ctx context.Context, config Config, logger *slog.Logger, seams nodeSeams,
) ([]confine.State, error) {
	probeConfinement := confine.MechanismStates
	if seams.mechanismStates != nil {
		probeConfinement = seams.mechanismStates
	}
	confinement := probeConfinement()
	if confine.IsConfined(confinement) {
		return confinement, nil
	}
	for _, state := range confinement {
		if !state.IsInForce {
			logger.WarnContext(ctx, "FFmpeg runs without a confinement mechanism", "mechanism", state.Mechanism,
				"reason", state.Reason, "remedy", state.Remedy)
		}
	}
	if config.Node.ConfinementPolicy == "require" {
		return nil, errConfinementRequired
	}
	return confinement, nil
}

// runNodeWith is runNode with a test's seams.
func runNodeWith(ctx context.Context, config Config, logger *slog.Logger, seams nodeSeams) error {
	confinement, err := checkConfinement(ctx, config, logger, seams)
	if err != nil {
		return err
	}
	roots := openRoots(ctx, config.Node.RootPaths, logger)
	defer func() {
		for _, root := range roots {
			if closeErr := root.Close(); closeErr != nil {
				logger.WarnContext(ctx, "could not close a media root", "err", closeErr)
			}
		}
	}()
	// Every FFmpeg run re-executes this binary as the shim that confines it (cut-a §7).
	shimPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find orpheum's own path for the shim: %w", err)
	}
	if seams.isUnconfined {
		shimPath = ""
	}
	runner, err := execx.NewConfinedRunner(config.Node.FFmpegDir, shimPath)
	if err != nil {
		return err
	}
	encoderChoice := chooseEncoder(ctx, runner, logger)
	hello, err := describeNode(ctx, config, runner, nodeCapabilities{
		confinement: confinement, encoderChoice: encoderChoice, gpus: hardware.GPUs(),
	})
	if err != nil {
		return err
	}
	scanner := newScanner(config, roots, runner, hello)
	// A node that cannot use its prepared directory still scans and serves files as they are; it
	// says why it prepares nothing, and each prepare it is asked for fails with that reason.
	var preparer agent.Preparer
	var preparedShelf agent.PreparedShelf
	directory, err := openPreparedDirectory(config, runner, encoderChoice.Chosen)
	if err != nil {
		logger.ErrorContext(ctx, "this node cannot prepare; it serves files as they are", "err", err)
	} else {
		preparer, preparedShelf = directory.preparer, directory.shelf
		defer readonly.Close(directory.root)
		if _, evictErr := directory.shelf.Evict(); evictErr != nil {
			logger.WarnContext(ctx, "could not bring the prepared directory within its budget", "err", evictErr)
		}
	}
	stopBytes, err := serveBytes(ctx, config, roots, directory, logger)
	if err != nil {
		return err
	}
	defer stopBytes()
	ticker := time.NewTicker(time.Duration(config.Node.StatusIntervalSeconds) * time.Second)
	defer ticker.Stop()
	// An image's probe and each of its encodes are one FFmpeg run over one untrusted input each,
	// bounded as a file's probe is.
	deriver := artwork.NewDeriver(runner, time.Duration(config.Node.ProbeTimeoutSeconds)*time.Second)
	var activeScanner agent.Scanner = scanner
	if seams.wrapScanner != nil {
		activeScanner = seams.wrapScanner(scanner)
	}
	workers := agent.Workers{
		Scanner: activeScanner, Artworker: deriver, Preparer: preparer,
		Subtitler:        subtitle.NewExtractor(runner, hello.Capacity.SlowestReadBytesPerSecond),
		LoudnessMeasurer: loudness.NewMeasurer(runner, hello.Capacity.SlowestReadBytesPerSecond),
		PreparedShelf:    preparedShelf,
	}
	workers.ProgressEvery, workers.EndedWorkKept = workPacing(config)
	if directory.root != nil {
		workers.PreparedDirectory = directory.root.Path()
	}
	coreSocketPath := filepath.Join(config.Server.SocketDir, coreSocketName)
	return agent.New(coreSocketPath, hello, roots, workers, clock.System{}, logger).Run(ctx, ticker.C)
}

// workPacing is how often, at most, a running prepare tells the core how far it is, and how long the
// node remembers work whose result the core took: node.progress_every_seconds and
// node.ended_work_kept_hours.
func workPacing(config Config) (progressEvery, endedWorkKept time.Duration) {
	return time.Duration(config.Node.ProgressEverySeconds) * time.Second,
		time.Duration(config.Node.EndedWorkKeptHours) * time.Hour
}

// chooseEncoder chooses, once, the encoder the node's encodes use: the first hardware one that
// works here, confined, else x264 (cut-a §6).
func chooseEncoder(ctx context.Context, runner *execx.Runner, logger *slog.Logger) prepare.Choice {
	encoderChoice := prepare.ChooseEncoder(ctx, runner)
	logger.InfoContext(ctx, "this node chose its encoder", "chosenEncoder", encoderChoice.Chosen, "usable",
		encoderChoice.Usable())
	return encoderChoice
}

// preparedDirectory is the node's prepared directory: opened for the byte socket to read, the
// preparer that writes into it, and the shelf that keeps it within its budget (D217 §5). Its zero
// value is a node that prepares nothing.
type preparedDirectory struct {
	root     *confine.MediaRoot
	preparer *prepare.Preparer
	shelf    *prepared.Shelf
}

// openPreparedDirectory makes the node's prepared directory if it is missing, and opens it.
func openPreparedDirectory(config Config, runner *execx.Runner, encoder prepare.Encoder) (preparedDirectory, error) {
	if err := os.MkdirAll(config.Node.PreparedDir, 0o750); err != nil {
		return preparedDirectory{}, fmt.Errorf("make the prepared directory: %w", err)
	}
	root, err := confine.OpenMediaRoot(config.Node.PreparedDir)
	if err != nil {
		return preparedDirectory{}, err
	}
	shelf := prepared.New(root.Path(), prepared.Budget{
		MaxBytes:             config.Node.PreparedBudgetGiB * diskspace.BytesPerGiB,
		KeptAfterLastPlay:    time.Duration(config.Node.PreparedKeptAfterPlayHours) * time.Hour,
		DiskUsedLimitPercent: int(config.Node.PreparedDiskUsedLimitPercent),
	}, clock.System{})
	preparer, err := prepare.New(runner, root.Path(), shelf, encoder)
	if err != nil {
		return preparedDirectory{}, errors.Join(err, root.Close())
	}
	return preparedDirectory{root: root, preparer: preparer, shelf: shelf}, nil
}

// serveBytes serves the node's byte socket until the returned stop is called.
func serveBytes(
	ctx context.Context, config Config, roots []*confine.MediaRoot, directory preparedDirectory, logger *slog.Logger,
) (func(), error) {
	listener, err := unixsocket.Listen(config.Server.SocketDir, nodeSocketName)
	if err != nil {
		return nil, err
	}
	server := byteserver.New(roots, directory.root)
	if directory.shelf != nil {
		server.OnRenditionServed(directory.shelf.MarkServed)
	}
	handler := server.Handler()
	httpServer := &http.Server{Handler: handler, ReadHeaderTimeout: config.readHeaderTimeout()}
	logger.InfoContext(ctx, "serving the byte socket", "socket", listener.Addr().String())
	var serving sync.WaitGroup
	serving.Go(func() {
		if serveErr := httpServer.Serve(listener); !errors.Is(serveErr, http.ErrServerClosed) {
			logger.ErrorContext(ctx, "the byte socket stopped", "err", serveErr)
		}
	})
	return func() {
		// A player's reads end with the node; the core answers them with node_silent.
		_ = httpServer.Close() //nolint:errcheck // the listener is ours and closing it cannot fail usefully
		serving.Wait()
	}, nil
}

// openRoots opens each configured media root. One that cannot be opened is logged and left out
// rather than stopping the node: a mount missing at boot must not take the others with it.
func openRoots(ctx context.Context, paths []string, logger *slog.Logger) []*confine.MediaRoot {
	roots := make([]*confine.MediaRoot, 0, len(paths))
	for _, path := range paths {
		root, err := confine.OpenMediaRoot(path)
		if err != nil {
			logger.ErrorContext(ctx, "a media root cannot be opened; the node serves the others", "rootPath", path,
				"err", err)
			continue
		}
		roots = append(roots, root)
	}
	return roots
}

// nodeCapabilities are what the node found it has: its confinement, per mechanism, the encoder
// trials that chose its encoder, and the GPU devices it sees (D225 §3).
type nodeCapabilities struct {
	confinement   []confine.State
	encoderChoice prepare.Choice
	gpus          []nodeproto.GPU
}

// newScanner scans the roots, probing each file with runner; an error it reports names the user
// and group the hello gives, so it can be read against them.
func newScanner(config Config, roots []*confine.MediaRoot, runner *execx.Runner, hello nodeproto.Hello) *scan.Scanner {
	prober := probe.NewProber(runner, time.Duration(config.Node.ProbeTimeoutSeconds)*time.Second)
	return scan.NewScanner(roots, prober, scan.Options{
		VideoExtensions:    config.Node.VideoExtensions,
		SubtitleExtensions: config.Node.SubtitleExtensions,
		MaxErrors:          config.Node.MaxErrorsPerRoot,
		UID:                hello.UID,
		GID:                hello.GID,
	})
}

// startIDBytes is a start id's random bytes before hex: eight never repeat between two starts of a
// node in practice.
const startIDBytes = 8

// newStartID names this start of the node, from random: its hello says it until the node starts
// again, so the core knows a node that lost the work it held in memory (D238 §3).
func newStartID(random io.Reader) (string, error) {
	raw, err := randbytes.Read(random, startIDBytes)
	if err != nil {
		return "", fmt.Errorf("name this start of the node: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// describeNode is the hello: who the node is, which FFmpeg it runs, what it can do and what
// confinement it has.
func describeNode(
	ctx context.Context, config Config, runner *execx.Runner, detected nodeCapabilities,
) (nodeproto.Hello, error) {
	// ffprobe -version prints a few kilobytes of build options; a mebibyte bounds a broken binary.
	output, err := runner.Run(ctx, execx.FFprobe, confine.Access{}, []string{"-version"}, 1<<20)
	if err != nil {
		return nodeproto.Hello{}, fmt.Errorf("run the pinned ffprobe: %w", err)
	}
	firstLine, _, _ := strings.Cut(string(output), "\n")
	ffmpegHash, err := fileSha256(filepath.Join(config.Node.FFmpegDir, string(execx.FFmpeg)))
	if err != nil {
		return nodeproto.Hello{}, err
	}
	startID, err := newStartID(rand.Reader)
	if err != nil {
		return nodeproto.Hello{}, err
	}
	return nodeproto.Hello{
		Name:          config.Node.Name,
		StartID:       startID,
		Build:         thisBuild(),
		Host:          host.Describe(),
		FFmpegVersion: firstLine,
		FFmpegSha256:  ffmpegHash,
		Capabilities:  capabilityNames(detected.encoderChoice.Usable()),
		Confinement:   confinementReport(detected.confinement),
		UID:           os.Geteuid(),
		GID:           os.Getegid(),
		Hardware:      hardwareReport(detected),
		Capacity:      capacityOf(config),
	}, nil
}

// capacityOf is what the node's config says its machine can do (nodeproto.Capacity).
func capacityOf(config Config) nodeproto.Capacity {
	return nodeproto.Capacity{
		EncodeSlotCount: int(config.Node.EncodeSlotCount), RemuxSlotCount: int(config.Node.RemuxSlotCount),
		SlowestReadBytesPerSecond: config.Node.SlowestReadMiBPerSecond << 20,
	}
}

// hardwareReport is the hello's account of hardware encoding: the devices, each trial and the
// encoder chosen (D225 §3).
func hardwareReport(detected nodeCapabilities) nodeproto.Hardware {
	trials := make([]nodeproto.EncoderTrial, 0, len(detected.encoderChoice.Trials))
	for _, trial := range detected.encoderChoice.Trials {
		trials = append(trials, nodeproto.EncoderTrial{Encoder: string(trial.Encoder), Failure: trial.Failure})
	}
	return nodeproto.Hardware{GPUs: detected.gpus, Trials: trials, ChosenEncoder: string(detected.encoderChoice.Chosen)}
}

// capabilityNames are the hello's (cut-a §4): remux and AAC, which every node can do, and the
// encoders it found it can use.
func capabilityNames(encoders []prepare.Encoder) []string {
	names := []string{"remux", "aac"}
	for _, encoder := range encoders {
		names = append(names, string(encoder))
	}
	return names
}

func fileSha256(path string) (string, error) {
	file, err := os.Open(path) //nolint:gosec // the configured FFmpeg
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	defer readonly.Close(file)
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// confinementReport is each confinement mechanism's state as a hello reports it (D112).
func confinementReport(states []confine.State) []nodeproto.Confinement {
	report := make([]nodeproto.Confinement, 0, len(states))
	for _, state := range states {
		report = append(report, nodeproto.Confinement{
			Mechanism: state.Mechanism, IsInForce: state.IsInForce, CauseCode: state.CauseCode, Reason: state.Reason,
			Remedy: state.Remedy,
		})
	}
	return report
}
