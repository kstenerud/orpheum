// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/node/agent"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

func TestRunNode_TheNodeScansItsRootAndTheCoreHearsEveryRecord(t *testing.T) {
	t.Parallel()
	scanned := scanOnce(t, "nfo", "a node finished a scan")
	config, captured, nodeID, media := scanned.config, scanned.log, scanned.nodeID, scanned.media
	log := captured.String()
	for _, want := range []string{
		testutil.EventInJSON("helloSaid"),
		testutil.EventInJSON("nodeHello"),
		`"ffmpegVersion":"{\"format\"`,
		// What the root is on: some device, of a kind, mounted at or above it (D240 §5).
		`"devices":[{"key":`,
		testutil.EventInJSON("nodeScanFinished"),
		`"listedCount":2,"examinedCount":1,"errorCount":0`,
		// The node watches its watched library's folder for changes, the folder and the one in it, as
		// the core names it (D272 §9, D359's amendment of 2026-10-27).
		testutil.EventInJSON("folderWatchStarted"),
		`"watchCount":2`,
		testutil.EventInJSON("libraryIdentified"),
	} {
		if !strings.Contains(log, want) {
			t.Errorf("the log lacks %s:\n%s", want, log)
		}
	}
	filings := map[string]string{}
	for _, file := range liveFilingsOfNode(t, config, nodeID) {
		filings[strings.TrimPrefix(file.path, media+"/")] = file.filing
	}
	film, notes := filings["Film (2020)/Film (2020).mkv"], filings["notes.txt"]
	if film != "unidentified" || notes != "skipped" || len(filings) != 2 {
		t.Errorf("persistent.db holds %v; want the film unidentified (no NFO, and the library reads NFO only) "+
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
	watchItsLibrary(t, config)
	captured := newCapturedLog("ready", awaited)
	server := serveWithNode(t, config, captured, awaited)
	if stopped := server.stop(); stopped != nil {
		t.Errorf("serve returned %v", stopped)
	}
	return scanResult{config: config, log: captured, nodeID: nodeID, media: media}
}

func TestRunNode_AFilmInATMDBLibraryIsQueuedForItsMatch(t *testing.T) {
	t.Parallel()
	shelved := filmOnNode(t, "tmdb")
	config := shelved.config
	// The film's match is queued by the identification a new file asks for, while the scan goes on,
	// or by the one at its end, whichever comes first (D236 §1).
	const identified, queued = "identified a library", "queued fetches"
	captured := newCapturedLog("ready", identified, queued)
	server := serveWithNode(t, config, captured, identified)
	testutil.Receive(t, captured.signals[queued], "the film's match queued")
	if stopped := server.stop(); stopped != nil {
		t.Errorf("serve returned %v", stopped)
	}
	if log := captured.String(); !strings.Contains(log, `"awaitingMatch":1`) {
		t.Errorf("the log does not say one film awaits its match:\n%s", log)
	}
	cachePath := filepath.Join(config.Instance.CacheDir, "cache.db")
	cache, err := store.OpenCache(t.Context(), cachePath, config.openOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cache.Close() }() //nolint:errcheck // read only
	jobs, err := cache.Read().ListJobs(t.Context(), cachegen.ListJobsParams{MaxRows: 10})
	if err != nil {
		t.Fatal(err)
	}
	fetches := slices.DeleteFunc(jobs, func(job cachegen.Job) bool { return job.Kind != "fetch_metadata" })
	if len(fetches) != 1 || fetches[0].State == "failed" {
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
	nodeID := enrolNodeWithLibrary(t, config, media,
		libraryShape{name: "Films", kind: "movie", metadataSource: metadataSource})
	return filmFixture{config: config, configPath: configPath, media: media, nodeID: nodeID}
}

// serveWithNode serves the core with the node running in-process on the fake FFmpeg, until the
// log says awaited; the returned stop ends both, and so does the test's end.
func serveWithNode(t *testing.T, config Config, captured capturedLog, awaited string) runningServer {
	t.Helper()
	return startServing(t, config, captured, awaited, func(ctx context.Context) error {
		return runNodeWith(ctx, config, captured.logger, fakeFFmpegSeams)
	})
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
	directories, err := instancedir.Open(config.Instance.paths())
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

// watchItsLibrary has the config's one library watch for changes, as an admin would set it, so its
// node watches its folder (D272 §9).
func watchItsLibrary(t *testing.T, config Config) {
	t.Helper()
	database := openDatabase(t, config)
	libraries, err := database.Read().ListLibraries(t.Context())
	if err != nil || len(libraries) != 1 {
		t.Fatalf("the libraries are %+v, %v; want one", libraries, err)
	}
	err = database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		_, setErr := queries.SetLibraryChangeDetection(t.Context(), sqlcgen.SetLibraryChangeDetectionParams{
			ChangeDetection: "watch", ID: libraries[0].ID,
		})
		return setErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
}

// liveFiling is one live place on a node and how it is filed: a video's filing by the kind of library
// that covers it, else its own row's (D363 §6).
type liveFiling struct {
	id              int64
	path            string
	reportedPathRaw string
	filing          string
	reason          string
}

// liveFilingsOfNode is every live place on nodeID with its filing, one per place: the tests that
// read them cover each with one library.
func liveFilingsOfNode(t *testing.T, config Config, nodeID int64) []liveFiling {
	t.Helper()
	rows := storetest.QueryRaw(t, openDatabase(t, config).Path(), fmt.Sprintf(
		`SELECT place.id, hex(CAST(place.path AS BLOB)), hex(place.reported_path_raw),
		 coalesce(file_filing.filing, `+filingWord+`), coalesce(file_filing.filing_reason, place.filing_reason, '')
		 FROM place JOIN file ON file.id = place.file_id
		 LEFT JOIN file_filing ON file_filing.file_id = place.file_id AND place.id = place.file_id
		 WHERE place.node_id = %d AND place.removed_at IS NULL ORDER BY place.path`, nodeID))
	filings := make([]liveFiling, 0, len(rows))
	for _, row := range rows {
		fields := strings.Split(row, "|")
		id, err := strconv.ParseInt(fields[0], 10, 64)
		path, pathErr := hex.DecodeString(fields[1])
		raw, rawErr := hex.DecodeString(fields[2])
		if err = errors.Join(err, pathErr, rawErr); err != nil || len(fields) != 5 {
			t.Fatalf("read the live filing %q: %v", row, err)
		}
		filings = append(filings, liveFiling{
			id: id, path: string(path), reportedPathRaw: string(raw), filing: fields[3], reason: fields[4],
		})
	}
	return filings
}

// filingWord is how a place is filed, in the words the API answers with: skipped or errored as it
// was stored, else attached for a sidecar, copy for a place holding a file found first elsewhere, and
// video (D426 section 4).
const filingWord = "CASE WHEN place.filing <> 'read' THEN place.filing " +
	"WHEN file.sidecar_kind IS NOT NULL THEN 'attached' " +
	"WHEN place.id IS NOT place.file_id THEN 'copy' ELSE 'video' END"

func openDatabase(t *testing.T, config Config) *store.Database {
	t.Helper()
	directories, err := instancedir.Open(config.Instance.paths())
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), directories.PersistentDatabasePath(), config.openOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() }) //nolint:errcheck // closing twice is harmless here
	return database
}

// fakeFFmpegSeams run the node with the fake FFmpeg, a shell script no confined run may start, posting
// its status every second (nodeSeams.statusEvery).
var fakeFFmpegSeams = nodeSeams{isUnconfined: true, statusEvery: time.Second}

// The byte socket serves its roots to the holder of the key it keeps beside itself, as the core
// dials it, and nothing to a process of the same user without it: a GPU run may open a Unix socket
// (D331 §1).
func TestRunNode_ServesItsRootsOnTheByteSocketToTheHolderOfItsKeyOnly(t *testing.T) {
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
		if err := testutil.Receive(t, done, "the node to stop"); err != nil {
			t.Errorf("runNode returned %v", err)
		}
	}()
	select {
	case <-captured.signals[listening]:
	case early := <-done:
		t.Fatalf("runNode returned before serving the byte socket: %v\n%s", early, captured)
	}
	socket := filepath.Join(config.Instance.RunDir, nodeSocketName)
	stranger := &http.Client{Transport: &http.Transport{
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
	refused, err := stranger.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = refused.Body.Close() //nolint:errcheck // only the status matters
	if refused.StatusCode != http.StatusUnauthorized {
		t.Errorf("the byte socket answered a request without its key %d; want 401", refused.StatusCode)
	}
	response, err := unixsocket.Client(socket, 0).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read to the end below
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || string(body) != "film" {
		t.Errorf("the byte socket answered %d %q (%v), want 200 with the film's bytes", response.StatusCode, body, err)
	}
}

func TestRunNode_StillServesFilesWhenItCannotMakeChunks(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	blocker := filepath.Join(t.TempDir(), "a file, not a directory")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	config.Instance.CacheDir = blocker // its prepared directory cannot be made under a file
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: "ffprobe version test"})
	config.Node.FFmpegDir = fake.Directory
	const listening, cannot = "serving the byte socket", "this node cannot make chunks; it serves files as they are"
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
	if err := testutil.Receive(t, done, "the node to stop"); err != nil {
		t.Errorf("runNode returned %v", err)
	}
}

