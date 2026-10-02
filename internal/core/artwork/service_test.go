// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/jobs/jobstest"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A WebP as our encoder writes one, as far as the service looks: a RIFF container of WebP.
var ourWebP = append([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), bytes.Repeat([]byte{7}, 16)...)

// fakeNode is the node channel with one node that answers each artwork command as answer says.
type fakeNode struct {
	service     *Service
	ctx         context.Context //nolint:containedctx // the test's, for the answers it sends
	isConnected bool
	answer      func(nodeproto.Artwork) nodeproto.ArtworkResult

	mutex      sync.Mutex
	sent       []nodeproto.Artwork
	sentJobIDs []int64       // the job each command in sent was for, as its envelope named it
	cancelled  []int64       // the jobs a cancel was sent for
	enqueued   chan struct{} // told of each artwork command, when set
}

// fakeNodeID is the fake's one node: the first a test's database enrols.
const fakeNodeID = 1

func (node *fakeNode) ConnectedNodeID(context.Context) (int64, bool) {
	return fakeNodeID, node.isConnected
}

// Send parks the job on a node that is not connected, as nodes.Service does.
func (node *fakeNode) Send(_ context.Context, nodeID int64, command nodeproto.Command) error {
	if nodeID != fakeNodeID || !node.isConnected {
		return jobs.ParkOnNode(nodeID, time.Time{}, errors.New("not connected"))
	}
	node.Enqueue(nodeID, command)
	return nil
}

func (*fakeNode) Unanswered(_ context.Context, _ int64, err error) error { return err }

func (node *fakeNode) Enqueue(_ int64, command nodeproto.Command) {
	node.mutex.Lock()
	if command.Type == nodeproto.CommandCancel {
		node.cancelled = append(node.cancelled, command.JobID)
		node.mutex.Unlock()
		return
	}
	node.sent = append(node.sent, *command.EncodeArtwork)
	node.sentJobIDs = append(node.sentJobIDs, command.JobID)
	node.mutex.Unlock()
	if node.enqueued != nil {
		node.enqueued <- struct{}{}
	}
	if node.answer != nil {
		result := node.answer(*command.EncodeArtwork)
		echoed := command.Work
		go func() {
			answered := nodeproto.Result{
				JobID: command.JobID, AttemptToken: command.AttemptToken, Work: &echoed, Artwork: &result,
			}
			// An answer not taken leaves nothing stored, which the test's own checks find.
			_ = node.service.exchanges.Deliver(node.ctx, fakeNodeID, answered) //nolint:errcheck // as above
		}()
	}
}

// fakeFetcher is a source's images by reference.
type fakeFetcher map[string][]byte

func (fakeFetcher) RecheckAfter() time.Duration { return 12 * time.Hour }

func (fakeFetcher) Destination() jobs.Destination { return jobs.DestinationTMDBImages }

func (images fakeFetcher) Fetch(_ context.Context, ref string) (Unsanitised, error) {
	image, found := images[ref]
	if !found {
		return Unsanitised{}, ErrNoImage
	}
	return Received(image), nil
}

// memoryStore is the art directory in memory.
type memoryStore map[string][]byte

func (art memoryStore) WriteArt(sourceSha256Hex, name string, contents []byte) error {
	art[sourceSha256Hex+"/"+name] = contents
	return nil
}

func (art memoryStore) ReadArt(sourceSha256Hex, name string) ([]byte, error) {
	contents, found := art[sourceSha256Hex+"/"+name]
	if !found {
		return nil, datadir.ErrNotFound
	}
	return contents, nil
}

func (art memoryStore) RemoveArt(sourceSha256Hex, name string) error {
	delete(art, sourceSha256Hex+"/"+name)
	return nil
}

func (art memoryStore) ListArt() ([]datadir.ArtFile, error) {
	var files []datadir.ArtFile
	for key, contents := range art {
		sourceSha256Hex, name, _ := strings.Cut(key, "/")
		files = append(files,
			datadir.ArtFile{SourceSha256Hex: sourceSha256Hex, Name: name, SizeBytes: int64(len(contents))})
	}
	return files, nil
}

func (art memoryStore) HasArt(sourceSha256Hex, name string) (bool, error) {
	_, found := art[sourceSha256Hex+"/"+name]
	return found, nil
}

