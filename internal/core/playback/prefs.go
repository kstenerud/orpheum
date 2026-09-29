// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// FilePrefs are a person's choices for one file (cut-a §5): kept per file, and lasting, so the
// next playback starts with them. A sync offset is in milliseconds of one file; a subtitle choice is
// per file so one broken file can be chosen around (D219).
type FilePrefs struct {
	// SubtitleOffsetMs shows the subtitles this many milliseconds later, or earlier when negative.
	SubtitleOffsetMs int64
	// Subtitle is the file's subtitle choice, or nil when it has none.
	Subtitle *SubtitleChoice
	// IsDialogueBoost is whether the person plays with the dialogue boost: theirs, for every file,
	// off until they turn it on (D83, D222).
	IsDialogueBoost bool
	// IsLevelling is whether the person plays with loudness levelled: theirs, for every file, on
	// until they turn it off (D226 §5).
	IsLevelling bool
}

// SubtitleChoice is a subtitle choice for one file: a track in it, or off (D219).
type SubtitleChoice struct {
	// TrackRef is the chosen track, its stream or a subtitle fetched for it; neither when IsOff.
	TrackRef
	IsOff bool `json:"isOff,omitempty"`
	// IsCarriedFromWork is a choice made in another file of the work and carried here, which a later
	// choice elsewhere carries over; a choice made in this file is its own and is kept.
	IsCarriedFromWork bool `json:"isCarriedFromWork,omitempty"`
	// Track is what the chosen track is. A rescan writes a file's streams again under new ids, so
	// a choice is found again by what its track is, not only by its id (D223 §8).
	Track *TrackDescription `json:"track,omitempty"`
}

// TrackDescription is what a subtitle track is, apart from its stream id: what finds the same
// track again when the file's streams are written anew.
type TrackDescription struct {
	Language          string `json:"language,omitempty"`
	Title             string `json:"title,omitempty"`
	IsForced          bool   `json:"isForced,omitempty"`
	IsHearingImpaired bool   `json:"isHearingImpaired,omitempty"`
}

// describe is what track is, apart from its stream id.
func describe(track SubtitleTrack) *TrackDescription {
	return &TrackDescription{
		Language: track.Language, Title: track.Title, IsForced: track.Forced, IsHearingImpaired: track.HearingImpaired,
	}
}

// trackLike is a track as a description names it, for matchingTrack.
func trackLike(description TrackDescription) SubtitleTrack {
	return SubtitleTrack{
		Language: description.Language, Title: description.Title, Forced: description.IsForced,
		HearingImpaired: description.IsHearingImpaired,
	}
}

// The preference table's names for what FilePrefs holds.
const (
	scopeFile            = "file"
	scopeUser            = "user"
	axisSubtitleOffsetMs = "subtitle_offset_ms"
	axisSubtitleChoice   = "subtitle_choice"
	axisDialogueBoost    = "dialogue_boost"
	axisLevelling        = "levelling"
	// A switch's value is JSON, as value_json keeps it.
	valueTrue  = "true"
	valueFalse = "false"
)

// ErrNoTrack is a subtitle choice naming a track the file does not have.
var ErrNoTrack = errors.New("no such subtitle track in this file")

// FilePrefs are the person's choices for the file, at their starting values where they made
// none.
func (service *Service) FilePrefs(ctx context.Context, userID, fileID int64) (FilePrefs, error) {
	stored, err := service.database.Read().ListPreferencesAtScope(ctx, sqlcgen.ListPreferencesAtScopeParams{
		UserID: userID, Scope: scopeFile, ScopeID: fileID,
	})
	if err != nil {
		return FilePrefs{}, fmt.Errorf("read the preferences for file %d: %w", fileID, err)
	}
	var prefs FilePrefs
	for _, preference := range stored {
		switch preference.Axis {
		case axisSubtitleOffsetMs:
			prefs.SubtitleOffsetMs, err = strconv.ParseInt(preference.ValueJson, 10, 64)
		case axisSubtitleChoice:
			prefs.Subtitle = &SubtitleChoice{}
			err = json.Unmarshal([]byte(preference.ValueJson), prefs.Subtitle)
		}
		if err != nil {
			return FilePrefs{}, fmt.Errorf("read the %s of file %d: %w", preference.Axis, fileID, err)
		}
	}
	personal, err := service.database.Read().ListPreferencesAtScope(ctx, sqlcgen.ListPreferencesAtScopeParams{
		UserID: userID, Scope: scopeUser, ScopeID: 0,
	})
	if err != nil {
		return FilePrefs{}, fmt.Errorf("read user %d's preferences: %w", userID, err)
	}
	if prefs.Subtitle, err = service.currentChoice(ctx, fileID, prefs.Subtitle); err != nil {
		return FilePrefs{}, err
	}
	prefs.IsLevelling = true
	for _, preference := range personal {
		switch preference.Axis {
		case axisDialogueBoost:
			prefs.IsDialogueBoost = preference.ValueJson == valueTrue
		case axisLevelling:
			prefs.IsLevelling = preference.ValueJson != valueFalse
		}
	}
	return prefs, nil
}

