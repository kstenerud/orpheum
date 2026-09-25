// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
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
	service   *Service
	ctx       context.Context //nolint:containedctx // the test's, for the answers it sends
	connected bool
	answer    func(nodeproto.Artwork) nodeproto.ArtworkResult

	mutex sync.Mutex
	sent  []nodeproto.Artwork
}

func (node *fakeNode) Connected() (int64, bool) { return 3, node.connected }

func (node *fakeNode) Enqueue(_ int64, command nodeproto.Command) {
	node.mutex.Lock()
	node.sent = append(node.sent, *command.Artwork)
	node.mutex.Unlock()
	if node.answer != nil {
		go node.service.Answered(node.ctx, 3, node.answer(*command.Artwork))
	}
}

// fetched is a source's images by reference.
type fetched map[string][]byte

func (images fetched) Fetch(_ context.Context, ref string) (Unsanitised, error) {
	image, found := images[ref]
	if !found {
		return Unsanitised{}, ErrNoImage
	}
	return Received(image), nil
}

// memoryStore is the art directory in memory.
type memoryStore map[string][]byte

func (art memoryStore) WriteArt(hash, name string, contents []byte) error {
	art[hash+"/"+name] = contents
	return nil
}

type bench struct {
	service  *Service
	database *store.Database
	node     *fakeNode
	art      memoryStore
	itemID   int64
}

func newBench(t *testing.T, images fetched) *bench {
	t.Helper()
	database := storetest.Open(t)
	var itemID int64
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		itemID, err = queries.InsertItem(t.Context(), sqlcgen.InsertItemParams{Kind: "title", CreatedAt: 1})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	node := &fakeNode{ctx: t.Context(), connected: true}
	art := memoryStore{}
	service := NewService(Parts{
		Database: database, Nodes: node, Store: art, Settings: settingstest.Seeded(t, database),
		Fetchers: map[string]Fetcher{"tmdb": images},
		Clock:    clocktest.NewFake(time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)),
		Logger:   slog.New(slog.DiscardHandler), Wait: 5 * time.Second,
	})
	node.service = service
	return &bench{service: service, database: database, node: node, art: art, itemID: itemID}
}

func (bench *bench) job(refs ...owedRef) jobs.Job {
	args, err := json.Marshal(owed{Source: "tmdb", Refs: refs})
	if err != nil {
		panic(err)
	}
	return jobs.Job{ID: 1, Kind: jobs.KindArtwork, Subject: bench.itemID, Args: string(args)}
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
	bench := newBench(t, fetched{"/poster.jpg": source})
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		return nodeproto.ArtworkResult{Job: command.Job, Width: 600, Height: 900, Images: []nodeproto.ArtImage{
			{Size: nodeproto.PortraitS, Bytes: ourWebP},
			{Size: nodeproto.PortraitM, Bytes: source},    // not WebP: never stored
			{Size: nodeproto.LandscapeS, Bytes: ourWebP},  // not asked for
			{Size: "../../state/library", Bytes: ourWebP}, // not a size at all
		}}
	}
	bench.handle(t, owedRef{Kind: identify.ArtPoster, Ref: "/poster.jpg"})
	sent := bench.node.sent
	if len(sent) != 1 || !bytes.Equal(sent[0].Source, source) || sent[0].Shape != nodeproto.ShapePortrait ||
		!slices.Equal(sent[0].Sizes, []nodeproto.ArtSizeName{nodeproto.PortraitS, nodeproto.PortraitM}) ||
		sent[0].MaxPixels != 40_000_000 || sent[0].Quality != 82 {
		t.Fatalf("sent %+v; want the source, portrait, the sizes made when fetched, and the settings", sent)
	}
	checkStored(t, bench.art, source)
	rows := bench.rows(t)
	if len(rows) != 1 || rows[0].Kind != "poster" || rows[0].Source != "tmdb" || *rows[0].SourceRef != "/poster.jpg" ||
		len(rows[0].ContentHash) != 32 {
		t.Errorf("rows %+v; want the poster with its hash", rows)
	}
}

