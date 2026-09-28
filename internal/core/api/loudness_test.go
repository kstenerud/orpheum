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
)

// withSound gives the shelved films the audio streams a scan records: the MP4's stereo track and
// the MKV's eight-channel one.
func (harness testHarness) withSound(t *testing.T) {
	t.Helper()
	storetest.ExecRaw(t, harness.database,
		`INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, channels)
		 VALUES (1, 1, 0, 'audio', 'aac', 2), (2, 2, 1, 'audio', 'truehd', 8)`)
}

// measureWith runs a measurement job for fileID, answers the node's command with answer, and
// returns the command and how the job ended.
func (harness testHarness) measureWith(
	t *testing.T, jobID, fileID int64, answer nodeproto.Loudness,
) (*nodeproto.MeasureLoudness, error) {
	t.Helper()
	service := harness.server.dependencies.Load().Playback
	finished := make(chan error, 1)
	go func() {
		job := jobs.Job{ID: jobID, Kind: jobs.KindMeasureLoudness, Subject: fileID}
		finished <- service.HandleLoudness(t.Context(), job)
	}()
	command := <-harness.nodes.commanded
	answer.Job = jobID
	service.Measured(t.Context(), 1, answer)
	return command.MeasureLoudness, <-finished
}

// toMeasure is what a sweep would find to measure now.
func (harness testHarness) toMeasure(t *testing.T) []int64 {
	t.Helper()
	ids, err := harness.library.Read().ListFilesToMeasure(t.Context(),
		sqlcgen.ListFilesToMeasureParams{After: 0, Through: 1000})
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
	harness.withSound(t)
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
	command, err := harness.measureWith(t, 90, 1, nodeproto.Loudness{Tracks: []nodeproto.TrackLoudness{
		{StreamIndex: 0, Stereo: alien},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if command == nil || command.Video.Hash != "0a0b" || string(command.Video.Path) != "Film (2001).mp4" ||
		!slices.Equal(command.Audio, []nodeproto.LoudnessTrack{{StreamIndex: 0, Channels: 2}}) {
		t.Errorf("the node was asked to measure %+v, want the MP4 by root, path and hash, with its stereo track",
			command)
	}
	if ids := harness.toMeasure(t); !slices.Equal(ids, []int64{2}) {
		t.Errorf("after measuring film 1, a sweep finds %v, want only film 2", ids)
	}
	direct := harness.play(t, user, chrome(1))
	if direct.Kind != "direct" || direct.Levelling != (LevellingBody{IsMeasured: true, GainDB: 12.7}) {
		t.Errorf("played directly: %s, %+v; want raised 12.7 dB to -23 LUFS (D226 §3)", direct.Kind, direct.Levelling)
	}
}

func TestLoudness_APreparedFilmIsLevelledAsItsStereoMixAndItsCopyCarriesTheGain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.withSound(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	// The 7.1 track is louder as it is than as its renormalised stereo mix; the mix's gain is
	// capped by its peak.
	_, err := harness.measureWith(t, 91, 2, nodeproto.Loudness{Tracks: []nodeproto.TrackLoudness{{
		StreamIndex: 1,
		Source:      &nodeproto.Measurement{IntegratedLUFS: -20.0, RangeLU: 10, TruePeakDBTP: new(-1.0)},
		Stereo:      nodeproto.Measurement{IntegratedLUFS: -31.0, RangeLU: 9, TruePeakDBTP: new(-6.0)},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	decided := harness.play(t, user, chrome(2))
	want := LevellingBody{IsMeasured: true, GainDB: 5.0}
	if decided.Kind != "hls" || len(decided.Audio) != 2 || decided.Audio[0].Levelling != want ||
		decided.Audio[1].Levelling != want || decided.Audio[0].CarriedGainDB != 0 || decided.Levelling != want {
		t.Fatalf("prepared: %+v; want both mixes at the stereo mix's gain, capped by its peak, and none carried yet",
			decided)
	}
	prepared := make(chan error, 1)
	go func() {
		prepared <- harness.server.dependencies.Load().Playback.HandlePrepare(t.Context(),
			jobs.Job{ID: 92, Kind: jobs.KindPrepare, Subject: 1})
	}()
	command := <-harness.nodes.commanded
	if command.Prepare == nil || len(command.Prepare.Audio) != 1 || command.Prepare.Audio[0].GainDB != 5.0 {
		t.Fatalf("the prepare sent %+v, want the track's gain in its mixes (D226 §4)", command.Prepare)
	}
	service := harness.server.dependencies.Load().Playback
	service.Reported(t.Context(), 1, nodeproto.PrepareReport{Job: 92, State: nodeproto.PrepareDone, SizeBytes: 1})
	if err := <-prepared; err != nil {
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
	harness.withSound(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	_, err := harness.measureWith(t, 93, 2, nodeproto.Loudness{Refused: "the file at this path is not the one named"})
	if err == nil || !strings.Contains(err.Error(), "not the one named") {
		t.Errorf("a measurement the node would not try ended %v, want its reason", err)
	}
	if ids := harness.toMeasure(t); !slices.Equal(ids, []int64{1, 2}) {
		t.Errorf("after the node would not try film 2, a sweep finds %v, want it asked again", ids)
	}
	_, err = harness.measureWith(t, 94, 2, nodeproto.Loudness{Failed: "ffmpeg: Invalid data found"})
	if err == nil || !strings.Contains(err.Error(), "Invalid data found") {
		t.Errorf("a failed measurement ended %v, want FFmpeg's words", err)
	}
	if ids := harness.toMeasure(t); !slices.Equal(ids, []int64{1}) {
		t.Errorf("after FFmpeg could not measure film 2, a sweep finds %v, want only film 1", ids)
	}
}

func TestLoudness_LevellingIsOnUntilThePersonTurnsItOff(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	if first := harness.play(t, user, chrome(1)); !first.Prefs.IsLevelling {
		t.Errorf("a person who never chose: levelling %t, want on (D226 §5)", first.Prefs.IsLevelling)
	}
	answer := harness.do(t, http.MethodPut, "/me/prefs", user, map[string]any{"isLevelling": false})
	if answer.status != http.StatusNoContent {
		t.Fatalf("PUT /me/prefs = %d %s", answer.status, answer.body)
	}
	if off := harness.play(t, user, chrome(2)); off.Prefs.IsLevelling {
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
		storetest.ExecRaw(t, harness.database, "DELETE FROM loudness")
		integrated := check.integrated
		err := harness.library.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			return queries.UpsertLoudness(t.Context(), sqlcgen.UpsertLoudnessParams{
				PayloadHash: []byte{0x0a, 0x0b}, StreamIndex: 0, Sound: "stereo", IntegratedLufs: &integrated,
				TruePeakDbtp: check.peak, MeasuredAt: 1,
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