type bench struct {
	service  *Service
	database *store.Database
	node     *fakeNode
	art      memoryStore
	itemID   int64
	settings *settings.Service
	clock    *clocktest.Fake
}

func newBench(t *testing.T, images fakeFetcher) *bench {
	t.Helper()
	database := storetest.OpenMigrated(t)
	var itemID int64
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		itemID, err = queries.InsertItem(t.Context(), sqlcgen.InsertItemParams{Kind: "title", AddedAt: 1})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	node := &fakeNode{ctx: t.Context(), isConnected: true}
	art := memoryStore{}
	config := settingstest.Seeded(t, database)
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC))
	service := NewService(Parts{
		Database: database, Queue: &jobstest.Queue{}, Nodes: node, ArtFiles: art,
		Exchanges: nodes.NewExchanges(node, fakeClock, rand.Reader, slog.New(slog.DiscardHandler)),
		Cache:     storetest.OpenCache(t),
		Settings:  config,
		Fetchers:  map[string]Fetcher{"tmdb": images},
		Clock:     fakeClock,
		Logger:    slog.New(slog.DiscardHandler), AnswerWait: 5 * time.Second,
	})
	node.service = service
	return &bench{
		service: service, database: database, node: node, art: art, itemID: itemID,
		settings: config, clock: fakeClock,
	}
}

func (bench *bench) job(refs ...Ref) jobs.Job {
	args, err := json.Marshal(artworkJobArgs{ArtworkSource: "tmdb", Refs: refs})
	if err != nil {
		panic(err)
	}
	return jobs.Job{
		ID: 1, Kind: jobs.KindMakeArtworkSizes, Target: jobs.Target{ItemID: bench.itemID}, ArgsJSON: string(args),
	}
}

func (bench *bench) handle(t *testing.T, refs ...Ref) {
	t.Helper()
	if err := bench.service.Handle(t.Context(), bench.job(refs...)); err != nil {
		t.Fatalf("%v: %v", refs, err)
	}
}

