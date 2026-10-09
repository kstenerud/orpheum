// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// keepFilmOnesTimeline keeps the timeline a read-through of film 1 finds: a keyframe every ten
// seconds of its thirty, and 300 kB in each second, 2.4 Mb/s.
func (harness testHarness) keepFilmOnesTimeline(t *testing.T) {
	t.Helper()
	err := harness.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.UpsertVideoTimeline(t.Context(), sqlcgen.UpsertVideoTimelineParams{
			PayloadHash: []byte{0x0a, 0x0b}, KeyframesMsJSON: "[0,10000,20000]",
			BytesInEachSecondJSON: "[" + strings.Repeat("300000,", 29) + "300000]",
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// openJobs are the jobs open in the queue, of kind.
func (harness testHarness) openJobs(t *testing.T, kind jobs.Kind) []cachegen.Job {
	t.Helper()
	all, err := harness.cache.Read().ListEveryOpenJob(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var open []cachegen.Job
	for _, job := range all {
		if job.Kind == string(kind) {
			open = append(open, job)
		}
	}
	return open
}

func TestChunks_AFilmWithNoGridIsTooEarlyAndReadThroughThenListsWhatThePlaybackIsOffered(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	base := Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
	early := harness.getBytePath(t, base+"master.m3u8", user, true, "")
	if early.status != http.StatusTooEarly || early.header.Get("Retry-After") != "1" {
		t.Errorf("before film 1 has a grid: %d %v, want 425 with Retry-After 1", early.status, early.header)
	}
	if read := harness.openJobs(t, jobs.KindReadThrough); len(read) != 1 || read[0].FileID == nil ||
		*read[0].FileID != 1 || read[0].Priority != jobs.PriorityPlayNow {
		t.Errorf("the read-throughs queued = %+v; want film 1's at play priority", read)
	}
	harness.keepFilmOnesTimeline(t)
	// Film 1's loudness is not measured yet: its sound is offered unlevelled (D259 §2).
	checkFilmOnesMaster(t, harness.getBytePath(t, base+"master.m3u8", user, true, ""), "sound0-96k-raw")
	stream := harness.getBytePath(t, base+"q1200k480p/stream.m3u8", user, true, "")
	if stream.status != http.StatusOK || !strings.Contains(stream.body, "#EXTINF:10.000,\n00002.m4s\n#EXT-X-ENDLIST") {
		t.Errorf("a quality's playlist: %d\n%s", stream.status, stream.body)
	}
	for _, unoffered := range []string{
		"q3000k720p/stream.m3u8", "sound1/00000.m4s", "picture/00003.m4s", "picture/notes.txt",
		"sound0-boost-96k/init.mp4", "sound0-192k/init.mp4",
	} {
		if refused := harness.getBytePath(t, base+unoffered, user, true, ""); refused.status != http.StatusNotFound {
			t.Errorf("%s: %d %s, want 404", unoffered, refused.status, refused.body)
		}
	}
}

// A film cut before its loudness was measured, as one cut from its file's index is, offers its sound
// unlevelled, a store of its own; once measured, a new playback's playlist offers it levelled, and a
// playback that started on the unlevelled sound goes on with it, its playlist asked again, as a
// stall's move to a lower quality asks it, naming it still (D259 §2).
func TestChunks_ASoundOfferedBeforeItsLoudnessIsUnlevelledAndAPlaybackKeepsIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	base := Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
	checkFilmOnesMaster(t, harness.getBytePath(t, base+"master.m3u8", user, true, ""), "sound0-96k-raw")
	harness.measureFilmOnesLoudness(t)
	fresh := Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
	checkFilmOnesMaster(t, harness.getBytePath(t, fresh+"master.m3u8", user, true, ""), "sound0-96k")
	checkFilmOnesMaster(t, harness.getBytePath(t, base+"master.m3u8", user, true, ""), "sound0-96k-raw")
	if raw := harness.getBytePath(t, base+"sound0-96k-raw/stream.m3u8", user, true, ""); raw.status != http.StatusOK {
		t.Errorf("the unlevelled sound's playlist, once the loudness is measured: %d %s; want it still", raw.status,
			raw.body)
	}
}

// A playback whose sound was offered unlevelled keeps it so: once the film's loudness is measured,
// its stall starts, and its offer waits on, the unlevelled sound its player plays, not a levelled one
// it never asks for (the M4 closing audit, §6).
func TestChunks_AStallStartsTheUnlevelledSoundItsPlaybackKept(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	playing := harness.play(t, user, chrome(1))
	harness.keepFilmOnesTimeline(t)
	base := Prefix + "/bytes/media/chunks/" + playing.MediaToken + "/"
	checkFilmOnesMaster(t, harness.getBytePath(t, base+"master.m3u8", user, true, ""), "sound0-96k-raw")
	harness.measureFilmOnesLoudness(t)
	if offered := lowerQualityOf(t, harness.report(t, user, playing, stallReport(1, 21_000))); offered == nil {
		t.Fatal("the stall was offered no lower quality")
	}
	var streams []string
	for _, run := range harness.openJobs(t, jobs.KindMakeChunks) {
		if run.ChunkStoreID == nil {
			continue
		}
		store, err := harness.cache.Read().GetChunkStore(t.Context(), *run.ChunkStoreID)
		if err != nil {
			t.Fatal(err)
		}
		streams = append(streams, store.Stream)
	}
	slices.Sort(streams)
	if !slices.Equal(streams, []string{"q1200k480p", "sound0-96k-raw"}) {
		t.Errorf("the stall started runs of %v; want the quality's and the unlevelled sound's", streams)
	}
}

// measureFilmOnesLoudness records film 1's first audio track's loudness as measured.
func (harness testHarness) measureFilmOnesLoudness(t *testing.T) {
	t.Helper()
	lufs := -20.0
	if err := harness.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.UpsertLoudness(t.Context(), sqlcgen.UpsertLoudnessParams{
			PayloadHash: []byte{0x0a, 0x0b}, StreamIndex: 1, MeasuredMix: "stereo", IntegratedLUFS: &lufs,
			MeasuredAt: 1,
		})
	}); err != nil {
		t.Fatal(err)
	}
}

func TestChunks_AChunkTheNodeHasNotMadeIsAskedForAndRelayedOnceItIs(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	// Played before the film is cut, so not on its ladder: no run is started for its start (D260 §1).
	base := Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
	harness.keepFilmOnesTimeline(t)
	grid, _, err := harness.chunks.Grid(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	store := nodeproto.PathBytesChunks + "1/chunks-q700k432p/" + grid.ID() + "/"
	harness.nodeBytes.mutex.Lock()
	harness.nodeBytes.files[store+"00001.m4s"] = "chunk one"
	harness.nodeBytes.asksBeforeFound = map[string]int{store + "00001.m4s": 3}
	harness.nodeBytes.mutex.Unlock()
	harness.setSetting(t, "chunk_hold_ms", "20000") // a hold, where one is set
	chunk := harness.getBytePath(t, base+"q700k432p/00001.m4s", user, true, "")
	if chunk.status != http.StatusOK || chunk.body != "chunk one" ||
		chunk.header.Get("Content-Type") != "video/iso.segment" ||
		chunk.header.Get("Cache-Control") != cacheForADay {
		t.Errorf("a chunk made while the core waited: %d %v %q, want it, cached", chunk.status, chunk.header,
			chunk.body)
	}
	harness.checkOneRunQueued(t, grid.ID())
	// The wait is logged: the chunk, its hold of three quarter-second waits, and the run queued for it,
	// which had made none of its chunks when asked.
	waited := `"msg":"waited for a chunk not made yet"`
	storeDir := `"storeDir":"1/chunks-q700k432p/` + grid.ID() + `"`
	harness.logs.AwaitLine(t, waited, storeDir, `"wantedChunk":1,`, `"outcome":"served"`, `"heldMs":750,`,
		`"isCovered":true`, `"runFirstChunk":1,`, `"runLastChunk":2,`, `"runMadeThrough":0`)
	// One no run makes in the time a player waits is too early: the player asks again.
	if late := harness.getBytePath(t, base+"q700k432p/00002.m4s", user, true, ""); late.status !=
		http.StatusTooEarly || late.header.Get("Retry-After") != "1" {
		t.Errorf("a chunk not made in time: %d %v, want 425 with Retry-After 1", late.status, late.header)
	}
	harness.logs.AwaitLine(t, waited, storeDir, `"wantedChunk":2,`, `"outcome":"askAgain"`, `"heldMs":20000,`,
		`"runFirstChunk":1,`)
	// A chunk the node has at once was not waited for, and says nothing.
	if again := harness.getBytePath(t, base+"q700k432p/00001.m4s", user, true, ""); again.status != http.StatusOK {
		t.Errorf("a chunk made, asked for again: %d, want it", again.status)
	}
	if count := strings.Count(harness.logs.String(), waited); count != 2 {
		t.Errorf("%d waits logged; want 2, none for a chunk the node had at once:\n%s", count, harness.logs.String())
	}
}

// By default a chunk not made yet is answered at once: the player is told to ask again, the run
// that makes it queued, and nothing held, since a player counts a hold as the chunk's download
// (D250 §5).
func TestChunks_AChunkNotMadeIsAnsweredAtOnceByDefault(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	// Played before the film is cut, so not on its ladder: no run is started for its start (D260 §1).
	base := Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
	harness.keepFilmOnesTimeline(t)
	grid, _, err := harness.chunks.Grid(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if early := harness.getBytePath(t, base+"q700k432p/00001.m4s", user, true, ""); early.status !=
		http.StatusTooEarly || early.header.Get("Retry-After") != "1" {
		t.Errorf("a chunk not made yet: %d %v, want 425 with Retry-After 1", early.status, early.header)
	}
	harness.checkOneRunQueued(t, grid.ID())
	harness.logs.AwaitLine(t, `"msg":"waited for a chunk not made yet"`, `"wantedChunk":1,`,
		`"outcome":"askAgain"`, `"heldMs":0,`)
}

// A chunk of a quality whose run was evicted to keep its machine's other runs faster than playback
// is answered 429, which nothing else on a chunk's path answers, so the player stops and says why
// (D250 §6).
func TestChunks_AnEvictedQualityIsRefusedWithItsOwnStatusAndCode(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	err := harness.server.chunksError(t.Context(), "have a chunk made", fmt.Errorf("1/chunks-q700k432p/x: %w",
		chunks.ErrNoRoomToMake))
	var refused *Error
	if !errors.As(err, &refused) || refused.GetStatus() != http.StatusTooManyRequests ||
		refused.ErrorBody.Error.Code != CodeNoRoomToMake {
		t.Errorf("an evicted quality's chunk = %v; want 429 %s", err, CodeNoRoomToMake)
	}
}

func TestChunks_AnotherPersonsTokenIsRefused(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	base := Prefix + "/bytes/media/chunks/" + harness.play(t, admin, chrome(1)).MediaToken + "/"
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	guest := harness.signIn(t, "partner", "partner-password")
	for _, path := range []string{"master.m3u8", "picture/stream.m3u8", "picture/00000.m4s"} {
		if refused := harness.getBytePath(t, base+path, guest, true, ""); refused.status != http.StatusNotFound {
			t.Errorf("%s by another person: %d %s, want 404", path, refused.status, refused.body)
		}
	}
}

// checkFilmOnesMaster fails the test unless master is film 1's master playlist: its own picture, then
// the qualities below its 2.4 Mb/s, with its one stereo track.
func checkFilmOnesMaster(t *testing.T, master byteAnswer, sound string) {
	t.Helper()
	if master.status != http.StatusOK || master.header.Get("Content-Type") != "application/vnd.apple.mpegurl" ||
		!strings.Contains(master.body, `NAME="`+sound+`",LANGUAGE="und",DEFAULT=YES`) ||
		strings.Contains(master.body, "sound0-boost") || strings.Contains(master.body, "q3000k720p") ||
		!strings.Contains(master.body, "\npicture/stream.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=3096000,"+
			`AVERAGE-BANDWIDTH=2096000,AUDIO="audio"`+"\nq2000k540p/stream.m3u8\n") ||
		!strings.HasSuffix(master.body, "\nq400k360p/stream.m3u8\n") {
		t.Errorf("film 1's master playlist: %d %v\n%s", master.status, master.header, master.body)
	}
}

// checkOneRunQueued fails the test unless one run is queued, at play priority, of chunks 1 to the
// last, 2, of the store of film 1's q700k432p on the grid gridID.
func (harness testHarness) checkOneRunQueued(t *testing.T, gridID string) {
	t.Helper()
	runs := harness.openJobs(t, jobs.KindMakeChunks)
	var args struct {
		FirstChunk int `json:"firstChunk"`
		LastChunk  int `json:"lastChunk"`
	}
	if len(runs) != 1 || runs[0].ChunkStoreID == nil || json.Unmarshal([]byte(runs[0].ArgsJSON), &args) != nil ||
		args.FirstChunk != 1 || args.LastChunk != 2 || runs[0].Priority != jobs.PriorityPlayNow {
		t.Fatalf("the runs queued = %+v (%+v); want one of the quality's store from chunk 1 to the last, at "+
			"play priority", runs, args)
	}
	if store, err := harness.cache.Read().GetChunkStore(t.Context(), *runs[0].ChunkStoreID); err != nil ||
		store.SourceFile != 1 || store.Stream != "q700k432p" || store.GridID != gridID {
		t.Errorf("the run's store = %+v, %v; want film 1's q700k432p on its grid", store, err)
	}
}
