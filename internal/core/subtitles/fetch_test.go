// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitles

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/providers/opensubtitles"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/bounded"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakeSubtitler is OpenSubtitles as the subtitles service asks it: it answers each search with its
// candidates and each download with its body, and counts what it was asked.
type fakeSubtitler struct {
	candidates []opensubtitles.Candidate
	body       []byte

	mutex     sync.Mutex
	queries   []opensubtitles.Query // guarded by mutex
	downloads int                   // guarded by mutex
}

func (subtitler *fakeSubtitler) Search(
	_ context.Context, query opensubtitles.Query,
) ([]opensubtitles.Candidate, error) {
	subtitler.mutex.Lock()
	defer subtitler.mutex.Unlock()
	subtitler.queries = append(subtitler.queries, query)
	return append([]opensubtitles.Candidate(nil), subtitler.candidates...), nil
}

func (subtitler *fakeSubtitler) Download(context.Context, int64) (opensubtitles.Downloaded, error) {
	subtitler.mutex.Lock()
	defer subtitler.mutex.Unlock()
	subtitler.downloads++
	return opensubtitles.Downloaded{Body: subtitler.body, FileName: "film.srt", RemainingDownloads: 19}, nil
}

func (*fakeSubtitler) Allowance(context.Context) (opensubtitles.Allowance, error) {
	return opensubtitles.Allowance{RemainingDownloads: 20, AllowedDownloads: 20}, nil
}

func (subtitler *fakeSubtitler) downloadCount() int {
	subtitler.mutex.Lock()
	defer subtitler.mutex.Unlock()
	return subtitler.downloads
}

// fetchBench is a bench whose one video is probed, hashed for OpenSubtitles and fetched for from a
// fake subtitler; a conversion a fetch queues is run at once by its handler, as a runner would.
type fetchBench struct {
	*bench
	subtitler *fakeSubtitler
	// converted carries each queued conversion job's answer.
	converted chan error
	// userID is the user the bench's fetches are made for.
	userID int64
}

const fetchedProviderFileID = 7

func newFetchBench(t *testing.T) *fetchBench {
	t.Helper()
	bench := newBench(t)
	librarytest.Probe(t, bench.database, librarytest.ProbeSpec{FileID: bareVideoFileID})
	storetest.ExecRaw(t, bench.database.Path(),
		"UPDATE file SET opensubtitles_hash = '8e245d9679d31e12' WHERE id = 1")
	subtitler := &fakeSubtitler{body: []byte("1\n00:00:00,000 --> 00:00:01,000\nHello.\n")}
	bench.service.subtitler = subtitler
	fetch := &fetchBench{
		bench: bench, subtitler: subtitler, converted: make(chan error, 4),
		userID: storetest.InsertUser(t, bench.database, "viewer", "user"),
	}
	nextJobID := int64(100)
	bench.queue.onEnqueue = func(request jobs.Request) {
		if request.Kind != jobs.KindConvertFetchedSubtitle {
			return
		}
		nextJobID++
		job := jobs.Job{ID: nextJobID, Kind: request.Kind, Target: request.Target, Priority: request.Priority}
		answer := bench.resolver.Start(t, bench.service.HandleConvertFetchedSubtitle, job)
		go func() { fetch.converted <- <-answer }()
	}
	return fetch
}

func (bench *fetchBench) fetch(t *testing.T) (Fetched, error) {
	t.Helper()
	return bench.service.Fetch(t.Context(), bench.userID, bareVideoFileID,
		FetchRequest{ProviderFileID: fetchedProviderFileID, Language: "en"})
}

