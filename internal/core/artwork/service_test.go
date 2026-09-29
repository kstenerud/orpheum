// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"bytes"
	"context"
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
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/jobs/jobstest"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A WebP as our encoder writes one, as far as the service looks: a RIFF container of WebP.
var ourWebP = append([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), bytes.Repeat([]byte{7}, 16)...)

// fakeNode is the node channel with one node that answers each artwork command as answer says.
type fakeNode struct {
	service     *Service
	ctx         context.Context //nolint:containedctx // the test's, for the answers it sends
	isConnected bool
	answer      func(nodeproto.Artwork) nodeproto.ArtworkResult

	mutex     sync.Mutex
	sent      []nodeproto.Artwork
	cancelled []int64       // the jobs a cancel was sent for
	enqueued  chan struct{} // told of each artwork command, when set
}

func (node *fakeNode) LastHeardNodeID() (int64, bool) { return 3, node.isConnected }

func (node *fakeNode) IsConnected(nodeID int64) bool { return nodeID == 3 && node.isConnected }

func (node *fakeNode) Enqueue(_ int64, command nodeproto.Command) {
	node.mutex.Lock()
	if command.Type == nodeproto.CommandCancel {
		node.cancelled = append(node.cancelled, command.Cancel.JobID)
		node.mutex.Unlock()
		return
	}
	node.sent = append(node.sent, *command.EncodeArtwork)
	node.mutex.Unlock()
	if node.enqueued != nil {
		node.enqueued <- struct{}{}
	}
	if node.answer != nil {
		go node.service.TakeArtworkAnswer(node.ctx, 3, node.answer(*command.EncodeArtwork))
	}
}

// fakeFetcher is a source's images by reference.
type fakeFetcher map[string][]byte

func (fakeFetcher) RecheckAfter() time.Duration { return 12 * time.Hour }

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
		Database: database, Queue: &jobstest.Queue{}, Nodes: node, ArtFiles: art, Cache: storetest.OpenCache(t),
		Settings: config,
		Fetchers: map[string]Fetcher{"tmdb": images},
		Clock:    fakeClock,
		Logger:   slog.New(slog.DiscardHandler), AnswerWait: 5 * time.Second,
	})
	node.service = service
	return &bench{
		service: service, database: database, node: node, art: art, itemID: itemID, settings: config, clock: fakeClock,
	}
}

func (bench *bench) job(refs ...owedRef) jobs.Job {
	args, err := json.Marshal(artworkJobArgs{ArtworkSource: "tmdb", Refs: refs})
	if err != nil {
		panic(err)
	}
	return jobs.Job{ID: 1, Kind: jobs.KindArtwork, Target: jobs.Target{ItemID: bench.itemID}, ArgsJSON: string(args)}
}

func (bench *bench) handle(t *testing.T, refs ...owedRef) {
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
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		return nodeproto.ArtworkResult{
			JobID: command.JobID, SourceWidth: 600, SourceHeight: 900,
			Images: []nodeproto.ArtImage{
				{SizeName: nodeproto.PortraitS, WebP: ourWebP},
				{SizeName: nodeproto.PortraitM, WebP: source},    // not WebP: never stored
				{SizeName: nodeproto.LandscapeS, WebP: ourWebP},  // not asked for
				{SizeName: "../../state/library", WebP: ourWebP}, // not a size at all
			},
		}
	}
	bench.handle(t, owedRef{Kind: identify.ArtPoster, SourceRef: "/poster.jpg"})
	sent := bench.node.sent
	if len(sent) != 1 || !bytes.Equal(sent[0].SourceImage, source) || sent[0].Shape != nodeproto.ShapePortrait ||
		!slices.Equal(sent[0].SizeNames, []nodeproto.ArtSizeName{nodeproto.PortraitS, nodeproto.PortraitM}) ||
		sent[0].MaxSourcePixels != 40_000_000 || sent[0].WebPQuality != 82 {
		t.Fatalf("sent %+v; want the source, portrait, the sizes made when fetched, and the settings", sent)
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
			return nodeproto.ArtworkResult{JobID: command.JobID, Images: []nodeproto.ArtImage{
				{SizeName: nodeproto.PortraitS, WebP: ourWebP},
			}}
		}
		return nodeproto.ArtworkResult{JobID: command.JobID, Refusal: "not an image"}
	}
	for _, ref := range []string{"/old.jpg", "/new.jpg", "/gone.jpg"} {
		bench.handle(t, owedRef{Kind: identify.ArtPoster, SourceRef: ref})
	}
	rows := bench.rows(t)
	if len(rows) != 3 {
		t.Fatalf("rows %+v; want the old poster, and the refused and missing ones held with no hash", rows)
	}
	shown, err := bench.database.Read().ListShownArtwork(t.Context(), sqlcgen.ListShownArtworkParams{
		ItemIds: "[" + strconv.FormatInt(bench.itemID, 10) + "]", Kind: "poster",
	})
	if err != nil || len(shown) == 0 || !bytes.Equal(shown[0].SourceSha256, Received([]byte("old")).sourceSha256()) {
		t.Errorf("shown %+v (%v); want the old poster", shown, err)
	}
	sent := len(bench.node.sent)
	bench.handle(t, owedRef{Kind: identify.ArtPoster, SourceRef: "/new.jpg"})
	if len(bench.node.sent) != sent {
		t.Error("a refused image was sent to the node again")
	}
}

func TestService_WaitsForANode(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{"/poster.jpg": []byte("x")})
	bench.node.isConnected = false
	err := bench.service.Handle(t.Context(), bench.job(owedRef{Kind: identify.ArtPoster, SourceRef: "/poster.jpg"}))
	if retry, isRetry := errors.AsType[*jobs.RetryAtError](err); !isRetry || retry.At.IsZero() {
		t.Errorf("with no node: %v, want a wait that costs no attempt", err)
	}
}

