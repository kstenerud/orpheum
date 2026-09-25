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
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/node/probe"
	"github.com/kstenerud/orpheum/internal/node/scan"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// coreSocket is the core's socket's name in server.socket_dir (cut-a §2).
const coreSocket = "core.sock"

// nodeSocket is the node's byte socket's name in server.socket_dir, from which the core reads
// media bytes (cut-a §4, D193 §1).
const nodeSocket = "node.sock"

// errConfinementRequired refuses to run FFmpeg unconfined while confinement is not built.
var errConfinementRequired = errors.New("node.confinement is \"require\", and confinement is not built " +
	"until M3; set it to \"report\" to run unconfined and say so (D112)")

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
	return runNodeScanning(ctx, config, logger, nil)
}

// runNodeScanning is runNode with the scanner wrapped: the M2 acceptance test records a real
// scan through it and replays one against a media root no process may read (plan-deviations P9).
func runNodeScanning(
	ctx context.Context, config Config, logger *slog.Logger, wrap func(agent.Scanner) agent.Scanner,
) error {
	if config.Node.Confinement == "require" {
		return errConfinementRequired
	}
	roots := openRoots(ctx, config.Node.Roots, logger)
	defer func() {
		for _, root := range roots {
			if err := root.Close(); err != nil {
				logger.WarnContext(ctx, "could not close a media root", "err", err)
			}
		}
	}()
	runner, err := execx.NewRunner(config.Node.FFmpegDir)
	if err != nil {
		return err
	}
	hello, err := describeNode(ctx, config, runner)
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
	prepared, preparer, err := openPrepared(config, runner)
	if err != nil {
		logger.ErrorContext(ctx, "this node cannot prepare; it serves files as they are", "err", err)
	} else {
		preparing = preparer
		defer func() { _ = prepared.Close() }() //nolint:errcheck // read-only
	}
	stopBytes, err := serveBytes(ctx, config, roots, prepared, logger)
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
	if wrap != nil {
		scanning = wrap(scanner)
	}
	workers := agent.Workers{Scanner: scanning, Artwork: deriver, Preparer: preparing}
	node := agent.New(filepath.Join(config.Server.SocketDir, coreSocket), hello, roots, workers, clock.System{}, logger)
	return node.Run(ctx, ticker.C)
}

// openPrepared makes the node's prepared directory if it is missing, and returns it opened for
// the byte socket to read, and the preparer that writes into it (D217 §5).
func openPrepared(config Config, runner *execx.Runner) (*confine.MediaRoot, *prepare.Preparer, error) {
	if err := os.MkdirAll(config.Node.PreparedDir, 0o750); err != nil {
		return nil, nil, fmt.Errorf("make the prepared directory: %w", err)
	}
	prepared, err := confine.OpenMediaRoot(config.Node.PreparedDir)
	if err != nil {
		return nil, nil, err
	}
	preparer, err := prepare.New(runner, prepared.Path())
	if err != nil {
		return nil, nil, errors.Join(err, prepared.Close())
	}
	return prepared, preparer, nil
}

// serveBytes serves the node's byte socket until the returned stop is called.
func serveBytes(
	ctx context.Context, config Config, roots []*confine.MediaRoot, prepared *confine.MediaRoot, logger *slog.Logger,
) (func(), error) {
	listener, err := datadir.ListenUnix(config.Server.SocketDir, nodeSocket)
	if err != nil {
		return nil, err
	}
	handler := byteserver.New(roots, prepared).Handler()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: config.readHeaderTimeout()}
	logger.InfoContext(ctx, "serving the byte socket", "socket", listener.Addr().String())
	var serving sync.WaitGroup
	serving.Go(func() {
		if serveErr := server.Serve(listener); !errors.Is(serveErr, http.ErrServerClosed) {
			logger.ErrorContext(ctx, "the byte socket stopped", "err", serveErr)
		}
	})
	return func() {
		// A player's reads end with the node; the core answers them with node_silent.
		_ = server.Close() //nolint:errcheck // the listener is ours and closing it cannot fail usefully
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

// describeNode is the hello: who the node is, which FFmpeg it runs and what confinement it has.
func describeNode(ctx context.Context, config Config, runner *execx.Runner) (nodeproto.Hello, error) {
	output, err := runner.Run(ctx, execx.FFprobe, []string{"-version"}, 1<<20)
	if err != nil {
		return nodeproto.Hello{}, fmt.Errorf("run the pinned ffprobe: %w", err)
	}
	firstLine, _, _ := strings.Cut(string(output), "\n")
	ffmpegHash, err := fileSHA256(filepath.Join(config.Node.FFmpegDir, string(execx.FFmpeg)))
	if err != nil {
		return nodeproto.Hello{}, err
	}
	notBuilt := "confinement is built in M3; until then every FFmpeg run is unconfined"
	return nodeproto.Hello{
		Name:          config.Node.Name,
		Version:       version,
		FFmpegVersion: firstLine,
		FFmpegHash:    ffmpegHash,
		Capabilities:  []string{},
		Confinement: []nodeproto.Confinement{
			{Mechanism: "landlock", Reason: notBuilt, Remedy: "none until M3"},
			{Mechanism: "seccomp", Reason: notBuilt, Remedy: "none until M3"},
		},
		UID: os.Geteuid(),
		GID: os.Getegid(),
	}, nil
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