// mustHaveConvertedAtPlayPriority checks the fetch's conversion job was queued at play priority,
// answered, and kept the node's text for the fetched subtitle.
func (bench *fetchBench) mustHaveConvertedAtPlayPriority(t *testing.T, fetched Fetched) {
	t.Helper()
	if err := testutil.Receive(t, bench.converted, "the conversion job's answer"); err != nil {
		t.Errorf("the conversion job: %v", err)
	}
	for _, request := range bench.queue.requests {
		if request.Kind == jobs.KindConvertFetchedSubtitle && request.Priority != jobs.PriorityPlayNow {
			t.Errorf("the conversion was queued at %v; want play priority", request.Priority)
		}
	}
	row, err := bench.database.Read().GetFetchedSubtitle(t.Context(), fetched.Track.FetchedSubtitleID)
	if err != nil {
		t.Fatalf("the fetched subtitle is not kept: %v", err)
	}
	text, err := bench.database.Read().GetSubtitleText(t.Context(), fetchedKey(row.BodyGzip))
	if err != nil || string(text.VTT) != "WEBVTT" {
		t.Errorf("its converted text: %+v, %v; want the node's", text, err)
	}
}

// fetchedCount is how many subtitles fetched for the bench's video persistent.db keeps.
func (bench *fetchBench) fetchedCount(t *testing.T) int {
	t.Helper()
	rows, err := bench.database.Read().ListFetchedSubtitlesOfVideo(t.Context(), bareVideoFileID)
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

// A subtitle fetched on request is converted by the video's node at play priority while the person
// waits, kept with its converted text once the node answers, and given as the file's track; asking
// for it again spends no second download (D224, D244 §4).
func TestFetch_KeepsWhatItsNodeConvertedAndDoesNotDownloadItAgain(t *testing.T) {
	t.Parallel()
	bench := newFetchBench(t)
	bench.node.answer = func(extract nodeproto.ExtractSubtitle) nodeproto.SubtitleText {
		if extract.FetchedSha256 == "" || extract.Video != nil {
			t.Errorf("the node was asked %+v; want a fetched subtitle's conversion by its hash", extract)
		}
		return nodeproto.SubtitleText{ASS: []byte("[Script Info]"), VTT: []byte("WEBVTT")}
	}
	fetched, err := bench.fetch(t)
	if err != nil {
		t.Fatalf("the fetch: %v", err)
	}
	if fetched.Track.FetchedSubtitleID == 0 || fetched.RemainingDownloads != 19 {
		t.Errorf("fetched %+v; want the kept subtitle as the track and the downloads left", fetched)
	}
	bench.mustHaveConvertedAtPlayPriority(t, fetched)
	again, err := bench.fetch(t)
	if err != nil || again.Track != fetched.Track || again.RemainingDownloads != 20 {
		t.Errorf("asked again: %+v, %v; want the same track and the allowance, no download", again, err)
	}
	if count := bench.subtitler.downloadCount(); count != 1 {
		t.Errorf("downloaded %d times; want once", count)
	}
}

// A fetched subtitle its node could not read is ErrUnreadable and kept nowhere, nor the node's word
// on it (D224 §5, §6).
func TestFetch_OneItsNodeCouldNotReadIsKeptNowhere(t *testing.T) {
	t.Parallel()
	bench := newFetchBench(t)
	bench.node.answer = func(nodeproto.ExtractSubtitle) nodeproto.SubtitleText {
		return nodeproto.SubtitleText{Failure: "not SubRip"}
	}
	if _, err := bench.fetch(t); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("the fetch: %v; want ErrUnreadable", err)
	}
	if count := bench.fetchedCount(t); count != 0 {
		t.Errorf("%d fetched subtitles kept; want none", count)
	}
	_, err := bench.database.Read().GetSubtitleText(t.Context(), fetchedKey(mustGzip(t, bench.subtitler.body)))
	if err == nil {
		t.Error("the converter's word on it is still kept")
	}
}

