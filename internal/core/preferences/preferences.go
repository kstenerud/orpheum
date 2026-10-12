// SPDX-License-Identifier: AGPL-3.0-or-later

package preferences

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Kind is what sort of track one is, beside its language (D24): a subtitle's forced or for the
// deaf and hard of hearing, an audio track's commentary or description, or plain.
type Kind string

// The kinds.
const (
	KindPlain           Kind = "plain"
	KindForced          Kind = "forced"
	KindHearingImpaired Kind = "hearing_impaired"
	KindCommentary      Kind = "commentary"
	KindDescribed       Kind = "described"
)

// TrackRef names a track of a file: a stream of the file's, or a subtitle fetched for it (D224).
// Exactly one is set.
type TrackRef struct {
	StreamID int64 `json:"streamId,omitempty"`
	// FetchedSubtitleID is a subtitle fetched on request; it is unverified, as every fetched one is in
	// Cut A (D224 §4, D178 §4).
	FetchedSubtitleID int64 `json:"fetchedSubtitleId,omitempty"`
}

// IsFetched is whether the track is a subtitle fetched on request.
func (ref TrackRef) IsFetched() bool { return ref.FetchedSubtitleID != 0 }

// Description is what a track is, apart from which one: what finds its like in another file, or
// the same track again when a rescan writes a file's streams anew under new ids (D223 §8).
type Description struct {
	// Language is BCP 47, or empty when nothing named one.
	Language string `json:"language,omitempty"`
	Kind     Kind   `json:"kind,omitempty"`
	// ContainerTitle is the track's title in its container, empty when it has none.
	ContainerTitle string `json:"containerTitle,omitempty"`
}

// Track is one subtitle or audio track of a file, as a choice sees it.
type Track struct {
	TrackRef
	Description
}

// Tracks are the tracks of one file a choice is worked out against: its text subtitles, its own
// and fetched, and its audio tracks.
type Tracks struct {
	Subtitles []Track
	Audio     []Track
}

// Used is what played in a file: a track, or, for subtitles, none (D230 §2). It keeps what the
// track is, so a rescan's new stream ids find it again.
type Used struct {
	TrackRef
	IsOff bool         `json:"isOff,omitempty"`
	Track *Description `json:"track,omitempty"`
}

// usedTrack is a track as what was used in its file.
func usedTrack(track Track) Used {
	description := track.Description
	return Used{TrackRef: track.TrackRef, Track: &description}
}

// Language is a person's last choice in a work (D230 §3): a language, with the kind and title of
// the track chosen, which a file's tracks are matched against when it plays; or, for subtitles,
// off.
type Language struct {
	IsOff bool `json:"isOff,omitempty"`
	Description
}

// Preferences are the choices a person's playback of one file starts with (cut-a §5): what was
// last used in the file when it was started, or else the person's last choice in its work, each
// worked out against the file's tracks as they are now (D230); the file's subtitle offset; and
// the person's own switches, which hold in every file (D223 §10).
type Preferences struct {
	// SubtitleOffsetMs shows the subtitles this many milliseconds later, or earlier when negative.
	SubtitleOffsetMs int64
	// Subtitle is the subtitle track to start with; nil plays without subtitles.
	Subtitle *TrackRef
	// Audio is the audio track to start with; nil plays the file's main one.
	Audio *TrackRef
	// IsDialogueBoostOn is whether the person plays with the dialogue boost: theirs, for every file,
	// off until they turn it on (D83, D222).
	IsDialogueBoostOn bool
	// IsLevellingOn is whether the person plays with loudness levelled: theirs, for every file, on
	// until they turn it off (D226 §5).
	IsLevellingOn bool
}

// The preference table's names for what is kept.
const (
	scopeFile = "file"
	scopeWork = "work"
	scopeUser = "user"
	// A file's.
	axisSubtitleOffsetMs = "subtitle_offset_ms"
	axisSubtitleUsed     = "subtitle_used"
	axisAudioUsed        = "audio_used"
	// A work's.
	axisSubtitleLanguage = "subtitle_language"
	axisAudioLanguage    = "audio_language"
	// A person's own.
	axisDialogueBoost = "dialogue_boost"
	axisLevelling     = "levelling"
	// A switch's value is JSON, as value_json keeps it.
	valueTrue  = "true"
	valueFalse = "false"
)

