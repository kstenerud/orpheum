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

// FilePreferences are a person's choices for one file (cut-a §5): kept per file, and lasting, so the
// next playback starts with them. A sync offset is in milliseconds of one file; a subtitle choice is
// per file so one broken file can be chosen around (D219).
type FilePreferences struct {
	// SubtitleOffsetMs shows the subtitles this many milliseconds later, or earlier when negative.
	SubtitleOffsetMs int64
	// SubtitleChoice is the file's subtitle choice, or nil when it has none.
	SubtitleChoice *SubtitleChoice
	// IsDialogueBoostOn is whether the person plays with the dialogue boost: theirs, for every file,
	// off until they turn it on (D83, D222).
	IsDialogueBoostOn bool
	// IsLevellingOn is whether the person plays with loudness levelled: theirs, for every file, on
	// until they turn it off (D226 §5).
	IsLevellingOn bool
}

// SubtitleChoice is a subtitle choice for one file: a track in it, or off (D219).
type SubtitleChoice struct {
	// TrackRef is the chosen track, its stream or a subtitle fetched for it; neither when IsOff.
	TrackRef
	IsOff bool `json:"isOff,omitempty"`
	// IsCarriedFromWork is a choice made in another file of the work and carried here, which a later
	// choice elsewhere carries over; a choice made in this file is its own and is kept.
	IsCarriedFromWork bool `json:"isCarriedFromWork,omitempty"`
	// TrackDescription is what the chosen track is. A rescan writes a file's streams again under new ids, so
	// a choice is found again by what its track is, not only by its id (D223 §8).
	TrackDescription *TrackDescription `json:"track,omitempty"`
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
		Language: track.Language, Title: track.Title, IsForced: track.IsForced,
		IsHearingImpaired: track.IsHearingImpaired,
	}
}

// trackLike is a track as a description names it, for matchingTrack.
func trackLike(description TrackDescription) SubtitleTrack {
	return SubtitleTrack{
		Language: description.Language, Title: description.Title, IsForced: description.IsForced,
		IsHearingImpaired: description.IsHearingImpaired,
	}
}

// The preference table's names for what FilePreferences holds.
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

// FilePreferences are the person's choices for the file, at their starting values where they made
// none.
func (service *Service) FilePreferences(ctx context.Context, userID, fileID int64) (FilePreferences, error) {
	return service.filePreferences(ctx, service.database.Read(), userID, fileID)
}

// filePreferences are FilePreferences as queries read them, the reader pool or a write transaction's own.
func (service *Service) filePreferences(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID int64,
) (FilePreferences, error) {
	stored, err := queries.ListPreferencesAtScope(ctx, sqlcgen.ListPreferencesAtScopeParams{
		UserID: userID, Scope: scopeFile, ScopeID: fileID,
	})
	if err != nil {
		return FilePreferences{}, fmt.Errorf("read the preferences for file %d: %w", fileID, err)
	}
	var prefs FilePreferences
	for _, preference := range stored {
		switch preference.Axis {
		case axisSubtitleOffsetMs:
			prefs.SubtitleOffsetMs, err = strconv.ParseInt(preference.ValueJson, 10, 64)
		case axisSubtitleChoice:
			prefs.SubtitleChoice = &SubtitleChoice{}
			err = json.Unmarshal([]byte(preference.ValueJson), prefs.SubtitleChoice)
		}
		if err != nil {
			return FilePreferences{}, fmt.Errorf("read the %s of file %d: %w", preference.Axis, fileID, err)
		}
	}
	personal, err := queries.ListPreferencesAtScope(ctx, sqlcgen.ListPreferencesAtScopeParams{
		UserID: userID, Scope: scopeUser, ScopeID: 0,
	})
	if err != nil {
		return FilePreferences{}, fmt.Errorf("read user %d's preferences: %w", userID, err)
	}
	if prefs.SubtitleChoice, err = service.currentChoice(ctx, queries, fileID, prefs.SubtitleChoice); err != nil {
		return FilePreferences{}, err
	}
	prefs.IsLevellingOn = true
	for _, preference := range personal {
		switch preference.Axis {
		case axisDialogueBoost:
			prefs.IsDialogueBoostOn = preference.ValueJson == valueTrue
		case axisLevelling:
			prefs.IsLevellingOn = preference.ValueJson != valueFalse
		}
	}
	return prefs, nil
}

// SetDialogueBoost keeps whether the person plays with the dialogue boost, for every file (D83).
func (service *Service) SetDialogueBoost(ctx context.Context, userID int64, isOn bool) error {
	return service.setPersonalSwitch(ctx, userID, axisDialogueBoost, isOn)
}

// SetLevelling keeps whether the person plays with loudness levelled, for every file (D226 §5).
func (service *Service) SetLevelling(ctx context.Context, userID int64, isOn bool) error {
	return service.setPersonalSwitch(ctx, userID, axisLevelling, isOn)
}

// setPersonalSwitch keeps one of the person's own choices that is on or off, for every file.
func (service *Service) setPersonalSwitch(ctx context.Context, userID int64, axis string, isOn bool) error {
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
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return service.keepSubtitleOffset(ctx, queries, userID, fileID, offsetMs)
	})
	if err != nil {
		return fmt.Errorf("keep the subtitle offset for file %d: %w", fileID, err)
	}
	return nil
}