// A fetched subtitle its node did not convert within the silence limit is nodes.ErrNodeSilent and
// kept nowhere (D224 §5).
func TestFetch_OneItsNodeDidNotConvertInTimeIsKeptNowhere(t *testing.T) {
	t.Parallel()
	bench := newFetchBench(t)
	bench.queue.onEnqueue = nil // the conversion is never run: the node says nothing
	done := make(chan error, 1)
	go func() {
		_, err := bench.fetch(t)
		done <- err
	}()
	// The silence limit is the only timer: move the clock to it once the fetch has set it.
	for !bench.clock.AdvanceToNext() {
		select {
		case err := <-done:
			t.Fatalf("the fetch answered %v before its silence limit", err)
		default:
			runtime.Gosched()
		}
	}
	if err := testutil.Receive(t, done, "the fetch's answer"); !errors.Is(err, nodes.ErrNodeSilent) {
		t.Fatalf("the fetch: %v; want ErrNodeSilent", err)
	}
	if count := bench.fetchedCount(t); count != 0 {
		t.Errorf("%d fetched subtitles kept; want none", count)
	}
}

// A node that is away is found before a download is spent (D224 §5).
func TestFetch_ANodeThatIsAwayIsFoundBeforeADownloadIsSpent(t *testing.T) {
	t.Parallel()
	bench := newFetchBench(t)
	bench.node.isConnected = false
	if _, err := bench.fetch(t); !errors.Is(err, nodes.ErrNodeSilent) {
		t.Fatalf("the fetch: %v; want ErrNodeSilent", err)
	}
	if count := bench.subtitler.downloadCount(); count != 0 {
		t.Errorf("downloaded %d times; want none", count)
	}
}

// A server started without a provider fetches nothing and lists nothing.
func TestFetch_AServerWithoutAProviderSaysSo(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	if _, err := bench.service.Candidates(t.Context(), bareVideoFileID, "en"); !errors.Is(err, ErrNoSubtitler) {
		t.Errorf("Candidates: %v; want ErrNoSubtitler", err)
	}
	_, err := bench.service.Fetch(t.Context(), 1, bareVideoFileID, FetchRequest{ProviderFileID: 1})
	if !errors.Is(err, ErrNoSubtitler) {
		t.Errorf("Fetch: %v; want ErrNoSubtitler", err)
	}
}

// A file not identified is searched for by its moviehash alone, in the language asked for in
// OpenSubtitles' spelling, and a candidate made for these bytes is listed first, the rest in the
// provider's order (D224 §3).
func TestSubtitleCandidates_AnUnidentifiedFileIsSearchedByItsHashWithAMatchFirst(t *testing.T) {
	t.Parallel()
	bench := newFetchBench(t)
	bench.subtitler.candidates = []opensubtitles.Candidate{
		{ProviderFileID: 1}, {ProviderFileID: 2}, {ProviderFileID: 3, IsOpensubtitlesHashMatch: true},
	}
	offer, err := bench.service.Candidates(t.Context(), bareVideoFileID, "PT-BR")
	if err != nil {
		t.Fatal(err)
	}
	var order []int64
	for _, candidate := range offer.Candidates {
		order = append(order, candidate.ProviderFileID)
	}
	if len(order) != 3 || order[0] != 3 || order[1] != 1 || order[2] != 2 {
		t.Errorf("listed %v; want the hash match first, then the provider's order", order)
	}
	if offer.DidAskByIDs || !offer.DidAskByOpensubtitlesHash || offer.Allowance.RemainingDownloads != 20 {
		t.Errorf("offer %+v; want asked by the hash alone, with the allowance", offer)
	}
	asked := bench.subtitler.queries[0]
	if asked.MovieHash != "8e245d9679d31e12" || asked.Language != "pt-br" || asked.IMDbID != "" {
		t.Errorf("asked %+v; want the file's hash and the language lowercased", asked)
	}
}