// ErrNoTrack is a choice naming a track the file does not have.
var ErrNoTrack = errors.New("no such track in this file")

// Service keeps a person's preferences. What concerns a file is read and written in its caller's
// transaction, which has read the file's tracks, so a choice is never decided on what that
// transaction does not see.
type Service struct {
	database *store.Database
	clock    clock.Clock
}

// NewService returns the preferences service.
func NewService(database *store.Database, clockSource clock.Clock) *Service {
	return &Service{database: database, clock: clockSource}
}

// ForFile are the choices the person's playback of a file starts with, worked out against its
// tracks, read through queries: the reader pool or a write transaction's own.
func (service *Service) ForFile(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID int64, tracks Tracks,
) (Preferences, error) {
	prefs := Preferences{IsLevellingOn: true}
	own, err := queries.ListPreferencesAtScope(ctx, sqlcgen.ListPreferencesAtScopeParams{
		UserID: userID, Scope: scopeFile, ScopeID: fileID,
	})
	if err != nil {
		return Preferences{}, fmt.Errorf("read the preferences for file %d: %w", fileID, err)
	}
	var subtitleUsed, audioUsed *Used
	for _, preference := range own {
		switch preference.Axis {
		case axisSubtitleOffsetMs:
			prefs.SubtitleOffsetMs, err = strconv.ParseInt(preference.ValueJSON, 10, 64)
		case axisSubtitleUsed:
			subtitleUsed, err = decode[Used](preference.ValueJSON)
		case axisAudioUsed:
			audioUsed, err = decode[Used](preference.ValueJSON)
		}
		if err != nil {
			return Preferences{}, fmt.Errorf("read the %s of file %d: %w", preference.Axis, fileID, err)
		}
	}
	subtitleLanguage, audioLanguage, err := service.workLanguages(ctx, queries, userID, fileID)
	if err != nil {
		return Preferences{}, err
	}
	prefs.Subtitle = startingTrack(tracks.Subtitles, subtitleUsed, subtitleLanguage)
	prefs.Audio = startingTrack(tracks.Audio, audioUsed, audioLanguage)
	personal, err := queries.ListPreferencesAtScope(ctx, sqlcgen.ListPreferencesAtScopeParams{
		UserID: userID, Scope: scopeUser, ScopeID: 0,
	})
	if err != nil {
		return Preferences{}, fmt.Errorf("read user %d's preferences: %w", userID, err)
	}
	for _, preference := range personal {
		switch preference.Axis {
		case axisDialogueBoost:
			prefs.IsDialogueBoostOn = preference.ValueJSON == valueTrue
		case axisLevelling:
			prefs.IsLevellingOn = preference.ValueJSON != valueFalse
		}
	}
	return prefs, nil
}