// A job waits for the node's answer by its own id, and cancelled while it waits, it tells the node
// to stop that job.
func TestService_ACancelledJobTellsTheNodeToStopItByItsID(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{"/poster.jpg": []byte("x")})
	bench.node.enqueued = make(chan struct{}, 1)
	job := bench.job(owedRef{Kind: identify.ArtPoster, SourceRef: "/poster.jpg"})
	job.ID = 41
	ctx, cancel := context.WithCancelCause(t.Context())
	ended := make(chan error, 1)
	go func() { ended <- bench.service.Handle(ctx, job) }()
	<-bench.node.enqueued
	cancel(jobs.ErrCancelled)
	err := <-ended
	bench.node.mutex.Lock()
	defer bench.node.mutex.Unlock()
	if !errors.Is(err, jobs.ErrCancelled) {
		t.Errorf("the cancelled job ended with %v, want it cancelled for good", err)
	}
	if len(bench.node.sent) != 1 || bench.node.sent[0].JobID != 41 || !slices.Equal(bench.node.cancelled, []int64{41}) {
		t.Errorf("the node was sent %+v and cancels for %v; want job 41's command, then its cancel",
			bench.node.sent, bench.node.cancelled)
	}
}

func TestService_OweQueuesOneJobPerItemThatFetchesWhatItNames(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{"/poster.jpg": []byte("p"), "/back.jpg": []byte("b")})
	queue := &jobstest.Queue{}
	bench.service.parts.Queue = queue
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		size := command.SizeNames[0]
		return nodeproto.ArtworkResult{JobID: command.JobID, Images: []nodeproto.ArtImage{{
			SizeName: size,
			WebP:     ourWebP,
		}}}
	}
	owed := identify.OwedArtwork{ItemID: bench.itemID, Source: "tmdb", Refs: []identify.ArtRef{
		{Kind: identify.ArtPoster, Ref: "/poster.jpg"}, {Kind: identify.ArtBackdrop, Ref: "/back.jpg"},
	}}
	bench.service.QueueOwed(t.Context(), []identify.OwedArtwork{owed, owed})
	if len(*queue) != 1 || (*queue)[0].Kind != jobs.KindArtwork || (*queue)[0].ItemID != bench.itemID {
		t.Fatalf("queued %+v; want one artwork job for the item", *queue)
	}
	job := jobs.Job{
		ID: 9, Kind: jobs.KindArtwork, Target: jobs.Target{ItemID: bench.itemID}, ArgsJSON: (*queue)[0].ArgsJSON,
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
	storetest.ExecRaw(t, bench.database.Path(),
		"INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at) "+
			"VALUES (9, 'L', 'movies', 'nfo_then_tmdb', 'manual', 0, 1)",
		"INSERT INTO node (id, name, enrolled_at) VALUES (3, 'local', 1)",
		"INSERT INTO file (id, library_id, node_id, path, reported_path_bytes, size_bytes, modified_at, filing, "+
			"filing_reason, "+
			"first_seen_at, "+
			"last_seen_at) VALUES (40, 9, 3, '/m/Film/poster.jpg', CAST('/m/Film/poster.jpg' AS BLOB), 5, 7, "+
			"'attached', 'image', 1, 1)")
	fileID = 40
	localSum := sha256.Sum256([]byte("the file beside the film"))
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		result := nodeproto.ArtworkResult{JobID: command.JobID, Images: []nodeproto.ArtImage{
			{SizeName: nodeproto.PortraitS, WebP: ourWebP},
		}}
		if command.AbsolutePath != nil {
			result.SourceSha256 = hex.EncodeToString(localSum[:])
		}
		return result
	}
	local := identify.LocalRef(fileID, 5, 7)
	refs := []owedRef{{Kind: identify.ArtPoster, SourceRef: local}}
	args, err := json.Marshal(artworkJobArgs{ArtworkSource: identify.SourceLocal, Refs: refs})
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
	if sent.AbsolutePath == nil || string(sent.AbsolutePath.Bytes) != "/m/Film/poster.jpg" || sent.SourceImage != nil {
		t.Fatalf("the node was sent %+v; want the file's path for it to read, and no bytes", sent)
	}
	// TMDB's poster arrives later, and is still not what the film shows.
	bench.handle(t, owedRef{Kind: identify.ArtPoster, SourceRef: "/poster.jpg"})
	shown, err := bench.database.Read().ListShownArtwork(t.Context(), sqlcgen.ListShownArtworkParams{
		ItemIds: "[" + strconv.FormatInt(bench.itemID, 10) + "]", Kind: identify.ArtPoster,
	})
	if err != nil || len(shown) == 0 || !bytes.Equal(shown[0].SourceSha256, localSum[:]) {
		t.Errorf("shown %+v (%v); want the image beside the media, stored under the hash the node read", shown, err)
	}
	// In a TMDB-only library, TMDB's poster replaces it once fetched (D215 §1).
	storetest.ExecRaw(t, bench.database.Path(),
		"UPDATE library SET metadata_source = 'tmdb' WHERE id = 9",
		fmt.Sprintf("UPDATE item SET library_id = 9 WHERE id = %d", bench.itemID))
	shown, err = bench.database.Read().ListShownArtwork(t.Context(), sqlcgen.ListShownArtworkParams{
		ItemIds: "[" + strconv.FormatInt(bench.itemID, 10) + "]", Kind: identify.ArtPoster,
	})
	if err != nil || len(shown) == 0 ||
		!bytes.Equal(shown[0].SourceSha256, Received([]byte("tmdb poster")).sourceSha256()) {
		t.Errorf("in a TMDB-only library shown %+v (%v); want TMDB's poster", shown, err)
	}
}