// SetDialogueBoost keeps whether the person plays with the dialogue boost, for every file (D83).
func (service *Service) SetDialogueBoost(ctx context.Context, userID int64, isOn bool) error {
	return service.setOwn(ctx, userID, axisDialogueBoost, isOn)
}

// SetLevelling keeps whether the person plays with loudness levelled, for every file (D226 §5).
func (service *Service) SetLevelling(ctx context.Context, userID int64, isOn bool) error {
	return service.setOwn(ctx, userID, axisLevelling, isOn)
}

// setOwn keeps one of the person's own choices that is on or off, for every file.
func (service *Service) setOwn(ctx context.Context, userID int64, axis string, isOn bool) error {
	value := valueFalse
	if isOn {
		value = valueTrue
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.SetPreference(ctx, sqlcgen.SetPreferenceParams{
			UserID: userID, Scope: scopeUser, ScopeID: 0, Axis: axis, ValueJson: value,
			SetAt: service.clock.Now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("keep user %d's %s: %w", userID, axis, err)
	}
	return nil
}

// SetSubtitleOffset keeps the person's subtitle offset for a file they may play.
func (service *Service) SetSubtitleOffset(ctx context.Context, userID, fileID, offsetMs int64) error {
	if _, err := service.file(ctx, fileID); err != nil {
		return err
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		offset := strconv.FormatInt(offsetMs, 10)
		return service.setPreference(ctx, queries, userID, fileID, axisSubtitleOffsetMs, offset)
	})
	if err != nil {
		return fmt.Errorf("keep the subtitle offset for file %d: %w", fileID, err)
	}
	return nil
}

func (service *Service) setPreference(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID int64, axis, value string,
) error {
	return queries.SetPreference(ctx, sqlcgen.SetPreferenceParams{
		UserID: userID, Scope: scopeFile, ScopeID: fileID, Axis: axis, ValueJson: value,
		SetAt: service.clock.Now().UnixMilli(),
	})
}

// ChooseSubtitle keeps a person's subtitle choice in a file as the file's own, and carries it to
// the work's other files that have no choice of their own (D219): each takes its track of the
// same language and kind, preferring the same title, or off; one with no such track is left
// without a choice, since a fallback is not a choice (D24).
func (service *Service) ChooseSubtitle(ctx context.Context, userID, fileID int64, choice SubtitleChoice) error {
	if _, err := service.file(ctx, fileID); err != nil {
		return err
	}
	tracks, err := service.textTracksOf(ctx, fileID)
	if err != nil {
		return err
	}
	var chosen *SubtitleTrack
	if !choice.IsOff {
		chosen = trackByID(tracks, choice.TrackRef)
		if chosen == nil {
			return ErrNoTrack
		}
	}
	siblings, err := service.filesOfWork(ctx, fileID)
	if err != nil {
		return err
	}
	own := SubtitleChoice{TrackRef: choice.TrackRef, IsOff: choice.IsOff}
	if chosen != nil {
		own.Track = describe(*chosen)
	}
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if err := service.keepChoice(ctx, queries, userID, fileID, &own); err != nil {
			return err
		}
		for _, sibling := range siblings {
			if sibling == fileID {
				continue
			}
			if err := service.carryChoice(ctx, queries, userID, sibling, chosen); err != nil {
				return err
			}
		}
		return nil
	})
}

