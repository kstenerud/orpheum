// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
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
	base := api.Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
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
	base := api.Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
	checkFilmOnesMaster(t, harness.getBytePath(t, base+"master.m3u8", user, true, ""), "sound0-96k-raw")
	harness.measureFilmOnesLoudness(t)
	fresh := api.Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
	// Film 1's stereo AAC, which Chrome plays as it is, is copied at the top; its light track, once
	// the loudness is measured, is made as loud as it (D368 §1).
	checkFilmOnesMaster(t, harness.getBytePath(t, fresh+"master.m3u8", user, true, ""), "sound0-96k-ownlevel")
	checkFilmOnesMaster(t, harness.getBytePath(t, base+"master.m3u8", user, true, ""), "sound0-96k-raw")
	if raw := harness.getBytePath(t, base+"sound0-96k-raw/stream.m3u8", user, true, ""); raw.status != http.StatusOK {
		t.Errorf("the unlevelled sound's playlist, once the loudness is measured: %d %s; want it still", raw.status,
			raw.body)
	}
}

// A person who plays without levelling is offered the film's sound carrying no levelling, though its
// loudness is measured: nothing for the player to take back out through Web Audio, whose latency
// plays the sound later on an Android TV (the owner's TCL TV, 2026-10-24). A player that cannot play
// the film's own sound is offered its mix unlevelled; one that plays it copied at the top, its mix as
// loud as it, levelling or not, which its player then levels as it levels the copy, by the decision's
// levelling, or not at all (D368 §1).
func TestChunks_APersonWithoutLevellingIsOfferedTheSoundUnlevelled(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.keepFilmOnesTimeline(t)
	harness.measureFilmOnesLoudness(t)
	master := func(started decision) byteAnswer {
		path := api.Prefix + "/bytes/media/chunks/" + started.MediaToken + "/master.m3u8"
		return harness.getBytePath(t, path, user, true, "")
	}
	copied := master(harness.play(t, user, chrome(1)))
	checkFilmOnesMaster(t, copied, "sound0-96k-ownlevel")
	checkStartingSound(t, copied, "sound0-own")
	answer := harness.do(t, http.MethodPut, "/me/preferences", user, map[string]any{"isLevellingOn": false})
	if answer.status != http.StatusNoContent && answer.status != http.StatusOK {
		t.Fatalf("PUT /me/preferences: %d %s", answer.status, answer.body)
	}
	unlevelled := master(harness.play(t, user, chrome(1)))
	checkFilmOnesMaster(t, unlevelled, "sound0-96k-ownlevel")
	checkStartingSound(t, unlevelled, "sound0-own")
	// A player that gave the film's own picture up starts at its twin, which plays the mix, unlevelled.
	twin := chrome(1)
	twin["hasGivenUpOwnPicture"] = true
	checkStartingSound(t, master(harness.play(t, user, twin)), "sound0-96k-raw")
}

// checkStartingSound fails the test unless a master playlist starts its top on sound's stream.
func checkStartingSound(t *testing.T, master byteAnswer, sound string) {
	t.Helper()
	first := strings.Index(master.body, "DEFAULT=YES")
	if master.status != http.StatusOK || first < 0 ||
		!strings.HasPrefix(master.body[first:], `DEFAULT=YES,AUTOSELECT=YES,CHANNELS="2",URI="`+sound+`/stream.m3u8"`) {
		t.Errorf("the master playlist: %d\n%s\nwant its top to start on %s", master.status, master.body, sound)
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
	base := api.Prefix + "/bytes/media/chunks/" + playing.MediaToken + "/"
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
	base := api.Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
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
		chunk.header.Get("Cache-Control") != api.CacheForADay {
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
	base := api.Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
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
		`"outcome":"askAgain"`, `"heldMs":0,`, `"isSought":false,`)
}

// A player that says a seek took it where it asks, by Orpheum-Sought, is taken at its word: the
// chunk's wait says so in the log, and a run starts there sooner (D284 §2).
func TestChunks_APlayerSayingASeekTookItThereIsTakenAtItsWord(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	base := api.Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
	harness.keepFilmOnesTimeline(t)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		harness.httpServer.URL+base+"q700k432p/00001.m4s", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+user)
	request.Header.Set("Orpheum-Sought", "true")
	answer, err := harness.httpServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = answer.Body.Close() //nolint:errcheck // its status is all that is read
	if answer.StatusCode != http.StatusTooEarly {
		t.Errorf("a chunk not made yet, asked for after a seek: %d, want 425", answer.StatusCode)
	}
	harness.logs.AwaitLine(t, `"msg":"waited for a chunk not made yet"`, `"wantedChunk":1,`, `"isSought":true,`)
}

