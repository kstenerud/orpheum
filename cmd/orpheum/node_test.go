// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/nodeproto"
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

func TestRunNode_TheNodeScansItsRootAndTheCoreHearsEveryRecord(t *testing.T) {
	t.Parallel()
	scanned := scanOnce(t, "nfo", "a node finished a scan")
	config, captured, nodeID, media := scanned.config, scanned.log, scanned.nodeID, scanned.media
	log := captured.String()
	for _, want := range []string{
		`"msg":"started the node"`,
		`"msg":"a node said hello"`,
		`"ffmpegVersion":"{\"format\"`,
		`"msg":"a node finished a scan"`,
		`"observedCount":1,"sidecarCount":0,"skippedCount":1,"errorCount":0`,
		`"msg":"identified a library"`,
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
		t.Errorf("library.db holds %v; want the film unidentified (no NFO, and the library reads NFO only) "+
			"and the notes skipped", filings)
	}
}

// scanResult is what scanOnce ran.
type scanResult struct {
	config Config
	log    capturedLog
	nodeID int64
	media  string
}

// scanOnce serves a library over a media root holding one film and a text file, runs the node
// in-process until the log says awaited, and stops.
func scanOnce(t *testing.T, metadataSource, awaited string) scanResult {
	t.Helper()
	shelved := filmOnNode(t, metadataSource)
	config, media, nodeID := shelved.config, shelved.media, shelved.nodeID
	captured := newCapturedLog("ready", awaited)
	server := serveWithNode(t, config, captured, awaited)
	if stopped := server.stop(); stopped != nil {
		t.Errorf("serve returned %v", stopped)
	}
	return scanResult{config: config, log: captured, nodeID: nodeID, media: media}
}

func TestRunNode_AFilmInATMDBLibraryIsQueuedForItsMatch(t *testing.T) {
	t.Parallel()
	scanned := scanOnce(t, "tmdb", "queued fetches")
	config := scanned.config
	if log := scanned.log.String(); !strings.Contains(log, `"awaitingMatch":1`) {
		t.Errorf("the log does not say one film awaits its match:\n%s", log)
	}
	cache, err := store.OpenCache(t.Context(), filepath.Join(config.Server.CacheDir, "cache.db"), config.openOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cache.Close() }() //nolint:errcheck // read only
	jobs, err := cache.Read().ListJobs(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Kind != "fetch_metadata" || jobs[0].State == "failed" {
		t.Errorf("cache.db holds %+v; want one fetch_metadata job, waiting (no TMDB key is set)", jobs)
	}
}

// filmFixture is a config whose node has one root and a library over it.
type filmFixture struct {
	config     Config
	configPath string
	media      string
	nodeID     int64
}

// filmOnNode makes a config whose node has one root, holding a film and a text file, and a
// library over it; the fake FFmpeg probes the film as Matroska with no streams.
func filmOnNode(t *testing.T, metadataSource string) filmFixture {
	t.Helper()
	return filmProbedAs(t, metadataSource, `{"format":{"format_name":"matroska","duration":"1.000"},"streams":[]}`)
}

// filmProbedAs is filmOnNode with the fake FFmpeg answering every run with reply: the probe of
// the film, and whatever else the node runs.
func filmProbedAs(t *testing.T, metadataSource, reply string) filmFixture {
	t.Helper()
	return filmAnswered(t, metadataSource, testutil.FakeReply{Stdout: reply})
}

// filmAnswered is filmProbedAs with the whole of the fake FFmpeg's reply: what it writes to
// stderr too, where FFmpeg's filters log what they measured.
func filmAnswered(t *testing.T, metadataSource string, reply testutil.FakeReply) filmFixture {
	t.Helper()
	config, configPath := testConfig(t)
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
	// One reply serves every run the node makes: `ffprobe -version`, whose first line becomes
	// the hello's FFmpeg version, and the probe of the film, which parses.
	fake := testutil.InstallFakeFFmpeg(t, reply)
	config.Node.RootPaths = []string{media}
	config.Node.FFmpegDir = fake.Directory
	config.Node.VideoExtensions = []string{"mkv"}
	nodeID := enrolNodeWithLibrary(t, config, media,
		libraryShape{name: "Films", kind: "movies", metadataSource: metadataSource})
	return filmFixture{config: config, configPath: configPath, media: media, nodeID: nodeID}
}

// serveWithNode serves the core with the node running in-process on the fake FFmpeg, until the
// log says awaited; the returned stop ends both, and so does the test's end.
func serveWithNode(t *testing.T, config Config, captured capturedLog, awaited string) runningServer {
	t.Helper()
	return startServing(t, config, captured, awaited, inProcessStarter(func(ctx context.Context) error {
		return runNodeWith(ctx, config, captured.logger, fakeFFmpegSeams)
	}))
}

// inProcessStarter is a node starter that runs a node's loop in this process.
func inProcessStarter(run func(ctx context.Context) error) nodes.Starter {
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

// libraryShape is what enrolNodeWithLibrary's library is: its name, kind and metadata source.
type libraryShape struct {
	name           string
	kind           string
	metadataSource string
}

// enrolNodeWithLibrary makes the database serve will open, enrols the config's node and creates
// a library of shape over root on it, and returns the node's id. The library's root is root with
// its symbolic links resolved, as the node names its roots in its hello (confine.OpenMediaRoot):
// the core refuses a library root no connected node has, and a temporary directory can sit under
// a link (macOS's /var).
func enrolNodeWithLibrary(t *testing.T, config Config, root string, shape libraryShape) int64 {
	t.Helper()
	directories, err := datadir.Prepare(config.Server.DataDir, config.Server.CacheDir)
	if err != nil {
		t.Fatal(err)
	}
	database, err := openAndMigrate(t.Context(), config, directories, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := database.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	nodeID, err := nodes.Enrol(t.Context(), database, clock.System{}, config.Node.Name)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = library.Create(t.Context(), database, clock.System{}, library.NewLibrary{
		Name: shape.name, Kind: shape.kind, MetadataSource: shape.metadataSource, ChangeDetection: "manual",
		NodeID: nodeID, Roots: []string{resolved},
	})
	if err != nil {
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
	database, err := store.Open(t.Context(), directories.LibraryDatabasePath(), config.openOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() }) //nolint:errcheck // closing twice is harmless here
	return database
}

// fakeFFmpegSeams run the node with the fake FFmpeg, a shell script no confined run may start.
var fakeFFmpegSeams = nodeSeams{isUnconfined: true}

func TestRunNode_RefusesToRunUnconfinedWhenConfinementIsRequired(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	config.Node.ConfinementPolicy = "require"
	captured := newCapturedLog()
	withoutLandlock := nodeSeams{isUnconfined: true, mechanismStates: func() []confine.State {
		return []confine.State{
			{Mechanism: confine.MechanismLandlock, Reason: "not enabled at boot", Remedy: "add lsm=landlock"},
			{Mechanism: confine.MechanismSeccomp, IsInForce: true},
		}
	}}
	err := runNodeWith(t.Context(), config, captured.logger, withoutLandlock)
	if err == nil || !strings.Contains(err.Error(), "not every mechanism is in force") {
		t.Errorf("runNode = %v, want a refusal saying a mechanism is not in force", err)
	}
	if logged := captured.String(); !strings.Contains(logged, "add lsm=landlock") {
		t.Errorf("the log = %s, want the missing mechanism's remedy", logged)
	}
}

func TestRunNode_ServesItsRootsOnTheByteSocket(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	media := t.TempDir()
	if err := os.WriteFile(filepath.Join(media, "film.mkv"), []byte("film"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: "ffprobe version test"})
	config.Node.RootPaths, config.Node.FFmpegDir = []string{media}, fake.Directory
	const listening = "serving the byte socket"
	captured := newCapturedLog(listening)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runNodeWith(ctx, config, captured.logger, fakeFFmpegSeams) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("runNode returned %v", err)
		}
	}()
	select {
	case <-captured.signals[listening]:
	case early := <-done:
		t.Fatalf("runNode returned before serving the byte socket: %v\n%s", early, captured)
	}
	socket := filepath.Join(config.Server.SocketDir, nodeSocketName)
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}}
	identity, err := hash.PayloadIdentity(bytes.NewReader([]byte("film")), 4)
	if err != nil {
		t.Fatal(err)
	}
	named := nodeproto.BytesFile{
		RootPath: media, Path: []byte("film.mkv"),
		PayloadHash: hex.EncodeToString(identity.PayloadHash[:]),
	}
	target := "http://node" + named.Target()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read to the end below
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || string(body) != "film" {
		t.Errorf("the byte socket answered %d %q (%v), want 200 with the film's bytes", response.StatusCode, body, err)
	}
}