func (bench *bench) rows(t *testing.T) []sqlcgen.ListLiveArtworkRow {
	t.Helper()
	rows, err := bench.database.Read().ListLiveArtwork(t.Context(), bench.itemID)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestService_StoresOnlyTheNodesAnswer(t *testing.T) {
	t.Parallel()
	source := []byte("a provider's JPEG, with whatever else it carries")
	bench := newBench(t, fakeFetcher{"/poster.jpg": source})
	bench.node.answer = func(nodeproto.Artwork) nodeproto.ArtworkResult {
		return nodeproto.ArtworkResult{
			SourceWidth: 600, SourceHeight: 900,
			Images: []nodeproto.ArtImage{
				{SizeName: nodeproto.PortraitS, WebP: ourWebP},
				{SizeName: nodeproto.PortraitM, WebP: source},    // not WebP: never stored
				{SizeName: nodeproto.LandscapeS, WebP: ourWebP},  // not asked for
				{SizeName: "../../state/library", WebP: ourWebP}, // not a size at all
			},
		}
	}
	bench.handle(t, Ref{Kind: library.ArtPoster, SourceRef: "/poster.jpg"})
	sent := bench.node.sent
	if len(sent) != 1 || !bytes.Equal(sent[0].SourceImage, source) || sent[0].Shape != nodeproto.ShapePortrait ||
		!slices.Equal(sent[0].SizeNames, []nodeproto.ArtSizeName{nodeproto.PortraitS, nodeproto.PortraitM}) ||
		sent[0].MaxSourcePixels != 40_000_000 || sent[0].WebPQuality != 82 ||
		sent[0].SourceSha256 != Received(source).sourceSha256Hex() {
		t.Fatalf("sent %+v; want the source and its hash, portrait, the sizes made when fetched, and the settings",
			sent)
	}
	checkStored(t, bench.art, source)
	rows := bench.rows(t)
	if len(rows) != 1 || rows[0].Kind != "poster" || rows[0].Source != "tmdb" || *rows[0].SourceRef != "/poster.jpg" ||
		len(rows[0].SourceSha256) != 32 {
		t.Errorf("rows %+v; want the poster with its hash", rows)
	}
}

// checkStored checks the store holds only portrait-s, our WebP, under the source's hash.
func checkStored(t *testing.T, art memoryStore, source []byte) {
	t.Helper()
	sourceSha256Hex := Received(source).sourceSha256Hex()
	if len(art) != 1 || !bytes.Equal(art[sourceSha256Hex+"/portrait-s.webp"], ourWebP) {
		t.Errorf("stored %v; want only portrait-s, our WebP, under the source's hash", storedNames(art))
	}
	for _, stored := range art {
		if bytes.Contains(stored, source) {
			t.Error("the source's own bytes reached the store")
		}
	}
}

func storedNames(art memoryStore) []string {
	var names []string
	for name := range art {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func TestService_RecordsARefusalAndKeepsTheOlderImage(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{"/old.jpg": []byte("old"), "/new.jpg": []byte("<html>")})
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		if string(command.SourceImage) == "old" {
			return nodeproto.ArtworkResult{Images: []nodeproto.ArtImage{
				{SizeName: nodeproto.PortraitS, WebP: ourWebP},
			}}
		}
		return nodeproto.ArtworkResult{Refusal: "not an image"}
	}
	for _, ref := range []string{"/old.jpg", "/new.jpg", "/gone.jpg"} {
		bench.handle(t, Ref{Kind: library.ArtPoster, SourceRef: ref})
	}
	rows := bench.rows(t)
	if len(rows) != 3 {
		t.Fatalf("rows %+v; want the old poster, and the refused and missing ones held with no hash", rows)
	}
	shown, err := bench.database.Read().ListShownArtwork(t.Context(), sqlcgen.ListShownArtworkParams{
		ItemIDListJSON: "[" + strconv.FormatInt(bench.itemID, 10) + "]", Kind: "poster",
	})
	if err != nil || len(shown) == 0 || !bytes.Equal(shown[0].SourceSha256, Received([]byte("old")).sourceSha256()) {
		t.Errorf("shown %+v (%v); want the old poster", shown, err)
	}
	sent := len(bench.node.sent)
	bench.handle(t, Ref{Kind: library.ArtPoster, SourceRef: "/new.jpg"})
	if len(bench.node.sent) != sent {
		t.Error("a refused image was sent to the node again")
	}
}

func TestService_WaitsForANode(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{"/poster.jpg": []byte("x")})
	bench.node.isConnected = false
	err := bench.service.Handle(t.Context(), bench.job(Ref{Kind: library.ArtPoster, SourceRef: "/poster.jpg"}))
	if parking, isParked := jobqueue.ParkingOf(err); !isParked || parking.Destination != jobs.DestinationAnyNode ||
		!parking.AwaitsSignal {
		t.Errorf("with no node: %v, want it parked until any node says hello", err)
	}
}

// A job waits for the node's answer by its own id, and cancelled while it waits, it tells the node
// to stop that job.
func TestService_ACancelledJobTellsTheNodeToStopItByItsID(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{"/poster.jpg": []byte("x")})
	bench.node.enqueued = make(chan struct{}, 1)
	job := bench.job(Ref{Kind: library.ArtPoster, SourceRef: "/poster.jpg"})
	job.ID = 41
	ctx, cancel := context.WithCancelCause(t.Context())
	ended := make(chan error, 1)
	go func() { ended <- bench.service.Handle(ctx, job) }()
	testutil.Receive(t, bench.node.enqueued, "the artwork command")
	cancel(jobs.ErrCancelled)
	err := testutil.Receive(t, ended, "the artwork job to end")
	bench.node.mutex.Lock()
	defer bench.node.mutex.Unlock()
	if !errors.Is(err, jobs.ErrCancelled) {
		t.Errorf("the cancelled job ended with %v, want it cancelled for good", err)
	}
	if len(bench.node.sent) != 1 || !slices.Equal(bench.node.sentJobIDs, []int64{41}) ||
		!slices.Equal(bench.node.cancelled, []int64{41}) {
		t.Errorf("the node was sent commands for jobs %v and cancels for %v; want job 41's command, then its cancel",
			bench.node.sentJobIDs, bench.node.cancelled)
	}
}

