// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/jobs/jobstest"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// frameWorld is a library of the bench's database, on its node, whose videos a test makes.
type frameWorld struct {
	bench     *bench
	queue     *jobstest.Queue
	nodeID    int64
	libraryID int64
	nextFile  int64
}

// newFrameWorld makes a library of kind and metadataSource, its root /m on the bench's node.
func newFrameWorld(t *testing.T, kind, metadataSource string) *frameWorld {
	t.Helper()
	bench := newBench(t, fakeFetcher{})
	queue := &jobstest.Queue{}
	bench.service.parts.Queue = queue
	nodeID, libraryID := librarytest.NodeWithLibrary(t, bench.database,
		librarytest.Library{Name: "L", Kind: kind, MetadataSource: metadataSource, Root: "/m"})
	return &frameWorld{bench: bench, queue: queue, nodeID: nodeID, libraryID: libraryID, nextFile: 100}
}

// video makes a title of the spec in the world's library with one read video, whose probe has a
// cover attached before its picture, and returns the title's id and its file's.
func (world *frameWorld) video(t *testing.T, spec librarytest.TitleSpec) (titleID, fileID int64) {
	t.Helper()
	database := world.bench.database
	spec.Library = world.libraryID
	titleID = librarytest.Title(t, database, spec)
	versionID := librarytest.Version(t, database, librarytest.VersionSpec{TitleID: titleID})
	world.nextFile++
	fileID = librarytest.File(t, database, librarytest.FileSpec{
		ID: world.nextFile, LibraryID: world.libraryID, NodeID: world.nodeID,
		Path: fmt.Sprintf("/m/video-%d.mkv", world.nextFile), SizeBytes: 5000, ModifiedAt: 7, PayloadHash: []byte{1, 2},
		VersionID: versionID, FirstSeenAt: 1, LastSeenAt: 10,
	})
	probe, err := json.Marshal(nodeproto.Probe{
		Format: nodeproto.ProbeFormat{DurationMs: 600_000},
		Streams: []nodeproto.ProbeStream{
			{StreamIndex: 0, Kind: "audio"},
			{StreamIndex: 1, Kind: "video", Disposition: []string{"attached_pic"}},
			{StreamIndex: 2, Kind: "video"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	librarytest.Probe(t, database, librarytest.ProbeSpec{FileID: fileID, ProbeJSON: string(probe), DurationMs: 600_000})
	return titleID, fileID
}

// fetched stores a provider's answer for an item, as its first fetch does.
func (world *frameWorld) fetched(t *testing.T, itemID int64) {
	t.Helper()
	storetest.ExecRaw(t, world.bench.database.Path(), fmt.Sprintf(
		"INSERT INTO provider_response (item_id, provider, part, fetched_at, last_asked_at, body_gzip) "+
			"VALUES (%d, 'tmdb', '', 1, 1, x'00')", itemID))
}

// series makes a series in the world's library, following source, with episode as its member.
func (world *frameWorld) series(t *testing.T, followedSource string, episodeIDs ...int64) int64 {
	t.Helper()
	seriesID := librarytest.Item(t, world.bench.database, librarytest.ItemSpec{
		Kind: "collection", LibraryID: world.libraryID,
	})
	statements := []string{fmt.Sprintf("INSERT INTO collection (id, kind, created_by, followed_source, "+
		"identification_state) VALUES (%d, 'series', 'scanner', '%s', 'identified')", seriesID,
		followedSource)}
	for _, episodeID := range episodeIDs {
		statements = append(statements, fmt.Sprintf(
			"INSERT INTO collection_member (collection_id, member_title) VALUES (%d, %d)", seriesID, episodeID))
	}
	storetest.ExecRaw(t, world.bench.database.Path(), statements...)
	return seriesID
}

// store records an image as stored for an item.
func (world *frameWorld) store(t *testing.T, itemID int64, row sqlcgen.InsertArtworkParams) {
	t.Helper()
	row.ItemID = itemID
	err := world.bench.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		_, err := queries.InsertArtwork(t.Context(), row)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// pass is a pass over the world's library, finding found, and returns the items a frame still was
// queued for, checking each job.
func (world *frameWorld) pass(t *testing.T, found Found) []int64 {
	t.Helper()
	*world.queue = nil
	if found.BesideTheMedia == nil {
		found.BesideTheMedia = &LocalImages{}
	}
	found.BesideTheMedia.LibraryID = world.libraryID
	world.bench.service.Take(t.Context(), found)
	var planned []int64
	for _, request := range *world.queue {
		var args artworkJobArgs
		if err := json.Unmarshal([]byte(request.ArgsJSON), &args); err != nil {
			t.Fatal(err)
		}
		if args.ArtworkSource != SourceFrame {
			continue
		}
		if request.Priority != jobs.PriorityBackground || len(args.Refs) != 1 || args.Refs[0].Kind != library.ArtStill {
			t.Errorf("a frame still's job %+v; want one still, at the background's priority (D362 §3)", request)
		}
		planned = append(planned, request.ItemID)
	}
	slices.Sort(planned)
	return planned
}

// A film is given a frame still only once every other source of a picture was tried and gave none:
// matched and fetched with no poster or backdrop named, or settled with no match; never while it
// awaits its match or its fetch, is ambiguous, its fetch failed, or a picture a source names is still
// to come; and never where it shows a poster or a backdrop (the owner, 2026-10-27; D362 §2).
func TestTake_AFilmGetsAFrameStillOnlyOnceEverySourceWasTriedAndGaveNoPicture(t *testing.T) {
	t.Parallel()
	world := newFrameWorld(t, "movie", "tmdb")
	matched := librarytest.TitleSpec{FollowedSource: "tmdb", FollowedSourceID: "1"}
	awaitingMatch, _ := world.video(t, librarytest.TitleSpec{IdentificationState: "unidentified"})
	awaitingFetch, _ := world.video(t, matched)
	fetchedBare, fileID := world.video(t, matched)
	world.fetched(t, fetchedBare)
	noMatch, _ := world.video(t, librarytest.TitleSpec{FollowedSource: "none", IdentificationState: "unidentified"})
	ambiguous, _ := world.video(t, librarytest.TitleSpec{IdentificationState: "ambiguous"})
	failed, _ := world.video(t, librarytest.TitleSpec{FollowedSource: "none", IdentificationState: "unidentified"})
	storetest.ExecRaw(t, world.bench.database.Path(), fmt.Sprintf(
		"INSERT INTO fetch_failure (item_id, failure, failed_at) VALUES (%d, 'a 500', 1)", failed))
	posterToCome, _ := world.video(t, matched)
	world.fetched(t, posterToCome)
	missingToRecheck, _ := world.video(t, matched)
	world.fetched(t, missingToRecheck)
	world.store(t, missingToRecheck, sqlcgen.InsertArtworkParams{
		Kind: library.ArtPoster, Source: "tmdb", SourceRef: new("/missing.jpg"), RefusedAt: new(int64(5)),
		Refusal: new("404"), IsMissingAtSource: 1,
	})
	givenUp, _ := world.video(t, matched)
	world.fetched(t, givenUp)
	world.store(t, givenUp, sqlcgen.InsertArtworkParams{
		Kind: library.ArtPoster, Source: "tmdb", SourceRef: new("/broken.jpg"), RefusedAt: new(int64(5)),
		Refusal: new("the node refused it: not an image"),
	})
	withBackdrop, _ := world.video(t, matched)
	world.fetched(t, withBackdrop)
	world.store(t, withBackdrop, sqlcgen.InsertArtworkParams{
		Kind: library.ArtBackdrop, Source: "tmdb", SourceRef: new("/back.jpg"), SourceSha256: []byte{9},
	})
	named := []ItemImages{
		{ItemID: posterToCome, Source: "tmdb", Refs: []Ref{{Kind: library.ArtPoster, SourceRef: "/coming.jpg"}}},
		{ItemID: missingToRecheck, Source: "tmdb", Refs: []Ref{{Kind: library.ArtPoster, SourceRef: "/missing.jpg"}}},
		{ItemID: givenUp, Source: "tmdb", Refs: []Ref{{Kind: library.ArtPoster, SourceRef: "/broken.jpg"}}},
		{ItemID: withBackdrop, Source: "tmdb", Refs: []Ref{{Kind: library.ArtBackdrop, SourceRef: "/back.jpg"}}},
	}
	planned := world.pass(t, Found{Named: named, AwaitingMatch: []int64{awaitingMatch}})
	if want := []int64{fetchedBare, noMatch, givenUp}; !slices.Equal(planned, want) {
		t.Errorf("frame stills planned for %v; want %v: fetched with no picture named, settled with no match, and "+
			"one whose poster was refused for good. Awaiting its match %d, its fetch %d, ambiguous %d, failed %d, "+
			"a poster to come %d, a miss to look at again %d, a backdrop %d", planned, want, awaitingMatch,
			awaitingFetch, ambiguous, failed, posterToCome, missingToRecheck, withBackdrop)
	}
	if got := queuedRefs(t, world.queue)[SourceFrame]; !slices.Contains(got,
		Ref{Kind: library.ArtStill, SourceRef: fmt.Sprintf("file:%d:5000:7", fileID)}) {
		t.Errorf("frame stills' refs %+v; want the video named by its file, size and time", got)
	}
}

// An episode is given a frame still once its series was fetched and its season's listing is not
// awaited, and the provider's still for it was looked for; an other video, never matched, once the
// pass read what is beside it; neither where it shows a picture of its kind (D362 §2).
func TestTake_AnEpisodeAndAnOtherVideoGetAFrameStillWhenNothingElseShowsThem(t *testing.T) {
	t.Parallel()
	shows := newFrameWorld(t, "series", "tmdb")
	episode := librarytest.TitleSpec{Kind: "episode", FollowedSource: "tmdb", FollowedSourceID: "1"}
	bare, _ := shows.video(t, episode)
	withStill, _ := shows.video(t, episode)
	shows.store(t, withStill, sqlcgen.InsertArtworkParams{
		Kind: library.ArtStill, Source: "tmdb", SourceRef: new("/still.jpg"), SourceSha256: []byte{9},
	})
	fetchedSeries := shows.series(t, "tmdb", bare, withStill)
	shows.fetched(t, fetchedSeries)
	ofUnfetched, _ := shows.video(t, episode)
	shows.series(t, "tmdb", ofUnfetched)
	listingAwaited, _ := shows.video(t, episode)
	awaitedSeries := shows.series(t, "tmdb", listingAwaited)
	shows.fetched(t, awaitedSeries)
	if planned := shows.pass(t, Found{AwaitingListings: []int64{awaitedSeries}}); !slices.Equal(planned,
		[]int64{bare}) {
		t.Errorf("episodes' frame stills planned for %v; want only %d: its series fetched, no still of its own",
			planned, bare)
	}
	other := newFrameWorld(t, "other", "nfo")
	video := librarytest.TitleSpec{Kind: "other", FollowedSource: "none", IdentificationState: "unidentified"}
	plain, _ := other.video(t, video)
	withBackdrop, _ := other.video(t, video)
	other.store(t, withBackdrop, sqlcgen.InsertArtworkParams{
		Kind: library.ArtBackdrop, Source: SourceLocal, SourceRef: new("file:1:1:1"), SourceSha256: []byte{9},
	})
	beside := &LocalImages{Images: []LocalImage{
		{ItemID: withBackdrop, Kind: library.ArtBackdrop, FileID: 1, SizeBytes: 1, ModifiedAtMs: 1},
	}}
	if planned := other.pass(t, Found{BesideTheMedia: beside}); !slices.Equal(planned, []int64{plain}) {
		t.Errorf("other videos' frame stills planned for %v; want only %d, which nothing beside it shows", planned,
			plain)
	}
}

// A still taken is held: no pass asks for it again. One the node found no frame for is held for good,
// until the video changes; one that failed is asked again once a scan has seen the video since, as an
// image beside the media is (G9; D362 §5).
func TestTake_AFrameStillIsNotAskedAgainButAFailureIsOnceAScanSeesTheVideo(t *testing.T) {
	t.Parallel()
	world := newFrameWorld(t, "other", "nfo")
	video := librarytest.TitleSpec{Kind: "other", FollowedSource: "none", IdentificationState: "unidentified"}
	taken, takenFile := world.video(t, video)
	black, blackFile := world.video(t, video)
	failed, failedFile := world.video(t, video)
	ref := func(fileID int64) *string { return new(fmt.Sprintf("file:%d:5000:7", fileID)) }
	world.store(t, taken, sqlcgen.InsertArtworkParams{
		Kind: library.ArtStill, Source: SourceFrame, SourceRef: ref(takenFile), SourceSha256: []byte{9},
	})
	world.store(t, black, sqlcgen.InsertArtworkParams{
		Kind: library.ArtStill, Source: SourceFrame, SourceRef: ref(blackFile), RefusedAt: new(int64(20)),
		Refusal: new("no frame"), IsMissingAtSource: 1,
	})
	world.store(t, failed, sqlcgen.InsertArtworkParams{
		Kind: library.ArtStill, Source: SourceFrame, SourceRef: ref(failedFile), RefusedAt: new(int64(20)),
		Refusal: new("a stall"),
	})
	if planned := world.pass(t, Found{}); len(planned) != 0 {
		t.Fatalf("planned %v; want nothing: a still held, a video with no frame, a failure no scan has seen past",
			planned)
	}
	storetest.ExecRaw(t, world.bench.database.Path(), "UPDATE file SET last_seen_at = 30")
	if planned := world.pass(t, Found{}); !slices.Equal(planned, []int64{failed}) {
		t.Errorf("planned %v once a scan saw the videos again; want only the failed %d asked again", planned, failed)
	}
	storetest.ExecRaw(t, world.bench.database.Path(), fmt.Sprintf("UPDATE file SET size_bytes = 6000 WHERE id = %d",
		blackFile))
	if planned := world.pass(t, Found{}); !slices.Contains(planned, black) {
		t.Errorf("planned %v once the black video changed; want %d asked again", planned, black)
	}
}

// otherVideo is an other video's title, never matched.
var otherVideo = librarytest.TitleSpec{Kind: "other", FollowedSource: "none", IdentificationState: "unidentified"}

// newHandleWorld is an other library whose node answers a still of video-<stillFile>.mkv with a
// size of it, and finds no frame in any other video.
func newHandleWorld(t *testing.T) (world *frameWorld, stillFile *int64) {
	t.Helper()
	world = newFrameWorld(t, "other", "nfo")
	stillFile = new(int64)
	stillSum := sha256.Sum256([]byte("a still the node took"))
	world.bench.node.answer = func(image nodeproto.Artwork) nodeproto.ArtworkResult {
		if string(image.Video.Path) == fmt.Sprintf("video-%d.mkv", *stillFile) {
			return nodeproto.ArtworkResult{
				Images:       []nodeproto.ArtImage{{SizeName: nodeproto.LandscapeS, WebP: ourWebP}},
				SourceSha256: hex.EncodeToString(stillSum[:]),
			}
		}
		return nodeproto.ArtworkResult{Refusal: "no frame in the stretch is not black", HasNoFrame: true}
	}
	return world, stillFile
}

// runFrameJob runs the job that takes a still of fileID for itemID, as the bench's item.
func runFrameJob(t *testing.T, world *frameWorld, itemID, fileID int64) []sqlcgen.ListLiveArtworkRow {
	t.Helper()
	world.bench.itemID = itemID
	args, err := json.Marshal(artworkJobArgs{ArtworkSource: SourceFrame, Refs: []Ref{
		{Kind: library.ArtStill, SourceRef: fmt.Sprintf("file:%d:5000:7", fileID)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	job := jobs.Job{
		ID: itemID, Kind: jobs.KindMakeArtworkSizes, Target: jobs.Target{ItemID: itemID}, ArgsJSON: string(args),
	}
	if err := world.bench.run(t, job); err != nil {
		t.Fatal(err)
	}
	return world.bench.rows(t)
}

// A frame still's job gives the node the video it names, by its root and payload hash, with its
// picture stream (not the cover attached to it) and a tenth of its length; the still the node
// answers is kept as the item's still (D362 §1, §2).
func TestHandle_AFrameStillIsTakenFromTheVideoAndKeptAsTheItemsStill(t *testing.T) {
	t.Parallel()
	world, stillFile := newHandleWorld(t)
	titleID, fileID := world.video(t, otherVideo)
	*stillFile = fileID
	rows := runFrameJob(t, world, titleID, fileID)
	sent := world.bench.node.sent[0]
	if sent.Video == nil || sent.Video.RootPath != "/m" || sent.FrameStreamIndex != 2 || sent.FrameAtMs != 60_000 ||
		sent.Shape != nodeproto.ShapeLandscape || sent.Video.PayloadHash != "0102" {
		t.Errorf("sent %+v; want the video under /m, its picture stream 2, at a tenth of its 600 s", sent)
	}
	if len(rows) != 1 || rows[0].Kind != library.ArtStill || rows[0].Source != SourceFrame ||
		rows[0].SourceSha256 == nil {
		t.Errorf("held %+v; want the still kept as the item's still, from a frame", rows)
	}
}

// A video the node found no frame in, and one with no picture at all, which no node is asked about,
// are held as having no frame (D362 §5); nothing is taken for an item that has come to show a picture
// of its own before its job ran (D362 §2).
func TestHandle_AVideoWithNoFrameIsHeldSoAndAPictureShownTakesNone(t *testing.T) {
	t.Parallel()
	world, _ := newHandleWorld(t)
	blackID, blackFile := world.video(t, otherVideo)
	if rows := runFrameJob(t, world, blackID, blackFile); len(rows) != 1 || rows[0].RefusedAt == nil ||
		rows[0].IsMissingAtSource != 1 {
		t.Errorf("held %+v; want a video the node found no frame in held so", rows)
	}
	sentBefore := len(world.bench.node.sent)
	soundID, soundFile := world.video(t, otherVideo)
	librarytest.Probe(t, world.bench.database, librarytest.ProbeSpec{
		FileID: soundFile, ProbeJSON: `{"streams":[{"streamIndex":0,"kind":"audio"}]}`,
	})
	if rows := runFrameJob(t, world, soundID, soundFile); len(rows) != 1 || rows[0].IsMissingAtSource != 1 {
		t.Errorf("a video with no picture: held %+v; want no frame held", rows)
	}
	shownID, shownFile := world.video(t, otherVideo)
	world.store(t, shownID, sqlcgen.InsertArtworkParams{
		Kind: library.ArtBackdrop, Source: SourceLocal, SourceRef: new("file:1:1:1"), SourceSha256: []byte{9},
	})
	if rows := runFrameJob(t, world, shownID, shownFile); len(rows) != 1 || rows[0].Source != SourceLocal {
		t.Errorf("an item showing a backdrop: held %+v; want no still taken", rows)
	}
	if sent := len(world.bench.node.sent) - sentBefore; sent != 0 {
		t.Errorf("%d stills asked of the node for a video with no picture and an item showing one; want none", sent)
	}
}