// A file neither identified nor hashed has nothing to name it to the provider.
func TestSubtitleCandidates_AFileWithNothingToSearchBySaysSo(t *testing.T) {
	t.Parallel()
	bench := newFetchBench(t)
	storetest.ExecRaw(t, bench.database.Path(), "UPDATE file SET opensubtitles_hash = NULL WHERE id = 1")
	if _, err := bench.service.Candidates(t.Context(), bareVideoFileID, "en"); !errors.Is(err, ErrNothingToSearchBy) {
		t.Errorf("Candidates: %v; want ErrNothingToSearchBy", err)
	}
	if len(bench.subtitler.queries) != 0 {
		t.Error("the provider was searched")
	}
}

// One fetch of a subtitle for a file runs at a time: a second ask waits until the first ends, or
// gives up when its own context ends; another file's fetch does not wait (D244 §4).
func TestFetchesUnderway_HoldOneFetchOfASubtitleAtATime(t *testing.T) {
	t.Parallel()
	underway := newFetchesUnderway()
	end, err := underway.begin(t.Context(), fetchKey(1, "7"))
	if err != nil {
		t.Fatal(err)
	}
	otherEnd, err := underway.begin(t.Context(), fetchKey(2, "7"))
	if err != nil {
		t.Fatalf("another file's fetch: %v; want it not to wait", err)
	}
	otherEnd()
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := underway.begin(cancelled, fetchKey(1, "7")); !errors.Is(err, context.Canceled) {
		t.Errorf("a second ask whose context ended: %v; want it to stop waiting", err)
	}
	second := make(chan error, 1)
	go func() {
		secondEnd, beginErr := underway.begin(t.Context(), fetchKey(1, "7"))
		if beginErr == nil {
			secondEnd()
		}
		second <- beginErr
	}()
	select {
	case <-second:
		t.Fatal("the second ask began while the first was underway")
	default:
	}
	end()
	if err := testutil.Receive(t, second, "the second ask"); err != nil {
		t.Errorf("the second ask: %v", err)
	}
}

// Everyone waiting for a text is told once it is kept, and only for its own key; a wait begun after
// is told the next time (keptTexts).
func TestKeptTexts_TellEveryoneWaitingForTheirTextOnly(t *testing.T) {
	t.Parallel()
	texts := newKeptTexts()
	one, other := textKey([]byte{1}, nil), textKey([]byte{2}, nil)
	first, second, others := texts.await(one), texts.await(one), texts.await(other)
	texts.wasKept(one)
	for _, kept := range []<-chan struct{}{first, second} {
		select {
		case <-kept:
		default:
			t.Error("a wait for the kept text was not told")
		}
	}
	later := texts.await(one)
	select {
	case <-others:
		t.Error("a wait for another text was told")
	case <-later:
		t.Error("a wait begun after was told of the earlier keeping")
	default:
	}
}