// filesOfWork are the live files of the work a file belongs to (D219), or none when the file is
// not yet a version of a title.
func (service *Service) filesOfWork(ctx context.Context, fileID int64) ([]int64, error) {
	titleID, err := store.OrNotFound(service.database.Read().GetTitleOfFile(ctx, fileID))
	if errors.Is(err, store.ErrNotFound) {
		return []int64{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the title of file %d: %w", fileID, err)
	}
	files, err := service.database.Read().ListFilesOfWork(ctx, titleID)
	if err != nil {
		return nil, fmt.Errorf("list the files of title %d: %w", titleID, err)
	}
	return files, nil
}

// carryChoice gives a file the choice carried from another, unless it has one of its own: the track
// that matches chosen, off when chosen is nil, or no choice when it has no match.
func (service *Service) carryChoice(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID int64, chosen *SubtitleTrack,
) error {
	current, err := service.FilePrefs(ctx, userID, fileID)
	if err != nil {
		return err
	}
	if current.Subtitle != nil && !current.Subtitle.IsCarriedFromWork {
		return nil // the file's own choice stands
	}
	carried := &SubtitleChoice{IsOff: true, IsCarriedFromWork: true}
	if chosen != nil {
		tracks, err := service.textTracksOf(ctx, fileID)
		if err != nil {
			return err
		}
		match := matchingTrack(tracks, *chosen)
		carried = nil
		if match != nil {
			carried = &SubtitleChoice{TrackRef: match.TrackRef, IsCarriedFromWork: true, Track: describe(*match)}
		}
	}
	return service.keepChoice(ctx, queries, userID, fileID, carried)
}

// currentChoice is a stored subtitle choice as the file's tracks are now: its track found again by
// what it is when a rescan gave it a new id, the matching track carried to a file whose tracks
// changed, or no choice when none matches (D223 §8).
func (service *Service) currentChoice(
	ctx context.Context, fileID int64, stored *SubtitleChoice,
) (*SubtitleChoice, error) {
	if stored == nil || stored.IsOff {
		return stored, nil
	}
	tracks, err := service.textTracksOf(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if trackByID(tracks, stored.TrackRef) != nil {
		return stored, nil
	}
	if stored.Track == nil {
		return nil, nil //nolint:nilnil // a choice made before tracks were described names nothing now
	}
	match := matchingTrack(tracks, trackLike(*stored.Track))
	if match == nil {
		return nil, nil //nolint:nilnil // no track of the file is the one chosen
	}
	found := *stored
	found.TrackRef = match.TrackRef
	return &found, nil
}

// keepChoice stores a file's subtitle choice, or removes it when choice is nil.
func (service *Service) keepChoice(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID int64, choice *SubtitleChoice,
) error {
	if choice == nil {
		err := queries.DeletePreference(ctx, sqlcgen.DeletePreferenceParams{
			UserID: userID, Scope: scopeFile, ScopeID: fileID, Axis: axisSubtitleChoice,
		})
		if err != nil {
			return fmt.Errorf("clear the subtitle choice of file %d: %w", fileID, err)
		}
		return nil
	}
	encoded, err := json.Marshal(choice)
	if err != nil {
		return fmt.Errorf("encode a subtitle choice: %w", err)
	}
	if err := service.setPreference(ctx, queries, userID, fileID, axisSubtitleChoice, string(encoded)); err != nil {
		return fmt.Errorf("keep the subtitle choice of file %d: %w", fileID, err)
	}
	return nil
}

func trackByID(tracks []SubtitleTrack, ref TrackRef) *SubtitleTrack {
	for index := range tracks {
		if tracks[index].TrackRef == ref {
			return &tracks[index]
		}
	}
	return nil
}

// matchingTrack is the track of the same language and kind as chosen, the one of the same title
// when there are several, or nil (D219).
func matchingTrack(tracks []SubtitleTrack, chosen SubtitleTrack) *SubtitleTrack {
	var match *SubtitleTrack
	for index := range tracks {
		track := &tracks[index]
		if track.Language != chosen.Language || track.Forced != chosen.Forced ||
			track.HearingImpaired != chosen.HearingImpaired {
			continue
		}
		if track.Title == chosen.Title {
			return track
		}
		if match == nil {
			match = track
		}
	}
	return match
}
