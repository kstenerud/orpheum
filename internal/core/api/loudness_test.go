// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// shelveAudioStreams gives the shelved films the audio streams a scan records: the MP4's stereo
// track and the MKV's eight-channel one.
func (harness testHarness) shelveAudioStreams(t *testing.T) {
	t.Helper()
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, channel_count)
		 VALUES (1, 1, 0, 'audio', 'aac', 2), (2, 2, 1, 'audio', 'truehd', 8)`)
}

// measureWith runs a measurement job for fileID, answers the node's command with answer, and
// returns the command and how the job ended.
func (harness testHarness) measureWith(
	t *testing.T, jobID, fileID int64, answer nodeproto.LoudnessResult,
) (*nodeproto.MeasureLoudness, error) {
	t.Helper()
	service := harness.server.dependencies.Load().Playback
	finished := make(chan error, 1)
	go func() {
		job := jobs.Job{ID: jobID, Kind: jobs.KindMeasureLoudness, Target: jobs.Target{FileID: fileID}}
		finished <- service.HandleMeasureLoudness(t.Context(), job)
	}()
	command := harness.nextCommand(t)
	answer.JobID = jobID
	service.TakeLoudness(t.Context(), 1, answer)
	//nolint:wrapcheck // the job's own error, which the test reads as it is
	return command.MeasureLoudness, testutil.Receive(t, finished, "the measurement job to end")
}

// filesToMeasure is what a sweep would find to measure now.
func (harness testHarness) filesToMeasure(t *testing.T) []int64 {
	t.Helper()
	ids, err := harness.database.Read().ListFilesToMeasure(t.Context(),
		sqlcgen.ListFilesToMeasureParams{AfterID: 0, ThroughID: 1000})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestLoudness_AFileIsMeasuredOnceAndPlaysAtTheLevel(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	service := harness.server.dependencies.Load().Playback
	if queued, err := service.QueueMeasurements(t.Context()); err != nil || queued != 2 {
		t.Fatalf("the sweep queued %d, %v; want both films with sound, not the skipped file (D226 §2)", queued, err)
	}
	unmeasured := harness.play(t, user, chrome(1))
	if unmeasured.Levelling.IsMeasured {
		t.Errorf("before the measurement: %+v, want it played as it is (D226 §6)", unmeasured.Levelling)
	}

	// Wing's ten minutes: quiet, with room below its peak for the whole gain.
	alien := nodeproto.Measurement{IntegratedLUFS: -35.7, RangeLU: 22.8, TruePeakDBTP: new(-16.2)}
	command, err := harness.measureWith(t, 90, 1, nodeproto.LoudnessResult{Tracks: []nodeproto.TrackLoudness{
		{StreamIndex: 0, Stereo: alien},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if command == nil || command.Video.PayloadHash != "0a0b" || string(command.Video.Path) != "Film (2001).mp4" ||
		!slices.Equal(command.Tracks, []nodeproto.LoudnessTrack{{StreamIndex: 0, ChannelCount: 2}}) {
		t.Errorf("the node was asked to measure %+v, want the MP4 by root, path and hash, with its stereo track",
			command)
	}
	if ids := harness.filesToMeasure(t); !slices.Equal(ids, []int64{2}) {
		t.Errorf("after measuring film 1, a sweep finds %v, want only film 2", ids)
	}
	direct := harness.play(t, user, chrome(1))
	if direct.Delivery != "direct" || direct.Levelling != (LevellingBody{IsMeasured: true, GainDB: 12.7}) {
		t.Errorf("played directly: %s, %+v; want raised 12.7 dB to -23 LUFS (D226 §3)",
			direct.Delivery, direct.Levelling)
	}
}

func TestLoudness_APreparedFilmIsLevelledAsItsStereoMixAndItsCopyCarriesTheGain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	// The 7.1 track is louder as it is than as its renormalised stereo mix; the mix's gain is
	// capped by its peak.
	_, err := harness.measureWith(t, 91, 2, nodeproto.LoudnessResult{Tracks: []nodeproto.TrackLoudness{{
		StreamIndex:  1,
		Multichannel: &nodeproto.Measurement{IntegratedLUFS: -20.0, RangeLU: 10, TruePeakDBTP: new(-1.0)},
		Stereo:       nodeproto.Measurement{IntegratedLUFS: -31.0, RangeLU: 9, TruePeakDBTP: new(-6.0)},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	decided := harness.play(t, user, chrome(2))
	want := LevellingBody{IsMeasured: true, GainDB: 5.0}
	if decided.Delivery != "hls" || len(decided.Audio) != 2 || decided.Audio[0].Levelling != want ||
		decided.Audio[1].Levelling != want || decided.Audio[0].CarriedGainDB != 0 || decided.Levelling != want {
		t.Fatalf("prepared: %+v; want both mixes at the stereo mix's gain, capped by its peak, and none carried yet",
			decided)
	}
	prepared := make(chan error, 1)
	go func() {
		prepared <- harness.server.dependencies.Load().Playback.HandlePrepare(t.Context(),
			jobs.Job{ID: 92, Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: 1}})
	}()
	command := harness.nextCommand(t)
	if command.Prepare == nil || len(command.Prepare.AudioTracks) != 1 ||
		command.Prepare.AudioTracks[0].CarriedGainDB != 5.0 {
		t.Fatalf("the prepare sent %+v, want the track's gain in its mixes (D226 §4)", command.Prepare)
	}
	service := harness.server.dependencies.Load().Playback
	done := nodeproto.PrepareReport{JobID: 92, State: nodeproto.PrepareDone, SizeBytes: 1}
	service.TakePrepareReport(t.Context(), 1, done)
	if err := testutil.Receive(t, prepared, "the prepare job to end"); err != nil {
		t.Fatal(err)
	}
	again := harness.play(t, user, chrome(2))
	if again.Audio[0].CarriedGainDB != 5.0 || again.Audio[1].CarriedGainDB != 5.0 {
		t.Errorf("once prepared: %+v; want each mix to say it carries the gain, so the player adds none", again.Audio)
	}
}

func TestLoudness_AFileFFmpegCannotMeasureIsNotAskedAgainButOneTheNodeDidNotTryIs(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	_, err := harness.measureWith(t, 93, 2,
		nodeproto.LoudnessResult{Refusal: "the file at this path is not the one named"})
	if err == nil || !strings.Contains(err.Error(), "not the one named") {
		t.Errorf("a measurement the node would not try ended %v, want its reason", err)
	}
	if ids := harness.filesToMeasure(t); !slices.Equal(ids, []int64{1, 2}) {
		t.Errorf("after the node would not try film 2, a sweep finds %v, want it asked again", ids)
	}
	_, err = harness.measureWith(t, 94, 2, nodeproto.LoudnessResult{Failure: "ffmpeg: Invalid data found"})
	if err == nil || !strings.Contains(err.Error(), "Invalid data found") {
		t.Errorf("a failed measurement ended %v, want FFmpeg's words", err)
	}
	if ids := harness.filesToMeasure(t); !slices.Equal(ids, []int64{1}) {
		t.Errorf("after FFmpeg could not measure film 2, a sweep finds %v, want only film 1", ids)
	}
}

func TestLoudness_AMeasurementTheNodeRefusedIsTriedAgainOnItsBackoff(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	measurement := jobs.Request{Kind: jobs.KindMeasureLoudness, Target: jobs.Target{FileID: 2}, Reason: "test"}
	if _, err := harness.queue.Enqueue(t.Context(), measurement); err != nil {
		t.Fatal(err)
	}
	playback := harness.server.dependencies.Load().Playback
	runner := harness.queue.NewRunner(jobs.KindMeasureLoudness, playback.HandleMeasureLoudness)
	ran := make(chan error, 1)
	go func() { ran <- runner.RunDue(t.Context()) }()
	command := harness.nextCommand(t)
	refusal := "the disk has no room"
	answer := nodeproto.LoudnessResult{JobID: command.MeasureLoudness.JobID, Refusal: refusal}
	playback.TakeLoudness(t.Context(), 1, answer)
	if err := testutil.Receive(t, ran, "the measurement runner to end"); err != nil {
		t.Fatal(err)
	}
	job := newestOf(t, harness.listedJobs(t, admin), jobs.KindMeasureLoudness)
	if job.State != "queued" || !strings.Contains(job.LastFailure, refusal) {
		t.Errorf("the refused measurement = %+v, want queued to be tried again, with the refusal", job)
	}
}

func TestLoudness_LevellingIsOnUntilThePersonTurnsItOff(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	if first := harness.play(t, user, chrome(1)); !first.Preferences.IsLevellingOn {
		t.Errorf("a person who never chose: levelling %t, want on (D226 §5)", first.Preferences.IsLevellingOn)
	}
	answer := harness.do(t, http.MethodPut, "/me/preferences", user, map[string]any{"isLevellingOn": false})
	if answer.status != http.StatusNoContent {
		t.Fatalf("PUT /me/preferences = %d %s", answer.status, answer.body)
	}
	if off := harness.play(t, user, chrome(2)); off.Preferences.IsLevellingOn {
		t.Error("after turning it off: levelling on, want off in every file")
	}
}

// The gain is towards -23 LUFS, both ways, capped by the true peak and at a 20 dB raise, and
// silence is left as it is (D226 §3).
func TestLoudness_TheGainGoesTowardsTheLevelWithinItsLimits(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	for _, check := range []struct {
		what       string
		integrated float64
		peak       *float64
		want       LevellingBody
	}{
		{"a loud film", -18.8, new(0.4), LevellingBody{IsMeasured: true, GainDB: -4.2}},
		{"a quiet film with high peaks", -30.0, new(-3.0), LevellingBody{IsMeasured: true, GainDB: 2.0}},
		{"a near-silent one", -60.0, new(-50.0), LevellingBody{IsMeasured: true, GainDB: 20.0}},
		{"silence", -70.0, nil, LevellingBody{IsMeasured: true, GainDB: 0}},
	} {
		storetest.ExecRaw(t, harness.databasePath, "DELETE FROM loudness")
		integrated := check.integrated
		err := harness.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			return queries.UpsertLoudness(t.Context(), sqlcgen.UpsertLoudnessParams{
				PayloadHash: []byte{0x0a, 0x0b}, StreamIndex: 0, MeasuredMix: "stereo", IntegratedLUFS: &integrated,
				TruePeakDBTP: check.peak, MeasuredAt: 1,
			})
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := harness.play(t, user, chrome(1)).Levelling; got != check.want {
			t.Errorf("%s: %+v, want %+v", check.what, got, check.want)
		}
	}
}

// The level and the most a sound is raised are the admin's: loudness_target_lufs and
// loudness_max_raise_db (D226 §3).
func TestLoudness_TheGainFollowsTheLevelAndTheRaiseTheSettingsSay(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	storetest.ExecRaw(t, harness.databasePath,
		"UPDATE setting SET value_json = '-16', is_modified = 1 WHERE key = 'loudness_target_lufs'",
		"UPDATE setting SET value_json = '10', is_modified = 1 WHERE key = 'loudness_max_raise_db'")
	for _, check := range []struct {
		what       string
		integrated float64
		want       float64
	}{
		{"a film just under the new level", -18.8, 2.8},
		{"a quiet one, raised only as far as allowed", -40.0, 10.0},
	} {
		storetest.ExecRaw(t, harness.databasePath, "DELETE FROM loudness")
		integrated, peak := check.integrated, -30.0
		err := harness.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			return queries.UpsertLoudness(t.Context(), sqlcgen.UpsertLoudnessParams{
				PayloadHash: []byte{0x0a, 0x0b}, StreamIndex: 0, MeasuredMix: "stereo", IntegratedLUFS: &integrated,
				TruePeakDBTP: &peak, MeasuredAt: 1,
			})
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := harness.play(t, user, chrome(1)).Levelling; got.GainDB != check.want {
			t.Errorf("%s at -16 LUFS, raised at most 10 dB: %+v, want a gain of %v", check.what, got, check.want)
		}
	}
}

// A measurement is waited for as long as reading its file takes at the slowest its node says it
// reads; a node that did not say is not sent one it could not be waited for.
func TestLoudness_AMeasurementOnANodeThatDidNotSayHowFastItReadsIsNotSent(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.nodes.isReadRateUnknown = true
	service := harness.server.dependencies.Load().Playback
	job := jobs.Job{ID: 5, Kind: jobs.KindMeasureLoudness, Target: jobs.Target{FileID: 1}}
	err := service.HandleMeasureLoudness(t.Context(), job)
	if err == nil || !strings.Contains(err.Error(), "has not said how fast it reads") {
		t.Errorf("measuring on a node that did not say how fast it reads: %v, want it not waited for", err)
	}
	if len(harness.nodes.queued) != 0 {
		t.Errorf("the node was sent %+v, want nothing", harness.nodes.queued)
	}
}

func TestLoudness_AFileUnderAMissingRootIsNotSweptAndItsMeasurementWaitsForTheRoot(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at)
		 VALUES (1, '/media', 'unreadable', 5000)`)
	if ids := harness.filesToMeasure(t); len(ids) != 0 {
		t.Errorf("with its root unreadable, a sweep finds %v to measure; want none (G6)", ids)
	}
	harness.nodes.commanded = make(chan nodeproto.Command, 1)
	measurement := jobs.Request{Kind: jobs.KindMeasureLoudness, Target: jobs.Target{FileID: 2}, Reason: "test"}
	if _, err := harness.queue.Enqueue(t.Context(), measurement); err != nil {
		t.Fatal(err)
	}
	playback := harness.server.dependencies.Load().Playback
	runner := harness.queue.NewRunner(jobs.KindMeasureLoudness, playback.HandleMeasureLoudness)
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(harness.nodes.commanded) != 0 {
		t.Error("the node was asked to measure a file its missing root holds")
	}
	views, err := harness.queue.List(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if job := views[0]; job.State != "queued" || job.AttemptCount != 0 ||
		job.Destination != jobs.RootDestination(1, "/media") || job.WaitingFor == nil {
		t.Errorf("the measurement = %+v; want queued on the missing root with no attempt spent", job)
	}
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE root_state SET last_scan_outcome = 'complete'`,
		`INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
		 VALUES (1, '/media', '/media', 9, 0)`)
	if ids := harness.filesToMeasure(t); len(ids) != 0 {
		t.Errorf("with the disk under its root missing, a sweep finds %v to measure; want none (G6)", ids)
	}
	storetest.ExecRaw(t, harness.databasePath, `DELETE FROM root_mount`)
	harness.queue.Unpark(t.Context(), jobs.RootDestination(1, "/media"))
	go func() {
		command, isSent := harness.commandWithin(t)
		if !isSent {
			return
		}
		playback.TakeLoudness(t.Context(), 1, nodeproto.LoudnessResult{
			JobID: command.MeasureLoudness.JobID, Failure: "Invalid data found when processing input",
		})
	}()
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if job := newestOf(t, harness.listedJobs(t, admin), jobs.KindMeasureLoudness); job.State != "failed" ||
		job.AttemptCount != 1 {
		t.Errorf("once the root came back, the measurement = %+v; want it run once, to FFmpeg's answer", job)
	}
}

func TestLoudness_AMeasurementThatGaveUpIsKeptAsTheFilesOwnFailureAndNotSweptAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	service := harness.server.dependencies.Load().Playback
	job := jobs.Job{ID: 91, Kind: jobs.KindMeasureLoudness, Target: jobs.Target{FileID: 1}}
	if err := service.RecordMeasurementFailure(t.Context(), job, "node 1 refused five times"); err != nil {
		t.Fatal(err)
	}
	if files := harness.filesToMeasure(t); !slices.Equal(files, []int64{2}) {
		t.Errorf("the sweep finds %v to measure; want only file 2, file 1's failure kept (D231 section 10)", files)
	}
	rows, err := harness.database.Read().ListLoudnessOfPayload(t.Context(), []byte{0x0a, 0x0b})
	if err != nil || len(rows) != 1 || rows[0].Failure == nil ||
		!strings.Contains(*rows[0].Failure, "refused five times") {
		t.Errorf("file 1's loudness %+v (%v); want its failure and why", rows, err)
	}
}
