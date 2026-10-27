// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/agent"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// recordingScanner lists and examines for real and keeps what it answered, for a replay.
type recordingScanner struct {
	inner agent.Scanner
	mutex sync.Mutex
	// listings are each listing's records, by the root listed.
	listings map[string][]nodeproto.ScanRecord
	// examined are each file's record, by its path relative to its root.
	examined map[string]nodeproto.ScanRecord
}

func (recorder *recordingScanner) List(ctx context.Context, root *confine.MediaRoot, command nodeproto.List) (
	[]nodeproto.ScanRecord, error,
) {
	records, err := recorder.inner.List(ctx, root, command)
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	if recorder.listings == nil {
		recorder.listings = map[string][]nodeproto.ScanRecord{}
	}
	recorder.listings[root.Path()] = records
	return records, err
}

func (recorder *recordingScanner) Examine(
	ctx context.Context, root *confine.MediaRoot, files []nodeproto.ExamineFile, between func() error,
) ([]nodeproto.ScanRecord, error) {
	records, err := recorder.inner.Examine(ctx, root, files, between)
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	if recorder.examined == nil {
		recorder.examined = map[string]nodeproto.ScanRecord{}
	}
	for _, record := range records {
		recorder.examined[string(examinedPath(record).Bytes())] = record
	}
	return records, err
}

// examinedPath is the path an examining's record is of.
func examinedPath(record nodeproto.ScanRecord) nodeproto.Path {
	switch {
	case record.Observed != nil:
		return record.Observed.Path
	case record.Sidecar != nil:
		return record.Sidecar.Path
	case record.Error != nil:
		return record.Error.Path
	}
	return nodeproto.Path{}
}

// replayingScanner answers with what was recorded and touches nothing: the node of plan-deviations
// P9.
type replayingScanner struct{ recorder *recordingScanner }

func (replay replayingScanner) List(_ context.Context, root *confine.MediaRoot, _ nodeproto.List) (
	[]nodeproto.ScanRecord, error,
) {
	return replay.recorder.listings[root.Path()], nil
}

func (replay replayingScanner) Examine(
	_ context.Context, _ *confine.MediaRoot, files []nodeproto.ExamineFile, _ func() error,
) ([]nodeproto.ScanRecord, error) {
	records := make([]nodeproto.ScanRecord, 0, len(files))
	for _, file := range files {
		records = append(records, replay.recorder.examined[string(file.Path.Bytes())])
	}
	return records, nil
}

// storedFiling is one file's filing as persistent.db holds it.
type storedFiling struct {
	filing string
	reason string
}

// acceptanceRun serves a films library over root with the node start gives it, until the core
// has identified the library, and returns every live file's filing by its path under root.
func acceptanceRun(
	t *testing.T, config Config, root string, start func(captured capturedLog) func(ctx context.Context) error,
) map[string]storedFiling {
	t.Helper()
	nodeID := enrolNodeWithLibrary(t, config, root,
		libraryShape{name: "Everything", kind: "movies", metadataSource: "nfo"})
	const identified = "identified a library"
	captured := newCapturedLog(identified)
	server := startServing(t, config, captured, identified, start(captured))
	if stopped := server.stop(); stopped != nil {
		t.Errorf("serve returned %v", stopped)
	}
	filings := map[string]storedFiling{}
	for _, file := range liveFilingsOfNode(t, config, nodeID) {
		filings[strings.TrimPrefix(file.reportedPathRaw, root+"/")] = storedFiling{
			filing: file.filing, reason: file.reason,
		}
	}
	return filings
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
	config.Node.RootPaths = []string{root}
	config.Node.FFmpegDir = testutil.FFmpegDirectory(t)

	recorder := &recordingScanner{}
	first := acceptanceRun(t, config, root, func(captured capturedLog) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			return runNodeWith(ctx, config, captured.logger, nodeSeams{
				statusEvery: time.Second,
				wrapScanner: func(inner agent.Scanner) agent.Scanner {
					recorder.inner = inner
					return recorder
				},
			})
		}
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
	replayConfig.Node.CoreSocket = filepath.Join(replayConfig.Instance.RunDir, coreSocketName)
	second := acceptanceRun(t, replayConfig, root, func(captured capturedLog) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			hello := nodeproto.Hello{
				Name: replayConfig.Node.Name, Build: thisBuild(), Capabilities: []string{},
				ByteSocketPath: filepath.Join(replayConfig.Instance.RunDir, nodeSocketName),
			}
			node := agent.New(replayConfig.Node.CoreSocket, hello,
				[]*confine.MediaRoot{media}, agent.Workers{Scanner: replayingScanner{recorder: recorder}},
				clock.System{}, captured.logger)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			return node.Run(ctx, ticker.C)
		}
	})
	checkSameFilings(t, first, second)
}

// checkFilings checks every file of the tree has a filing, every file not listed a reason, the
// unreadable file an error, and every trailer and featurette of a film is an extra.
func checkFilings(t *testing.T, tree testutil.FixtureTree, first map[string]storedFiling) {
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
		t.Fatalf("%d of %d files in persistent.db; missing %v", len(first), len(tree.Files), missing)
	}
	if unreadable := first[tree.UnreadablePath]; unreadable.filing != "errored" {
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
func checkSameFilings(t *testing.T, first, second map[string]storedFiling) {
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
