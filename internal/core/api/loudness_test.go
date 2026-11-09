// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
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

	// Arkon's ten minutes: quiet, with room below its peak for the whole gain. The node is told how
	// long the film is, so its watchdog allows decoding all of it.
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE file_probe SET probe_json = json_set(probe_json, '$.format.durationMs', 600000),`+
			` duration_ms = 600000 WHERE file_id = 1`)
	arkon := nodeproto.Measurement{IntegratedLUFS: -35.7, RangeLU: 22.8, TruePeakDBTP: new(-16.2)}
	command, err := harness.readThroughWith(t, 90, 1, nodeproto.ReadThroughResult{Loudness: []nodeproto.TrackLoudness{
		{StreamIndex: 0, Stereo: arkon},
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
	if direct.Delivery != "direct" || direct.Levelling != (api.LevellingBody{IsMeasured: true, GainDB: 12.7}) {
		t.Errorf("played directly: %s, %+v; want raised 12.7 dB to -23 LUFS (D226 §3)",
			direct.Delivery, direct.Levelling)
	}
	wantTheRaiseTheServersOnALateDevice(t, harness, user)
}

// wantTheRaiseTheServersOnALateDevice checks that user's film 1, measured to be raised, plays on its
// ladder for a device whose Web Audio is late, and, once it cannot be cut, as it is (D436).
func wantTheRaiseTheServersOnALateDevice(t *testing.T, harness testHarness, user string) {
	t.Helper()
	// On a device whose Web Audio is late, the raise is the server's, on the film's ladder (D436).
	late := chrome(1)
	late["isWebAudioLate"] = true
	raised := harness.play(t, user, late)
	if raised.Delivery != "hls" || !slices.Equal(raised.NotDirectReasons, []string{"levellingRaise"}) {
		t.Errorf("played on a late device: %s, %v; want its ladder, for the raise (D436)",
			raised.Delivery, raised.NotDirectReasons)
	}
	// A film with no ladder to take the raise, its length unknown so it cannot be cut, plays as it is,
	// unraised, rather than refused as one that cannot be converted (D436 §3).
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE file_probe SET probe_json = json_remove(probe_json, '$.format.durationMs'), duration_ms = 0 `+
			`WHERE file_id = 1`)
	if uncut := harness.play(t, user, late); uncut.Delivery != "direct" || len(uncut.NotDirectReasons) != 0 {
		t.Errorf("a film that cannot be cut, on a late device: %s, %v; want it played as it is",
			uncut.Delivery, uncut.NotDirectReasons)
	}
}

// A film converted is levelled as its track's stereo mix, the gain capped by the mix's peak, and its
// sound's chunks carry the gain, which the player does not add again (D226 §4, D344 §1).
func TestLoudness_AConvertedFilmIsLevelledAsItsStereoMixAndItsChunksCarryTheGain(t *testing.T) {
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
	if decided.Delivery != "hls" || decided.OnLadder == nil || decided.OnLadder.SoundCarriedGainDB != 5.0 {
		t.Fatalf("converted: %+v; want it on its ladder, its sound's chunks carrying the stereo mix's gain, "+
			"capped by its peak", decided)
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
		want       api.LevellingBody
	}{
		{"a loud film", -18.8, new(0.4), api.LevellingBody{IsMeasured: true, GainDB: -4.2}},
		{"a quiet film with high peaks", -30.0, new(-3.0), api.LevellingBody{IsMeasured: true, GainDB: 2.0}},
		{"a near-silent one", -60.0, new(-50.0), api.LevellingBody{IsMeasured: true, GainDB: 20.0}},
		{"silence", -70.0, nil, api.LevellingBody{IsMeasured: true, GainDB: 0}},
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
	harness.setSetting(t, "loudness_target_lufs", "-16")
	harness.setSetting(t, "loudness_max_raise_db", "10")
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
