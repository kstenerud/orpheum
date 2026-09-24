// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// inProcessNode runs the node's loop in this process, where run would start a child: the same
// code from runNode on, without a second binary.
type inProcessNode struct {
	done   chan struct{}
	err    error
	cancel context.CancelFunc
}

func (node *inProcessNode) Done() <-chan struct{} { return node.done }
func (node *inProcessNode) Err() error            { return node.err }
func (node *inProcessNode) Kill() error {
	node.cancel()
	<-node.done
	return nil
}

func TestRun_TheNodeScansItsRootAndTheCoreHearsEveryRecord(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	media := t.TempDir()
	for name, contents := range map[string]string{"Film (2020)/Film (2020).mkv": "film", "notes.txt": "notes"} {
		path := filepath.Join(media, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// One reply serves both runs the node makes: `ffprobe -version`, whose first line becomes
	// the hello's FFmpeg version, and the probe of the film, which parses.
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{
		Stdout: `{"format":{"format_name":"matroska","duration":"1.000"},"streams":[]}`,
	})
	config.Node.Roots = []string{media}
	config.Node.FFmpegDir = fake.Directory
	config.Node.VideoExtensions = []string{"mkv"}
	nodeID := createLibrary(t, config, media)

	captured := newCapturedLog("ready", "a node finished a scan")
	var start nodes.Starter = func(ctx context.Context) (nodes.Process, error) {
		ctx, cancel := context.WithCancel(ctx)
		node := &inProcessNode{done: make(chan struct{}), cancel: cancel}
		go func() {
			defer close(node.done)
			node.err = runNode(ctx, config, captured.logger)
		}()
		return node, nil
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, config, ConfigReport{}, listener, start, captured.logger) }()
	select {
	case <-captured.signals["a node finished a scan"]:
	case early := <-done:
		cancel()
		t.Fatalf("serve returned before the scan finished: %v\n%s", early, captured)
	}
	cancel()
	if stopped := <-done; stopped != nil {
		t.Errorf("serve returned %v", stopped)
	}

	log := captured.String()
	for _, want := range []string{
		`"msg":"started the node"`,
		`"msg":"a node said hello"`,
		`"ffmpeg":"{\"format\"`,
		`"msg":"a node finished a scan"`,
		`"observed":1,"skipped":1,"errors":0`,
	} {
		if !strings.Contains(log, want) {
			t.Errorf("the log lacks %s:\n%s", want, log)
		}
	}
	database := openDatabase(t, config)
	files, err := database.Read().ListLiveFilesByNode(t.Context(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	filings := map[string]string{}
	for _, file := range files {
		filings[strings.TrimPrefix(file.Path, media+"/")] = file.Filing
	}
	film, notes := filings["Film (2020)/Film (2020).mkv"], filings["notes.txt"]
	if film != "unidentified" || notes != "skipped" || len(filings) != 2 {
		t.Errorf("library.db holds %v; want the film unidentified and the notes skipped", filings)
	}
}

// createLibrary makes the database serve will open, with a library covering root on the
// config's node, and returns the node's id.
func createLibrary(t *testing.T, config Config, root string) int64 {
	t.Helper()
	database := openDatabase(t, config)
	if err := database.Migrate(t.Context(), config.migrateOptions(t.TempDir(), nil)); err != nil {
		t.Fatal(err)
	}
	nodeID, enrolErr := nodes.Enrol(t.Context(), database, clock.System{}, config.Node.Name)
	if enrolErr != nil {
		t.Fatal(enrolErr)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = library.Create(t.Context(), database, clock.System{}, library.NewLibrary{
		Name: "Films", Kind: "movies", MetadataSource: "nfo", ChangeDetection: "manual",
		NodeID: nodeID, Roots: []string{resolved},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	return nodeID
}

func openDatabase(t *testing.T, config Config) *store.Database {
	t.Helper()
	directories, err := datadir.Prepare(config.Server.DataDir, config.Server.CacheDir)
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), directories.LibraryDatabase(), config.openOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() }) //nolint:errcheck // closing twice is harmless here
	return database
}

func TestRunNode_RefusesToRunUnconfinedWhenConfinementIsRequired(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	config.Node.Confinement = "require"
	captured := newCapturedLog()
	if err := runNode(t.Context(), config, captured.logger); err == nil || !strings.Contains(err.Error(), "M3") {
		t.Errorf("runNode = %v, want a refusal saying confinement comes in M3", err)
	}
}