// A jump's picture is made with its sound: once the player has asked for a sound, the picture's
// first chunk asked for where a seek took it starts the sound there too, as hls.js asks for a seek's
// sound only once that chunk has come (the owner's test, 2026-10-12: 5 s of a 12.5 s jump).
func TestChunks_AJumpsPictureStartsItsSoundThereToo(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	base := api.Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
	harness.keepFilmOnesTimeline(t)
	harness.measureFilmOnesLoudness(t)
	if early := harness.getBytePath(t, base+"sound0-96k/00002.m4s", user, true, ""); early.status !=
		http.StatusTooEarly {
		t.Fatalf("the sound's chunk, before the jump: %d %s; want 425", early.status, early.body)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		harness.httpServer.URL+base+"q700k432p/00001.m4s", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+user)
	request.Header.Set("Orpheum-Sought", "true")
	answer, err := harness.httpServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = answer.Body.Close() //nolint:errcheck // its status is all that is read
	if answer.StatusCode != http.StatusTooEarly {
		t.Errorf("the jump's picture, not made yet: %d, want 425", answer.StatusCode)
	}
	harness.logs.AwaitLine(t, `"msg":"queued a run of chunks"`, `"storeDir":"1/chunks-sound0-96k/`,
		`"wantedChunk":1,`)
}

// A player that gave up the film's own picture as one its device cannot play is not offered it at
// its next playback of the film; another player, here another session, still is (D266): Firefox on
// a Mac gave Flow's up anew at every playback (the owner's test, 2026-10-12).
func TestChunks_AFilmsOwnPictureGivenUpIsNotOfferedToThePlayersNextPlayback(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	first := harness.play(t, user, chrome(1))
	body := reportBody(1, 420_500, 0)
	body["unplayables"] = []map[string]any{{"qualityKbps": 3096, "positionMs": 420_500, "isFilmsOwnPicture": true}}
	if answer := harness.report(t, user, first, body); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	next := api.Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/master.m3u8"
	if master := harness.getBytePath(t, next, user, true, ""); master.status != http.StatusOK ||
		strings.Contains(master.body, "picture/stream.m3u8") || !strings.Contains(master.body, "q2000k540p") {
		t.Errorf("the next playback's master playlist offers the picture given up, or nothing:\n%s", master.body)
	}
	other := harness.signIn(t, "owner", "owner-password")
	elsewhere := api.Prefix + "/bytes/media/chunks/" + harness.play(t, other, chrome(1)).MediaToken + "/master.m3u8"
	if master := harness.getBytePath(t, elsewhere, other, true, ""); !strings.Contains(master.body,
		"picture/stream.m3u8") {
		t.Errorf("another player's master playlist lost the picture:\n%s", master.body)
	}
	harness.logs.AwaitLine(t, `"msg":"a player found a quality its device cannot play"`, `"isFilmsOwnPicture":true`)
}