func TestCapabilityNames_AreRemuxAACAndTheEncodersTheNodeFound(t *testing.T) {
	t.Parallel()
	got := capabilityNames([]encode.Encoder{encode.EncoderQuickSync, encode.EncoderX264})
	if want := []string{"remux", "aac", "h264_qsv", "x264"}; !slices.Equal(got, want) {
		t.Errorf("capabilities = %v, want %v (cut-a §4)", got, want)
	}
}

func TestHardwareReport_IsTheDevicesEachTrialTheEncoderChosenAndItsSpeed(t *testing.T) {
	t.Parallel()
	found := nodeCapabilities{
		encoderChoice: encode.Choice{Chosen: encode.EncoderNVENC, Trials: []encode.Trial{
			{Encoder: encode.EncoderQuickSync, Failure: "no render node"}, {Encoder: encode.EncoderNVENC},
		}},
		encodeSpeedPercent: 1200, gpus: []nodeproto.GPU{{DevicePath: "/dev/nvidia0", Driver: "nvidia"}},
	}
	got := hardwareReport(found)
	want := nodeproto.Hardware{
		GPUs: found.gpus, ChosenEncoder: "h264_nvenc", EncodeSpeedPercent: 1200,
		Trials: []nodeproto.EncoderTrial{{Encoder: "h264_qsv", Failure: "no render node"}, {Encoder: "h264_nvenc"}},
	}
	if got.ChosenEncoder != want.ChosenEncoder || !slices.Equal(got.Trials, want.Trials) || !slices.Equal(got.GPUs,
		want.GPUs) || got.EncodeSpeedPercent != want.EncodeSpeedPercent {
		t.Errorf("the hello's hardware = %+v, want %+v (D225 §3, D457)", got, want)
	}
}