// checkStored checks the store holds only portrait-s, our WebP, under the source's hash.
func checkStored(t *testing.T, art memoryStore, source []byte) {
	t.Helper()
	hash := Received(source).hash()
	if len(art) != 1 || !bytes.Equal(art[hash+"/portrait-s.webp"], ourWebP) {
		t.Errorf("stored %v; want only portrait-s, our WebP, under the source's hash", keys(art))
	}
	for _, stored := range art {
		if bytes.Contains(stored, source) {
			t.Error("the source's own bytes reached the store")
		}
	}
}

func keys(art memoryStore) []string {
	var names []string
	for name := range art {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func TestService_RecordsARefusalAndKeepsTheOlderImage(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fetched{"/old.jpg": []byte("old"), "/new.jpg": []byte("<html>")})
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		if string(command.Source) == "old" {
			return nodeproto.ArtworkResult{Job: command.Job, Images: []nodeproto.ArtImage{
				{Size: nodeproto.PortraitS, Bytes: ourWebP},
			}}
		}
		return nodeproto.ArtworkResult{Job: command.Job, Refused: "not an image"}
	}
	for _, ref := range []string{"/old.jpg", "/new.jpg", "/gone.jpg"} {
		bench.handle(t, owedRef{Kind: identify.ArtPoster, Ref: ref})
	}
	rows := bench.rows(t)
	if len(rows) != 3 {
		t.Fatalf("rows %+v; want the old poster, and the refused and missing ones held with no hash", rows)
	}
	shown, err := bench.database.Read().GetItemArtwork(t.Context(), sqlcgen.GetItemArtworkParams{
		ItemID: bench.itemID, Kind: "poster",
	})
	if err != nil || !bytes.Equal(shown, Received([]byte("old")).hash32()) {
		t.Errorf("shown %x (%v); want the old poster", shown, err)
	}
	sent := len(bench.node.sent)
	bench.handle(t, owedRef{Kind: identify.ArtPoster, Ref: "/new.jpg"})
	if len(bench.node.sent) != sent {
		t.Error("a refused image was sent to the node again")
	}
}

func TestService_WaitsForANode(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fetched{"/poster.jpg": []byte("x")})
	bench.node.connected = false
	err := bench.service.Handle(t.Context(), bench.job(owedRef{Kind: identify.ArtPoster, Ref: "/poster.jpg"}))
	if retry, isRetry := errors.AsType[*jobs.RetryAtError](err); !isRetry || retry.At.IsZero() {
		t.Errorf("with no node: %v, want a wait that costs no attempt", err)
	}
}

// queued is the job queue as a list, which says a job is open while one for its subject is.
type queued []jobs.New

func (queue *queued) Enqueue(_ context.Context, job jobs.New) (bool, error) {
	for _, open := range *queue {
		if open.Kind == job.Kind && open.Subject == job.Subject {
			return false, nil
		}
	}
	*queue = append(*queue, job)
	return true, nil
}

func TestService_OweQueuesOneJobPerItemThatFetchesWhatItNames(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fetched{"/poster.jpg": []byte("p"), "/back.jpg": []byte("b")})
	queue := &queued{}
	bench.service.parts.Queue = queue
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		size := command.Sizes[0]
		return nodeproto.ArtworkResult{Job: command.Job, Images: []nodeproto.ArtImage{{Size: size, Bytes: ourWebP}}}
	}
	owed := identify.OwedArtwork{ItemID: bench.itemID, Source: "tmdb", Refs: []identify.ArtRef{
		{Kind: identify.ArtPoster, Ref: "/poster.jpg"}, {Kind: identify.ArtBackdrop, Ref: "/back.jpg"},
	}}
	bench.service.Owe(t.Context(), []identify.OwedArtwork{owed, owed})
	if len(*queue) != 1 || (*queue)[0].Kind != jobs.KindArtwork || (*queue)[0].Subject != bench.itemID {
		t.Fatalf("queued %+v; want one artwork job for the item", *queue)
	}
	job := jobs.Job{ID: 9, Kind: jobs.KindArtwork, Subject: bench.itemID, Args: (*queue)[0].Args}
	if err := bench.service.Handle(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	rows := bench.rows(t)
	if len(rows) != 2 || rows[0].Kind != "backdrop" || rows[1].Kind != "poster" {
		t.Errorf("rows %+v; want the backdrop and the poster the job named", rows)
	}
}