// A node's answer is kept where its command's source says: a subtitle file's by its payload hash, a
// fetched subtitle's by its body's hash; a refusal or a stall keeps nothing; a text for a track the
// command did not name is malformed (record).
func TestRecord_KeepsEachAnswerWhereItsSourceSays(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	ctx := t.Context()
	recordOf := func(command nodeproto.ExtractSubtitle, answer nodeproto.SubtitleText) error {
		return bench.service.record(ctx, fakeNodeID, nodeproto.Result{
			Work: &nodeproto.Work{ExtractSubtitle: &command}, Subtitle: &answer,
		})
	}
	sidecar := nodeproto.ExtractSubtitle{Sidecar: &nodeproto.BytesFile{PayloadHash: "0a"}}
	if err := recordOf(sidecar, nodeproto.SubtitleText{Refusal: "changed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := bench.database.Read().GetSubtitleText(ctx, textKey([]byte{0x0a}, nil)); err == nil {
		t.Error("a refusal was kept")
	}
	if err := recordOf(sidecar, nodeproto.SubtitleText{ASS: []byte("a")}); err != nil {
		t.Fatal(err)
	}
	row, err := bench.database.Read().GetSubtitleText(ctx, textKey([]byte{0x0a}, nil))
	if err != nil || string(row.ASS) != "a" {
		t.Errorf("the subtitle file's text: %+v, %v; want its ASS kept", row, err)
	}
	fetched := nodeproto.ExtractSubtitle{FetchedSha256: "0b"}
	if err = recordOf(fetched, nodeproto.SubtitleText{Failure: "unreadable"}); err != nil {
		t.Fatal(err)
	}
	row, err = bench.database.Read().GetSubtitleText(ctx, textKey([]byte{0x0b}, nil))
	if err != nil || row.Failure == nil || *row.Failure != "unreadable" {
		t.Errorf("the fetched subtitle's failure: %+v, %v; want it kept", row, err)
	}
	video := nodeproto.ExtractSubtitle{Video: &nodeproto.BytesFile{PayloadHash: "01"}, StreamIndexes: []int{2}}
	stray := nodeproto.SubtitleText{Tracks: []nodeproto.TrackText{{StreamIndex: 5, ASS: []byte("x")}}}
	if err := recordOf(video, stray); !errors.Is(err, nodes.ErrMalformedResult) {
		t.Errorf("a text for a track not asked for: %v; want ErrMalformedResult", err)
	}
	if err := recordOf(nodeproto.ExtractSubtitle{Sidecar: &nodeproto.BytesFile{PayloadHash: "zz"}},
		nodeproto.SubtitleText{ASS: []byte("a")}); !errors.Is(err, nodes.ErrMalformedResult) {
		t.Errorf("a source hash not hex: %v; want ErrMalformedResult", err)
	}
}

// The subtitles fetched for a file confirmed a copy of another move onto that file, one fetched for
// both kept once; those of a purged file are deleted with it (D363 §2, D306 §8).
func TestForget_FetchedSubtitlesMoveToTheCopysFileAndGoWithAPurgedFile(t *testing.T) {
	t.Parallel()
	bench := newFetchBench(t)
	bench.node.answer = func(nodeproto.ExtractSubtitle) nodeproto.SubtitleText {
		return nodeproto.SubtitleText{ASS: []byte("a"), VTT: []byte("v")}
	}
	if _, err := bench.fetch(t); err != nil {
		t.Fatal(err)
	}
	librarytest.File(t, bench.database, librarytest.FileSpec{
		ID: 2, NodeID: fakeNodeID, Path: "/m/f2.mkv", PayloadHash: []byte{0x01}, Filing: "video",
	})
	ctx := t.Context()
	write := func(do func(queries *sqlcgen.Queries) error) {
		t.Helper()
		if err := bench.database.Write(ctx, do); err != nil {
			t.Fatal(err)
		}
	}
	write(func(queries *sqlcgen.Queries) error {
		return MoveFetchedToCopysFile(ctx, queries, bareVideoFileID, 2)
	})
	if count := bench.fetchedCount(t); count != 0 {
		t.Errorf("%d still fetched for the copy; want them moved", count)
	}
	moved, err := bench.database.Read().ListFetchedSubtitlesOfVideo(ctx, 2)
	if err != nil || len(moved) != 1 {
		t.Fatalf("fetched for the file it is a copy of: %d, %v; want the one", len(moved), err)
	}
	write(func(queries *sqlcgen.Queries) error { return ForgetSubtitlesOfPurgedFiles(ctx, queries, "[2]") })
	left, err := bench.database.Read().ListFetchedSubtitlesOfVideo(ctx, 2)
	if err != nil || len(left) != 0 {
		t.Errorf("fetched for the purged file: %d, %v; want none", len(left), err)
	}
}

// mustGzip is body as persistent.db keeps a fetched subtitle.
func mustGzip(t *testing.T, body []byte) []byte {
	t.Helper()
	gzipped, err := bounded.Gzip(body)
	if err != nil {
		t.Fatal(err)
	}
	return gzipped
}