// A node that started again lost the re-encode it held (D238 §3): the tool's failure, tried again
// at no cost the first time, and the job's failure on the second in a row; nothing is sent to stop
// what the node no longer has.
func TestService_ANodeThatStartedAgainIsTheToolsFailureAndTwoInARowFailTheJob(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{"/poster.jpg": []byte("x")})
	bench.node.enqueued = make(chan struct{}, 1)
	reEncodeAfter := func(previousAttemptToolFailed bool) error {
		job := bench.job(Ref{Kind: library.ArtPoster, SourceRef: "/poster.jpg"})
		job.ID, job.PreviousAttemptToolFailed = 41, previousAttemptToolFailed
		ended := make(chan error, 1)
		go func() { ended <- bench.service.Handle(t.Context(), job) }()
		testutil.Receive(t, bench.node.enqueued, "the artwork command")
		bench.service.exchanges.Interrupt(t.Context(), fakeNodeID)
		return testutil.Receive(t, ended, "the artwork job to end")
	}
	first := reEncodeAfter(false)
	if !errors.Is(first, nodes.ErrNodeRestarted) || !strings.HasPrefix(first.Error(), "interrupted: ") {
		t.Errorf("a first restart of the node ended %v; want it tried again as the tool's failure", first)
	}
	second := reEncodeAfter(true)
	if !errors.Is(second, nodes.ErrNodeRestarted) || !strings.HasPrefix(second.Error(), jobs.ToolFailedTwice) {
		t.Errorf("a second tool failure in a row ended %v; want the job's failure, saying so", second)
	}
	bench.node.mutex.Lock()
	defer bench.node.mutex.Unlock()
	if len(bench.node.cancelled) != 0 {
		t.Errorf("the node was told to stop jobs %v, want none: it lost them with its restart", bench.node.cancelled)
	}
}

func TestTake_QueuesOneJobPerItemThatFetchesWhatItNames(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{"/poster.jpg": []byte("p"), "/back.jpg": []byte("b")})
	queue := &jobstest.Queue{}
	bench.service.parts.Queue = queue
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		size := command.SizeNames[0]
		return nodeproto.ArtworkResult{Images: []nodeproto.ArtImage{{
			SizeName: size,
			WebP:     ourWebP,
		}}}
	}
	named := ItemImages{ItemID: bench.itemID, Source: "tmdb", Refs: []Ref{
		{Kind: library.ArtPoster, SourceRef: "/poster.jpg"}, {Kind: library.ArtBackdrop, SourceRef: "/back.jpg"},
	}}
	bench.service.Take(t.Context(), Found{Named: []ItemImages{named, named}})
	if len(*queue) != 1 || (*queue)[0].Kind != jobs.KindMakeArtworkSizes || (*queue)[0].ItemID != bench.itemID {
		t.Fatalf("queued %+v; want one make_artwork_sizes job for the item", *queue)
	}
	job := jobs.Job{
		ID: 9, Kind: jobs.KindMakeArtworkSizes, Target: jobs.Target{ItemID: bench.itemID},
		ArgsJSON: (*queue)[0].ArgsJSON,
	}
	if err := bench.service.Handle(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	rows := bench.rows(t)
	if len(rows) != 2 || rows[0].Kind != "backdrop" || rows[1].Kind != "poster" {
		t.Errorf("rows %+v; want the backdrop and the poster the job named", rows)
	}
}