// keepSubtitleOffset keeps a subtitle offset for a file that queries, the transaction that keeps
// it, finds playable: a file a rescan removed meanwhile gets none.
func (service *Service) keepSubtitleOffset(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID, offsetMs int64,
) error {
	if _, err := service.playableFile(ctx, queries, fileID); err != nil {
		return err
	}
	offset := strconv.FormatInt(offsetMs, 10)
	return service.setFilePreference(ctx, queries, userID, fileID, axisSubtitleOffsetMs, offset)
}

func (service *Service) setFilePreference(
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
// without a choice, since a fallback is not a choice (D24). What it reads, the file, its tracks,
// the work's files and their choices, it reads in the transaction that keeps the choice, so a
// rescan or another choice committed meanwhile cannot leave it deciding on what is gone.
func (service *Service) ChooseSubtitle(ctx context.Context, userID, fileID int64, choice SubtitleChoice) error {
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if _, err := service.playableFile(ctx, queries, fileID); err != nil {
			return err
		}
		own, chosen, err := service.ownChoice(ctx, queries, fileID, choice)
		if err != nil {
			return err
		}
		if keepErr := service.keepChoice(ctx, queries, userID, fileID, &own); keepErr != nil {
			return keepErr
		}
		siblings, err := service.filesOfWork(ctx, queries, fileID)
		if err != nil {
			return err
		}
		for _, sibling := range siblings {
			if sibling == fileID {
				continue
			}
			if carryErr := service.carryChoice(ctx, queries, userID, sibling, chosen); carryErr != nil {
				return carryErr
			}
		}
		return nil
	})
}

// ownChoice is a choice made in a file as the file keeps it, with what its track is, and the
// chosen track, nil for off; ErrNoTrack when the file has no such track.
func (service *Service) ownChoice(
	ctx context.Context, queries *sqlcgen.Queries, fileID int64, choice SubtitleChoice,
) (SubtitleChoice, *SubtitleTrack, error) {
	own := SubtitleChoice{TrackRef: choice.TrackRef, IsOff: choice.IsOff}
	if choice.IsOff {
		return own, nil, nil
	}
	tracks, err := service.textTracksOf(ctx, queries, fileID)
	if err != nil {
		return own, nil, err
	}
	chosen := trackByID(tracks, choice.TrackRef)
	if chosen == nil {
		return own, nil, ErrNoTrack
	}
	own.TrackDescription = describe(*chosen)
	return own, chosen, nil
}

// filesOfWork are the live files of the work a file belongs to (D219), or none when the file is
// not yet a version of a title.
func (service *Service) filesOfWork(ctx context.Context, queries *sqlcgen.Queries, fileID int64) ([]int64, error) {
	titleID, err := store.OrNotFound(queries.GetTitleOfFile(ctx, fileID))
	if errors.Is(err, store.ErrNotFound) {
		return []int64{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the title of file %d: %w", fileID, err)
	}
	files, err := queries.ListFilesOfWork(ctx, titleID)
	if err != nil {
		return nil, fmt.Errorf("list the files of title %d: %w", titleID, err)
	}
	return files, nil
}

// carryChoice gives a file the choice carried from another, unless it has one of its own: the track
// that matches chosen, off when chosen is nil, or no choice when it has no match. It reads the
// file's choice and tracks through queries, the transaction that keeps the carried choice.
func (service *Service) carryChoice(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID int64, chosen *SubtitleTrack,
) error {
	current, err := service.filePreferences(ctx, queries, userID, fileID)
	if err != nil {
		return err
	}
	if current.SubtitleChoice != nil && !current.SubtitleChoice.IsCarriedFromWork {
		return nil // the file's own choice stands
	}
	carried := &SubtitleChoice{IsOff: true, IsCarriedFromWork: true}
	if chosen != nil {
		tracks, err := service.textTracksOf(ctx, queries, fileID)
		if err != nil {
			return err
		}
		match := matchingTrack(tracks, *chosen)
		carried = nil
		if match != nil {
			carried = &SubtitleChoice{
				TrackRef: match.TrackRef, IsCarriedFromWork: true, TrackDescription: describe(*match),
			}
		}
	}
	return service.keepChoice(ctx, queries, userID, fileID, carried)
}

// currentChoice is a stored subtitle choice as the file's tracks are now: its track found again by
// what it is when a rescan gave it a new id, the matching track carried to a file whose tracks
// changed, or no choice when none matches (D223 §8).
func (service *Service) currentChoice(
	ctx context.Context, queries *sqlcgen.Queries, fileID int64, stored *SubtitleChoice,
) (*SubtitleChoice, error) {
	if stored == nil || stored.IsOff {
		return stored, nil
	}
	tracks, err := service.textTracksOf(ctx, queries, fileID)
	if err != nil {
		return nil, err
	}
	if trackByID(tracks, stored.TrackRef) != nil {
		return stored, nil
	}
	if stored.TrackDescription == nil {
		return nil, nil //nolint:nilnil // a choice made before tracks were described names nothing now
	}
	match := matchingTrack(tracks, trackLike(*stored.TrackDescription))
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
	if err := service.setFilePreference(ctx, queries, userID, fileID, axisSubtitleChoice, string(encoded)); err != nil {
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
		if track.Language != chosen.Language || track.IsForced != chosen.IsForced ||
			track.IsHearingImpaired != chosen.IsHearingImpaired {
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