func TestNewStartID_IsEightRandomBytesInHex(t *testing.T) {
	t.Parallel()
	first, err := newStartID(bytes.NewReader([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
	if err != nil {
		t.Fatal(err)
	}
	if first != "0102030405060708" {
		t.Errorf("a start id = %q, want its eight random bytes in hex", first)
	}
	if _, err := newStartID(bytes.NewReader([]byte{1, 2})); err == nil {
		t.Error("a start id from too few random bytes was made, want an error")
	}
}

func TestWorkPacing_IsTheNodesConfig(t *testing.T) {
	t.Parallel()
	var config Config
	config.Node.EndedWorkKeptHours, config.Node.ProbeTimeoutSeconds = 5, 60
	endedWorkKept, readStallWindow := workPacing(config)
	if endedWorkKept != 5*time.Hour {
		t.Errorf("the node's ended work is kept %v; want 5h from ended_work_kept_hours", endedWorkKept)
	}
	// An examining does not move while ffprobe reads one file, so its read window outlasts the probe.
	if readStallWindow != agent.ReadStallWindowFor(time.Minute) || readStallWindow <= time.Minute {
		t.Errorf("the node's read window = %v; want past probe_timeout_seconds' minute", readStallWindow)
	}
}

func TestPreparedBudget_IsTheNodesConfig(t *testing.T) {
	t.Parallel()
	var config Config
	config.Node.PreparedBudgetGiB, config.Node.PreparedKeptAfterPlayHours = 2, 3
	config.Node.PreparedDiskUsedLimitPercent, config.Node.PreparedDiskMinFreeMiB = 90, 100
	budget := preparedBudget(config)
	if budget.MaxBytes != 2<<30 || budget.KeptAfterLastPlay != 3*time.Hour || budget.DiskUsedLimitPercent != 90 ||
		budget.MinFreeBytes != 100<<20 {
		t.Errorf("the prepared budget = %+v; want 2 GiB, kept 3 h, 90%% full at most and 100 MiB kept free", budget)
	}
}

// hangingListings lists nothing, as a listing of a disk that does not answer, until its job ends or
// the test does.
type hangingListings struct {
	agent.Scanner
	testCtx context.Context //nolint:containedctx // the test's, which bounds the hang
}

func (scanner hangingListings) List(ctx context.Context, _ *confine.MediaRoot, _ nodeproto.List) (
	[]nodeproto.ScanRecord, error,
) {
	// Answered as stalled, its job is ended, which ends ctx: the stall is answered first.
	select {
	case <-ctx.Done():
	case <-scanner.testCtx.Done():
	}
	return nil, nil
}

// A listing whose reading stops moving is answered as stalled on its disk, and the server makes the
// root unavailable and asks for a scan of it: the stall reaches the scans through the exchanges
// (D233 §10).
func TestRunNode_AListingStalledOnItsDiskMakesItsRootUnavailable(t *testing.T) {
	t.Parallel()
	shelved := filmOnNode(t, "nfo")
	config := shelved.config
	const stalled = "work stalled on a disk; it is unavailable until it answers or a scan reads it"
	captured := newCapturedLog("ready", stalled)
	seams := fakeFFmpegSeams
	seams.readStallWindow = 100 * time.Millisecond
	seams.wrapScanner = func(inner agent.Scanner) agent.Scanner {
		return hangingListings{Scanner: inner, testCtx: t.Context()}
	}
	server := startServing(t, config, captured, stalled, func(ctx context.Context) error {
		return runNodeWith(ctx, config, captured.logger, seams)
	})
	if stopped := server.stop(); stopped != nil {
		t.Errorf("serve returned %v", stopped)
	}
	if log := captured.String(); !strings.Contains(log, `"rootPath":"`+shelved.media) {
		t.Errorf("the log does not name the stalled root, %s:\n%s", shelved.media, log)
	}
}

// The node's roots open through a gate keyed by the disks mounted inside them, so one stuck inside a
// root holds up only what is on it (D328 §1): /proc is mounted inside / on any Linux, and a reading
// held under it leaves / itself free.
func TestOpenRoots_KeysTheGateByTheDisksMountedInsideARoot(t *testing.T) {
	t.Parallel()
	roots, gate := openRoots(t.Context(), []string{"/"}, slog.New(slog.DiscardHandler))
	t.Cleanup(func() {
		for _, root := range roots {
			_ = root.Close() //nolint:errcheck // read-only
		}
	})
	release, isHeld := gate.Hold("/", "/proc/self/status")
	if !isHeld {
		t.Fatal("/proc could not be held")
	}
	t.Cleanup(release)
	if stuck, isStuck := gate.IsStuck("/", "/"); isStuck {
		t.Errorf("/ itself reads as stuck on %+v; want only /proc, the disk held inside it", stuck)
	}
	if _, isStuck := gate.IsStuck("/", "/proc/1/status"); !isStuck {
		t.Error("/proc reads as free; want it held")
	}
}

// A node makes its system folder in its persistent directory and serves it as a root beside its
// media, so the core can scan the server's own videos as a library's (D435 §4).
func TestOpenNodeRoots_ServesTheSystemFolderBesideTheMedia(t *testing.T) {
	t.Parallel()
	media, persistent := t.TempDir(), t.TempDir()
	config := Config{Node: NodeConfig{RootPaths: []string{media}}, Instance: Instance{PersistentDir: persistent}}
	roots, _, systemFolder := openNodeRoots(t.Context(), config, slog.New(slog.DiscardHandler))
	t.Cleanup(func() {
		for _, root := range roots {
			_ = root.Close() //nolint:errcheck // read-only
		}
	})
	want, err := filepath.EvalSymlinks(filepath.Join(persistent, "system-videos"))
	if err != nil {
		t.Fatalf("the system folder was not made: %v", err)
	}
	if systemFolder != want {
		t.Errorf("the system folder is %q; want %q", systemFolder, want)
	}
	served := make([]string, 0, len(roots))
	for _, root := range roots {
		served = append(served, root.Path())
	}
	if !slices.Contains(served, want) || len(served) != 2 {
		t.Errorf("the node serves %q; want its media and its system folder %q", served, want)
	}
}
