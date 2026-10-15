// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"net/http"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

func TestLoudness_AFileIsMeasuredOnceAndPlaysAtTheLevel(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	if queued, err := harness.readThrough.QueueReadThroughs(t.Context()); err != nil || queued != 2 {
		t.Fatalf("the sweep queued %d, %v; want both films with sound, not the skipped file (D226 §2)", queued, err)
	}
	unmeasured := harness.play(t, user, chrome(1))
	if unmeasured.Levelling.IsMeasured {
		t.Errorf("before the measurement: %+v, want it played as it is (D226 §6)", unmeasured.Levelling)
	}

	// Wing's ten minutes: quiet, with room below its peak for the whole gain. The node is told how
	// long the film is, so its watchdog allows decoding all of it.
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE file_probe SET probe_json = json_set(probe_json, '$.format.durationMs', 600000),`+
			` duration_ms = 600000 WHERE file_id = 1`)
	alien := nodeproto.Measurement{IntegratedLUFS: -35.7, RangeLU: 22.8, TruePeakDBTP: new(-16.2)}
	command, err := harness.readThroughWith(t, 90, 1, nodeproto.ReadThroughResult{Loudness: []nodeproto.TrackLoudness{
		{StreamIndex: 0, Stereo: alien},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if command == nil || command.Video.PayloadHash != "0a0b" || string(command.Video.Path) != "Film (2001).mp4" ||
		!slices.Equal(command.LoudnessTracks, []nodeproto.LoudnessTrack{{StreamIndex: 0, ChannelCount: 2}}) ||
		command.DurationMs != 600_000 {
		t.Errorf("the node was asked to read through %+v, want the MP4 by root, path and hash, with its stereo track "+
			"and its ten minutes", command)
	}
	if ids := harness.filesToReadThrough(t); !slices.Equal(ids, []int64{2}) {
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
	_, err := harness.readThroughWith(t, 91, 2, nodeproto.ReadThroughResult{Loudness: []nodeproto.TrackLoudness{{
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
	prepared := harness.handle(t, harness.renditions.HandlePrepare,
		jobs.Job{ID: 92, Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: 1}})
	command := harness.nextCommand(t)
	if command.Prepare == nil || len(command.Prepare.AudioTracks) != 1 ||
		command.Prepare.AudioTracks[0].CarriedGainDB != 5.0 {
		t.Fatalf("the prepare sent %+v, want the track's gain in its mixes (D226 §4)", command.Prepare)
	}
	done := nodeproto.PrepareReport{State: nodeproto.PrepareDone, SizeBytes: 1}
	harness.answerPrepare(t, 92, done)
	if err := testutil.Receive(t, prepared, "the prepare job to end"); err != nil {
		t.Fatal(err)
	}
	again := harness.play(t, user, chrome(2))
	if again.Audio[0].CarriedGainDB != 5.0 || again.Audio[1].CarriedGainDB != 5.0 {
		t.Errorf("once prepared: %+v; want each mix to say it carries the gain, so the player adds none", again.Audio)
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