// workLanguages are the person's last subtitle and audio choices in the work a file is of, nil
// where they made none or the file is of no work yet.
func (service *Service) workLanguages(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID int64,
) (subtitle, audio *Language, err error) {
	workID, found, err := workOf(ctx, queries, fileID)
	if err != nil || !found {
		return nil, nil, err
	}
	stored, err := queries.ListPreferencesAtScope(ctx, sqlcgen.ListPreferencesAtScopeParams{
		UserID: userID, Scope: scopeWork, ScopeID: workID,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read the preferences for work %d: %w", workID, err)
	}
	for _, preference := range stored {
		switch preference.Axis {
		case axisSubtitleLanguage:
			subtitle, err = decode[Language](preference.ValueJSON)
		case axisAudioLanguage:
			audio, err = decode[Language](preference.ValueJSON)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read the %s of work %d: %w", preference.Axis, workID, err)
		}
	}
	return subtitle, audio, nil
}

// workOf is the item that names the work a live file is of: its series, or its title (D230).
func workOf(ctx context.Context, queries *sqlcgen.Queries, fileID int64) (int64, bool, error) {
	workID, err := store.OrNotFound(queries.GetWorkOfFile(ctx, fileID))
	if errors.Is(err, store.ErrNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read the work of file %d: %w", fileID, err)
	}
	return workID, true, nil
}

// startingTrack is the track a playback starts with (D230 §2, §3): what was last used in the file,
// found again by what it is when a rescan gave it a new id; else, for a file never started or whose
// used track is gone, the track matching the work's language; nil for none, or off.
func startingTrack(tracks []Track, used *Used, language *Language) *TrackRef {
	if used != nil && used.IsOff {
		return nil
	}
	if track := foundAgain(tracks, used); track != nil {
		return &track.TrackRef
	}
	if language == nil || language.IsOff {
		return nil
	}
	if track := workTrack(tracks, language.Description); track != nil {
		return &track.TrackRef
	}
	return nil
}

// workTrack is the track a work's chosen language finds (D230 §3): one of that language exactly,
// else one of its primary language subtag, so a choice of en-GB finds an en track and a choice of
// en an en-GB one; nil when neither is there.
func workTrack(tracks []Track, chosen Description) *Track {
	if track := matchingTrack(tracks, chosen, isSameTag); track != nil {
		return track
	}
	return matchingTrack(tracks, chosen, isSamePrimaryLanguage)
}

// isSameTag is whether two BCP 47 tags are the same.
func isSameTag(a, b string) bool { return a == b }

// isSamePrimaryLanguage is whether two BCP 47 tags share their primary language subtag: en-GB
// and en, or en-GB and en-US.
func isSamePrimaryLanguage(a, b string) bool {
	primaryA, _, _ := strings.Cut(a, "-")
	primaryB, _, _ := strings.Cut(b, "-")
	return strings.EqualFold(primaryA, primaryB)
}

// foundAgain is the track used names among tracks: by its id, or, when a rescan gave it a new one,
// by what it is; nil when used is nil or off, or its track is gone.
func foundAgain(tracks []Track, used *Used) *Track {
	if used == nil || used.IsOff {
		return nil
	}
	if track := trackByRef(tracks, used.TrackRef); track != nil {
		return track
	}
	if used.Track == nil {
		return nil
	}
	return matchingTrack(tracks, *used.Track, isSameTag)
}

func trackByRef(tracks []Track, ref TrackRef) *Track {
	for index := range tracks {
		if tracks[index].TrackRef == ref {
			return &tracks[index]
		}
	}
	return nil
}

// matchingTrack is the track of the language described, as sameLanguage compares tags,
// preferring the same kind and then the same title (D230 §3), or nil when none is in that
// language: a fallback is not a choice (D24).
func matchingTrack(tracks []Track, described Description, sameLanguage func(a, b string) bool) *Track {
	var best *Track
	bestRank := -1
	for index := range tracks {
		track := &tracks[index]
		if !sameLanguage(track.Language, described.Language) {
			continue
		}
		rank := 0
		if track.Kind == described.Kind {
			rank += 2
		}
		if track.ContainerTitle == described.ContainerTitle {
			rank++
		}
		if rank > bestRank {
			best, bestRank = track, rank
		}
	}
	return best
}

// UsedIn is what a playback that starts with prefs uses: its subtitle track or none, and its audio
// track, the file's main one, mainAudio, when prefs names none; nil when the file has no audio.
func UsedIn(prefs Preferences, tracks Tracks, mainAudio *TrackRef) (subtitle Used, audio *Used) {
	subtitle = Used{IsOff: true}
	if prefs.Subtitle != nil {
		if track := trackByRef(tracks.Subtitles, *prefs.Subtitle); track != nil {
			subtitle = usedTrack(*track)
		}
	}
	played := cmp.Or(prefs.Audio, mainAudio)
	if played == nil {
		return subtitle, nil
	}
	if track := trackByRef(tracks.Audio, *played); track != nil {
		used := usedTrack(*track)
		return subtitle, &used
	}
	return subtitle, nil
}

// decode reads a stored JSON value.
func decode[T any](valueJSON string) (*T, error) {
	var value T
	if err := json.Unmarshal([]byte(valueJSON), &value); err != nil {
		return nil, err
	}
	return &value, nil
}
