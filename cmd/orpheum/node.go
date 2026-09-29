// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
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

	"github.com/kstenerud/orpheum/internal/core/datadir"
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
	"github.com/kstenerud/orpheum/internal/platform/host"
)

// coreSocket is the core's socket's name in server.socket_dir (cut-a §2).
const coreSocket = "core.sock"

// nodeSocket is the node's byte socket's name in server.socket_dir, from which the core reads
// media bytes (cut-a §4, D193 §1).
const nodeSocket = "node.sock"

// errConfinementRequired refuses to run FFmpeg without every confinement mechanism in force.
var errConfinementRequired = errors.New("node.confinement_policy is \"require\", and not every mechanism is in " +
	"force; fix what is missing, or set it to \"report\" to run with what there is and say so (D112)")

// commandNode runs a media node: the child `run` starts, or a node on its own.
func commandNode(ctx context.Context, call invocation) int {
	config, configReport, err := call.configFlags("node", nil)
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
	// probe stands in for confine.Probe, so a test can see what a machine without a mechanism does.
	probe func() []confine.State
	// isUnconfined runs the tools as they are, not through the shim: a test's fake FFmpeg is a
	// shell script that records each run in its own directory, which no confined run may do.
	isUnconfined bool
}

// checkConfinement is what the machine can confine FFmpeg with (D112): each mechanism missing is
// logged with its remedy, and refuses the node's start when the config requires them all.
func checkConfinement(
	ctx context.Context, config Config, logger *slog.Logger, seams nodeSeams,
) ([]confine.State, error) {
	probeConfinement := confine.Probe
	if seams.probe != nil {
		probeConfinement = seams.probe
	}
	confinement := probeConfinement()
	if confine.IsConfined(confinement) {
		return confinement, nil
	}
	for _, state := range confinement {
		if !state.InForce {
			logger.WarnContext(ctx, "FFmpeg runs without a confinement mechanism", "mechanism", state.Mechanism,
				"why", state.Reason, "remedy", state.Remedy)
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
	roots := openRoots(ctx, config.Node.Roots, logger)
	defer func() {
		for _, root := range roots {
			if closeErr := root.Close(); closeErr != nil {
				logger.WarnContext(ctx, "could not close a media root", "err", closeErr)
			}
		}
	}()
	// Every FFmpeg run re-executes this binary as the shim that confines it (cut-a §7).
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find orpheum's own path for the shim: %w", err)
	}
	if seams.isUnconfined {
		self = ""
	}
	runner, err := execx.NewConfinedRunner(config.Node.FFmpegDir, self)
	if err != nil {
		return err
	}
	encoding := chooseEncoder(ctx, runner, logger)
	hello, err := describeNode(ctx, config, runner, nodeCapabilities{
		confinement: confinement, encoding: encoding, gpus: hardware.GPUs(),
	})
	if err != nil {
		return err
	}
	prober := probe.NewProber(runner, time.Duration(config.Node.ProbeTimeoutS)*time.Second)
	scanner := scan.NewScanner(roots, prober, scan.Options{
		VideoExtensions:    config.Node.VideoExtensions,
		SubtitleExtensions: config.Node.SubtitleExtensions,
		MaxErrors:          config.Node.MaxErrorsPerRoot,
		UID:                hello.UID,
		GID:                hello.GID,
	})
	// A node that cannot use its prepared directory still scans and serves files as they are; it
	// says why it prepares nothing, and each prepare it is asked for fails with that reason.
	var preparing agent.Preparer
	var keeping agent.Keeper
	directory, err := openPrepared(config, runner, encoding.Chosen)
	if err != nil {
		logger.ErrorContext(ctx, "this node cannot prepare; it serves files as they are", "err", err)
	} else {
		preparing, keeping = directory.preparer, directory.shelf
		defer func() { _ = directory.root.Close() }() //nolint:errcheck // read-only
		if _, evictErr := directory.shelf.Evict(); evictErr != nil {
			logger.WarnContext(ctx, "could not bring the prepared directory within its budget", "err", evictErr)
		}
	}
	stopBytes, err := serveBytes(ctx, config, roots, directory, logger)
	if err != nil {
		return err
	}
	defer stopBytes()
	ticker := time.NewTicker(time.Duration(config.Node.StatusIntervalS) * time.Second)
	defer ticker.Stop()
	// An image's probe and each of its encodes are one FFmpeg run over one untrusted input each,
	// bounded as a file's probe is.
	deriver := artwork.NewDeriver(runner, time.Duration(config.Node.ProbeTimeoutS)*time.Second)
	var scanning agent.Scanner = scanner
	if seams.wrapScanner != nil {
		scanning = seams.wrapScanner(scanner)
	}
	workers := agent.Workers{
		Scanner: scanning, Artwork: deriver, Preparer: preparing, Subtitles: subtitle.NewExtractor(runner),
		Loudness: loudness.NewMeasurer(runner), Keeper: keeping,
	}
	if directory.root != nil {
		workers.PreparedDirectory = directory.root.Path()
	}
	node := agent.New(filepath.Join(config.Server.SocketDir, coreSocket), hello, roots, workers, clock.System{}, logger)
	return node.Run(ctx, ticker.C)
}

// chooseEncoder chooses, once, the encoder the node's encodes use: the first hardware one that
// works here, confined, else x264 (cut-a §6).
func chooseEncoder(ctx context.Context, runner *execx.Runner, logger *slog.Logger) prepare.Choice {
	encoding := prepare.ChooseEncoder(ctx, runner)
	logger.InfoContext(ctx, "this node chose its encoder", "encoder", encoding.Chosen, "usable", encoding.Usable())
	return encoding
}

// preparedDirectory is the node's prepared directory: opened for the byte socket to read, the
// preparer that writes into it, and the shelf that keeps it within its budget (D217 §5). Its zero
// value is a node that prepares nothing.
type preparedDirectory struct {
	root     *confine.MediaRoot
	preparer *prepare.Preparer
	shelf    *prepared.Shelf
}

// openPrepared makes the node's prepared directory if it is missing, and opens it.
func openPrepared(config Config, runner *execx.Runner, encoder prepare.Encoder) (preparedDirectory, error) {
	if err := os.MkdirAll(config.Node.PreparedDir, 0o750); err != nil {
		return preparedDirectory{}, fmt.Errorf("make the prepared directory: %w", err)
	}
	root, err := confine.OpenMediaRoot(config.Node.PreparedDir)
	if err != nil {
		return preparedDirectory{}, err
	}
	shelf := prepared.New(root.Path(), prepared.Budget{
		MaxBytes: config.Node.PreparedBudgetGiB << 30, Grace: time.Duration(config.Node.PreparedGraceH) * time.Hour,
		MaxDiskPercent: int(config.Node.PreparedDiskUsedLimitPercent),
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
	listener, err := datadir.ListenUnix(config.Server.SocketDir, nodeSocket)
	if err != nil {
		return nil, err
	}
	server := byteserver.New(roots, directory.root)
	if directory.shelf != nil {
		server.OnRenditionServed(directory.shelf.Served)
	}
	handler := server.Handler()
	listening := &http.Server{Handler: handler, ReadHeaderTimeout: config.readHeaderTimeout()}
	logger.InfoContext(ctx, "serving the byte socket", "socket", listener.Addr().String())
	var serving sync.WaitGroup
	serving.Go(func() {
		if serveErr := listening.Serve(listener); !errors.Is(serveErr, http.ErrServerClosed) {
			logger.ErrorContext(ctx, "the byte socket stopped", "err", serveErr)
		}
	})
	return func() {
		// A player's reads end with the node; the core answers them with node_silent.
		_ = listening.Close() //nolint:errcheck // the listener is ours and closing it cannot fail usefully
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
			logger.ErrorContext(ctx, "a media root cannot be opened; the node serves the others", "root", path,
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
	confinement []confine.State
	encoding    prepare.Choice
	gpus        []nodeproto.GPU
}

// describeNode is the hello: who the node is, which FFmpeg it runs, what it can do and what
// confinement it has.
func describeNode(
	ctx context.Context, config Config, runner *execx.Runner, found nodeCapabilities,
) (nodeproto.Hello, error) {
	output, err := runner.Run(ctx, execx.FFprobe, confine.Access{}, []string{"-version"}, 1<<20)
	if err != nil {
		return nodeproto.Hello{}, fmt.Errorf("run the pinned ffprobe: %w", err)
	}
	firstLine, _, _ := strings.Cut(string(output), "\n")
	ffmpegHash, err := fileSHA256(filepath.Join(config.Node.FFmpegDir, string(execx.FFmpeg)))
	if err != nil {
		return nodeproto.Hello{}, err
	}
	report := make([]nodeproto.Confinement, 0, len(found.confinement))
	for _, state := range found.confinement {
		report = append(report, nodeproto.Confinement{
			Mechanism: state.Mechanism, IsInForce: state.InForce, CauseCode: state.Cause, Reason: state.Reason,
			Remedy: state.Remedy,
		})
	}
	return nodeproto.Hello{
		Name:          config.Node.Name,
		Build:         thisBuild(),
		Host:          host.Describe(),
		FFmpegVersion: firstLine,
		FFmpegSha256:  ffmpegHash,
		Capabilities:  capabilities(found.encoding.Usable()),
		Confinement:   report,
		UID:           os.Geteuid(),
		GID:           os.Getegid(),
		Hardware:      hardwareReport(found),
	}, nil
}

// hardwareReport is the hello's account of hardware encoding: the devices, each trial and the
// encoder chosen (D225 §3).
func hardwareReport(found nodeCapabilities) nodeproto.Hardware {
	trials := make([]nodeproto.EncoderTrial, 0, len(found.encoding.Trials))
	for _, trial := range found.encoding.Trials {
		trials = append(trials, nodeproto.EncoderTrial{Encoder: string(trial.Encoder), Failure: trial.Failure})
	}
	return nodeproto.Hardware{GPUs: found.gpus, Trials: trials, ChosenEncoder: string(found.encoding.Chosen)}
}

// capabilities are the hello's (cut-a §4): remux and AAC, which every node can do, and the
// encoders it found it can use.
func capabilities(encoders []prepare.Encoder) []string {
	names := []string{"remux", "aac"}
	for _, encoder := range encoders {
		names = append(names, string(encoder))
	}
	return names
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path) //nolint:gosec // the configured FFmpeg
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // read-only
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