func TestService_AnImageBesideTheMediaIsReadByItsNodeAndShownOverTheProviders(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{"/poster.jpg": []byte("tmdb poster")})
	var fileID int64
	nodeID, libraryID := librarytest.NodeWithLibrary(t, bench.database,
		librarytest.Library{Name: "L", Kind: "movies", MetadataSource: "nfo_then_tmdb", Root: "/m"})
	storetest.ExecRaw(t, bench.database.Path(), fmt.Sprintf(
		"INSERT INTO file (id, library_id, node_id, path, reported_path_raw, size_bytes, modified_at, filing, "+
			"filing_reason, first_seen_at, last_seen_at) "+
			"VALUES (40, %d, %d, '/m/Film/poster.jpg', CAST('/m/Film/poster.jpg' AS BLOB), 5, 7, "+
			"'attached', 'image', 1, 1)", libraryID, nodeID))
	fileID = 40
	localSum := sha256.Sum256([]byte("the file beside the film"))
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		result := nodeproto.ArtworkResult{Images: []nodeproto.ArtImage{
			{SizeName: nodeproto.PortraitS, WebP: ourWebP},
		}}
		if command.AbsolutePath != nil {
			result.SourceSha256 = hex.EncodeToString(localSum[:])
		}
		return result
	}
	local := localRef(LocalImage{FileID: fileID, SizeBytes: 5, ModifiedAtMs: 7})
	refs := []Ref{{Kind: library.ArtPoster, SourceRef: local}}
	args, err := json.Marshal(artworkJobArgs{ArtworkSource: SourceLocal, Refs: refs})
	if err != nil {
		t.Fatal(err)
	}
	if err = bench.service.Handle(t.Context(), jobs.Job{
		ID: 5, Target: jobs.Target{ItemID: bench.itemID},
		ArgsJSON: string(args),
	}); err != nil {
		t.Fatal(err)
	}
	sent := bench.node.sent[len(bench.node.sent)-1]
	if sent.AbsolutePath == nil || string(sent.AbsolutePath.Raw) != "/m/Film/poster.jpg" || sent.SourceImage != nil {
		t.Fatalf("the node was sent %+v; want the file's path for it to read, and no bytes", sent)
	}
	// TMDB's poster arrives later, and is still not what the film shows.
	bench.handle(t, Ref{Kind: library.ArtPoster, SourceRef: "/poster.jpg"})
	shown, err := bench.database.Read().ListShownArtwork(t.Context(), sqlcgen.ListShownArtworkParams{
		ItemIDListJSON: "[" + strconv.FormatInt(bench.itemID, 10) + "]", Kind: library.ArtPoster,
	})
	if err != nil || len(shown) == 0 || !bytes.Equal(shown[0].SourceSha256, localSum[:]) {
		t.Errorf("shown %+v (%v); want the image beside the media, stored under the hash the node read", shown, err)
	}
	// In a TMDB-only library, TMDB's poster replaces it once fetched (D215 §1).
	storetest.ExecRaw(t, bench.database.Path(),
		fmt.Sprintf("UPDATE library SET metadata_source = 'tmdb' WHERE id = %d", libraryID),
		fmt.Sprintf("UPDATE item SET library_id = %d WHERE id = %d", libraryID, bench.itemID))
	shown, err = bench.database.Read().ListShownArtwork(t.Context(), sqlcgen.ListShownArtworkParams{
		ItemIDListJSON: "[" + strconv.FormatInt(bench.itemID, 10) + "]", Kind: library.ArtPoster,
	})
	if err != nil || len(shown) == 0 ||
		!bytes.Equal(shown[0].SourceSha256, Received([]byte("tmdb poster")).sourceSha256()) {
		t.Errorf("in a TMDB-only library shown %+v (%v); want TMDB's poster", shown, err)
	}
}

// A re-encode's answer is recorded as it arrives, whether or not its job still waits (D238 §2): the
// sizes asked for, in WebP, under the SHA-256 the command names; no other.
func TestRecordArtwork_StoresTheSizesAskedForByTheSourceTheCommandNames(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{})
	sourceSha256 := strings.Repeat("ab", 32)
	webp := []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")
	command := nodeproto.Command{
		JobID:        9,
		AttemptToken: "no job waits for this",
		Work: nodeproto.Work{Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: &nodeproto.Artwork{
			SourceSha256: sourceSha256, Shape: nodeproto.ShapePortrait,
			SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
		}},
	}
	err := bench.service.exchanges.Deliver(t.Context(), fakeNodeID, nodeproto.Result{
		JobID: 9, AttemptToken: command.AttemptToken, Work: &command.Work,
		Artwork: &nodeproto.ArtworkResult{Images: []nodeproto.ArtImage{
			{SizeName: nodeproto.PortraitS, WebP: webp}, {SizeName: nodeproto.PortraitM, WebP: webp},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := bench.art[sourceSha256+"/"+string(nodeproto.PortraitS)+webpSuffix]; !found {
		t.Errorf("the art store holds %v, want the small portrait under the command's source", keysOf(bench.art))
	}
	if _, found := bench.art[sourceSha256+"/"+string(nodeproto.PortraitM)+webpSuffix]; found {
		t.Error("the art store holds a size nobody asked for")
	}
}

func keysOf(art memoryStore) []string {
	keys := make([]string, 0, len(art))
	for key := range art {
		keys = append(keys, key)
	}
	return keys
}
