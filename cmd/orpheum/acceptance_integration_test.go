// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package main

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/node/agent"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/scan"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// recordingScanner scans for real and keeps every record it emits, for a replay.
type recordingScanner struct {
	inner   agent.Scanner
	mutex   sync.Mutex
	records []nodeproto.Record
}

func (recorder *recordingScanner) Scan(ctx context.Context, root *confine.MediaRoot, emit scan.Emit) error {
	return recorder.inner.Scan(ctx, root, func(record nodeproto.Record) error {
		recorder.mutex.Lock()
		recorder.records = append(recorder.records, record)
		recorder.mutex.Unlock()
		return emit(record)
	})
}

// replayingScanner emits a recorded scan and touches nothing: the node of plan-deviations P9.
type replayingScanner struct{ records []nodeproto.Record }

func (replay replayingScanner) Scan(_ context.Context, _ *confine.MediaRoot, emit scan.Emit) error {
	for _, record := range replay.records {
		if err := emit(record); err != nil {
			return err
		}
	}
	return nil
}

// filed is one file's filing as library.db holds it.
type filed struct {
	filing string
	reason string
}

// acceptanceRun serves a mixed library over root with the node start gives it, until the core
// has identified the library, and returns every live file's filing by its path under root.
func acceptanceRun(
	t *testing.T, config Config, root string, start func(captured capturedLog) nodes.Starter,
) map[string]filed {
	t.Helper()
	nodeID := createMixedLibrary(t, config, root)
	captured := newCapturedLog("identified a library")
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		outside := wiring{startNode: start(captured), tmdbBaseURL: noTMDB, tmdbImagesBaseURL: noTMDB}
		done <- serve(ctx, config, ConfigReport{}, listener, outside, captured.logger)
	}()
	select {
	case <-captured.signals["identified a library"]:
	case early := <-done:
		cancel()
		t.Fatalf("serve returned before identifying the library: %v\n%s", early, captured)
	case <-time.After(5 * time.Minute):
		cancel()
		t.Fatalf("no identification within five minutes:\n%s", captured)
	}
	cancel()
	if stopped := <-done; stopped != nil {
		t.Errorf("serve returned %v", stopped)
	}
	files, err := openDatabase(t, config).Read().ListLiveFilesByNode(t.Context(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	filings := map[string]filed{}
	for _, file := range files {
		raw := file.Path
		if file.PathRaw != nil {
			raw = string(file.PathRaw)
		}
		reason := ""
		if file.FilingReason != nil {
			reason = *file.FilingReason
		}
		filings[strings.TrimPrefix(raw, root+"/")] = filed{filing: file.Filing, reason: reason}
	}
	return filings
}

// createMixedLibrary makes the database with one mixed library, reading NFO files, over root.
func createMixedLibrary(t *testing.T, config Config, root string) int64 {
	t.Helper()
	database := openDatabase(t, config)
	if err := database.Migrate(t.Context(), config.migrateOptions(t.TempDir(), nil)); err != nil {
		t.Fatal(err)
	}
	nodeID, err := nodes.Enrol(t.Context(), database, clock.System{}, config.Node.Name)
	if err != nil {
		t.Fatal(err)
	}
	_, err = library.Create(t.Context(), database, clock.System{}, library.NewLibrary{
		Name: "Everything", Kind: "mixed", MetadataSource: "nfo", ChangeDetection: "manual",
		NodeID: nodeID, Roots: []string{root},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	return nodeID
}

// The M2 acceptance (cut-a §8): on a 200-file fixture tree every file has a filing, and every
// file not listed says why (D163); and a core whose media root no process may read, fed the same
// scan by a replaying node, files it the same way, because the core never opens a media path
// (D193 §1, plan-deviations P9).
func TestM2_EveryFileHasAFilingAndAReasonAndTheCoreNeverOpensAMediaPath(t *testing.T) {
	if os.Geteuid() == 0 {
		testutil.Require(t, "unprivileged-user", errors.New("the tests run as root, which can read a mode-000 root"))
	}
	tree := testutil.BuildFixtureTree(t)
	root, err := filepath.EvalSymlinks(tree.Root)
	if err != nil {
		t.Fatal(err)
	}
	config, _ := testConfig(t)
	config.Node.Roots = []string{root}
	config.Node.FFmpegDir = testutil.FFmpegDirectory(t)
	config.Node.VideoExtensions = []string{"mkv"}
	config.Node.SubtitleExtensions = []string{"srt"}

	recorder := &recordingScanner{}
	first := acceptanceRun(t, config, root, func(captured capturedLog) nodes.Starter {
		return inProcess(func(ctx context.Context) error {
			return runNodeScanning(ctx, config, captured.logger, func(inner agent.Scanner) agent.Scanner {
				recorder.inner = inner
				return recorder
			})
		})
	})
	checkFilings(t, tree, first)

	// The replay: the node opens the root while it can, then no one can read it.
	media, err := confine.OpenMediaRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = media.Close() }() //nolint:errcheck // the test's own handle
	if err := os.Chmod(root, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) }) //nolint:errcheck,gosec // so the temporary directory can go
	replayConfig, _ := testConfig(t)
	replayConfig.Node = config.Node
	second := acceptanceRun(t, replayConfig, root, func(captured capturedLog) nodes.Starter {
		return inProcess(func(ctx context.Context) error {
			hello := nodeproto.Hello{Name: replayConfig.Node.Name, Version: version, Capabilities: []string{}}
			node := agent.New(filepath.Join(replayConfig.Server.SocketDir, coreSocket), hello,
				[]*confine.MediaRoot{media}, agent.Workers{Scanner: replayingScanner{records: recorder.records}},
				clock.System{}, captured.logger)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			return node.Run(ctx, ticker.C)
		})
	})
	checkSameFilings(t, first, second)
}