// A page asking again because its player gave the film's own picture up starts at the twin, H.264 at
// the picture's own height, whether or not the player's report reached the core first; another player
// still gets the picture (D323 §2).
func TestChunks_APageThatGaveUpTheOwnPictureStartsAgainAtTheTwin(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	// Film one's picture is 1080 lines, the height its twin is made at.
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE file_probe SET probe_json = json_set(probe_json, '$.streams[0].video.width', 1920, `+
			`'$.streams[0].video.height', 1080) WHERE file_id = 1`)
	asked := chrome(1)
	asked["hasGivenUpOwnPicture"] = true
	asked["cause"] = "ownPictureGivenUp"
	again := harness.play(t, user, asked)
	master := harness.getBytePath(t, api.Prefix+"/bytes/media/chunks/"+again.MediaToken+"/master.m3u8", user, true, "")
	if master.status != http.StatusOK || strings.Contains(master.body, "picture/stream.m3u8") ||
		!strings.Contains(master.body, "q2400k1080p/stream.m3u8") {
		t.Errorf("a page that gave up the picture is not offered the twin, H.264 at 1080p at 2.4 Mb/s:\n%s",
			master.body)
	}
	other := harness.signIn(t, "owner", "owner-password")
	elsewhere := api.Prefix + "/bytes/media/chunks/" + harness.play(t, other, chrome(1)).MediaToken + "/master.m3u8"
	if master := harness.getBytePath(t, elsewhere, other, true, ""); !strings.Contains(master.body,
		"picture/stream.m3u8") {
		t.Errorf("another player's master playlist lost the picture:\n%s", master.body)
	}
}

// A chunk of a quality whose run was evicted to keep its machine's other runs faster than playback
// is answered 429, which nothing else on a chunk's path answers, so the player stops and says why
// (D250 §6).
func TestChunks_AnEvictedQualityIsRefusedWithItsOwnStatusAndCode(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	err := harness.server.ChunksError(t.Context(), "have a chunk made", fmt.Errorf("1/chunks-q700k432p/x: %w",
		chunks.ErrNoRoomToMake))
	var refused *api.Error
	if !errors.As(err, &refused) || refused.GetStatus() != http.StatusTooManyRequests ||
		refused.ErrorBody.Error.Code != api.CodeNoRoomToMake {
		t.Errorf("an evicted quality's chunk = %v; want 429 %s", err, api.CodeNoRoomToMake)
	}
}

// A chunk whose runs FFmpeg failed on the same way each time is answered 422 cannot_convert, which
// the player does not ask again after, not 425 for as long as it waits (file 15102 on nas.example,
// 2026-10-27); FFmpeg's word, which may name the film, stays in the core's log.
func TestChunks_AChunkThatCannotBeMadeIsRefusedForGood(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	err := harness.server.ChunksError(t.Context(), "have a chunk made", fmt.Errorf("1/chunks-sound0-96k-raw/x "+
		"chunk 0: %w: raw 00000.mp4 holds less", chunks.ErrCannotBeMade))
	var refused *api.Error
	if !errors.As(err, &refused) || refused.GetStatus() != http.StatusUnprocessableEntity ||
		refused.ErrorBody.Error.Code != api.CodeCannotConvert || strings.Contains(err.Error(), "raw 00000") {
		t.Errorf("a chunk that cannot be made = %v; want 422 %s, without FFmpeg's word", err, api.CodeCannotConvert)
	}
}

func TestChunks_AnotherPersonsTokenIsRefused(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	base := api.Prefix + "/bytes/media/chunks/" + harness.play(t, admin, chrome(1)).MediaToken + "/"
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

// A chunk not made yet of a film whose root has become unavailable, its disk not answering or its
// node away, is answered media_unreachable at once, saying why, rather than "being prepared" for as
// long as the player asks: no run will make it (D241 §7; staging, 2026-10-21, nas.example off).
func TestChunks_AnUnmadeChunkOfAFilmWhoseRootIsUnavailableIsUnreachable(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	base := api.Prefix + "/bytes/media/chunks/" + harness.play(t, user, chrome(1)).MediaToken + "/"
	harness.keepFilmOnesTimeline(t)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at)
		 VALUES (1, '/media', 'unreadable', 5000)`,
		`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (1, '/media', '/media', 4000, 'unread')`)
	unmade := harness.getBytePath(t, base+"q1200k480p/00000.m4s", user, true, "")
	if unmade.status != http.StatusServiceUnavailable || !strings.Contains(unmade.body, `"code":"media_unreachable"`) ||
		!strings.Contains(unmade.body, `"reason":"root_missing"`) {
		t.Errorf("an unmade chunk under an unavailable root: %d %s; want media_unreachable, its root missing",
			unmade.status, unmade.body)
	}
}