func TestRunNode_StillServesFilesWhenItCannotPrepare(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	blocker := filepath.Join(t.TempDir(), "a file, not a directory")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	config.Node.PreparedDir = filepath.Join(blocker, "prepared")
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: "ffprobe version test"})
	config.Node.FFmpegDir = fake.Directory
	const listening, cannot = "serving the byte socket", "this node cannot prepare; it serves files as they are"
	captured := newCapturedLog(listening, cannot)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runNodeWith(ctx, config, captured.logger, fakeFFmpegSeams) }()
	for _, awaited := range []string{cannot, listening} {
		select {
		case <-captured.signals[awaited]:
		case early := <-done:
			cancel()
			t.Fatalf("runNode returned before logging %q: %v\n%s", awaited, early, captured)
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("runNode returned %v", err)
	}
}

func TestCapabilityNames_AreRemuxAACAndTheEncodersTheNodeFound(t *testing.T) {
	t.Parallel()
	got := capabilityNames([]prepare.Encoder{prepare.EncoderQuickSync, prepare.EncoderX264})
	if want := []string{"remux", "aac", "h264_qsv", "x264"}; !slices.Equal(got, want) {
		t.Errorf("capabilities = %v, want %v (cut-a §4)", got, want)
	}
}

func TestHardwareReport_IsTheDevicesEachTrialAndTheEncoderChosen(t *testing.T) {
	t.Parallel()
	found := nodeCapabilities{
		encoderChoice: prepare.Choice{Chosen: prepare.EncoderNVENC, Trials: []prepare.Trial{
			{Encoder: prepare.EncoderQuickSync, Failure: "no render node"}, {Encoder: prepare.EncoderNVENC},
		}},
		gpus: []nodeproto.GPU{{DevicePath: "/dev/nvidia0", Driver: "nvidia"}},
	}
	got := hardwareReport(found)
	want := nodeproto.Hardware{
		GPUs: found.gpus, ChosenEncoder: "h264_nvenc",
		Trials: []nodeproto.EncoderTrial{{Encoder: "h264_qsv", Failure: "no render node"}, {Encoder: "h264_nvenc"}},
	}
	if got.ChosenEncoder != want.ChosenEncoder || !slices.Equal(got.Trials, want.Trials) || !slices.Equal(got.GPUs,
		want.GPUs) {
		t.Errorf("the hello's hardware = %+v, want %+v (D225 §3)", got, want)
	}
}