// inProcess starts a node's loop in this process.
func inProcess(run func(ctx context.Context) error) nodes.Starter {
	return func(ctx context.Context) (nodes.Process, error) {
		ctx, cancel := context.WithCancel(ctx)
		node := &inProcessNode{done: make(chan struct{}), cancel: cancel}
		go func() {
			defer close(node.done)
			node.err = run(ctx)
		}()
		return node, nil
	}
}

// checkFilings checks every file of the tree has a filing, every file not listed a reason, the
// unreadable file an error, and every trailer and featurette of a film is an extra.
func checkFilings(t *testing.T, tree testutil.FixtureTree, first map[string]filed) {
	t.Helper()
	var missing []string
	for _, file := range tree.Files {
		got, found := first[file]
		switch {
		case !found:
			missing = append(missing, file)
		case got.filing == "":
			t.Errorf("%s has no filing", file)
		case got.filing != "listed" && got.reason == "":
			t.Errorf("%s is %s with no reason", file, got.filing)
		}
	}
	if len(missing) > 0 || len(first) != len(tree.Files) {
		t.Fatalf("%d of %d files in library.db; missing %v", len(first), len(tree.Files), missing)
	}
	if unreadable := first[tree.Unreadable]; unreadable.filing != "errored" {
		t.Errorf("the mode-000 file is %+v, want errored", unreadable)
	}
	for path, got := range first {
		isExtra := strings.Contains(path, "/Trailers/") || strings.HasSuffix(path, "-featurette.mkv")
		if isExtra != (got.filing == "extra") {
			t.Errorf("%s is filed %s (%s); a trailer or featurette of a film is an extra (D162)", path,
				got.filing, got.reason)
		}
	}
}

// checkSameFilings checks the replay filed every file as the real scan did.
func checkSameFilings(t *testing.T, first, second map[string]filed) {
	t.Helper()
	var differ []string
	for path, want := range first {
		if second[path] != want {
			differ = append(differ, path)
		}
	}
	slices.Sort(differ)
	if len(differ) > 0 || len(second) != len(first) {
		t.Errorf("with the root unreadable, %d files, %d filed differently: %v", len(second), len(differ), differ)
	}
}
