// SPDX-License-Identifier: AGPL-3.0-or-later

package preferences

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ChooseSubtitle keeps a person's subtitle choice in a file, a track of tracks or off: as what was
// last used in the file, and its language, kind and title, or off, as their last choice in the
// file's work, which its unwatched files follow (D230). It writes through queries, the transaction
// that read tracks. ErrNoTrack is a track the file does not have.
func (service *Service) ChooseSubtitle(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID int64, choice Used, tracks []Track,
) error {
	kept := keptChoice{
		usedAxis: axisSubtitleUsed, used: Used{IsOff: true},
		languageAxis: axisSubtitleLanguage, language: Language{IsOff: true},
	}
	if !choice.IsOff {
		track := trackByRef(tracks, choice.TrackRef)
		if track == nil {
			return ErrNoTrack
		}
		kept.used, kept.language = usedTrack(*track), Language{Description: track.Description}
	}
	return service.choose(ctx, queries, userID, fileID, kept)
}

// ChooseAudio keeps a person's choice of an audio track of tracks in a file, as ChooseSubtitle
// keeps a subtitle choice (D230 §5).
func (service *Service) ChooseAudio(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID int64, ref TrackRef, tracks []Track,
) error {
	track := trackByRef(tracks, ref)
	if track == nil {
		return ErrNoTrack
	}
	return service.choose(ctx, queries, userID, fileID, keptChoice{
		usedAxis: axisAudioUsed, used: usedTrack(*track),
		languageAxis: axisAudioLanguage, language: Language{Description: track.Description},
	})
}

// keptChoice is a choice as it is kept: what was used in the file, and the work's language, each
// at its axis.
type keptChoice struct {
	usedAxis     string
	used         Used
	languageAxis string
	language     Language
}

// choose keeps a choice as what was used in the file, and as the work's language.
func (service *Service) choose(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID int64, kept keptChoice,
) error {
	inFile := place{userID: userID, scope: scopeFile, scopeID: fileID, axis: kept.usedAxis}
	if err := service.set(ctx, queries, replacing, inFile, kept.used); err != nil {
		return err
	}
	workID, found, err := workOf(ctx, queries, fileID)
	if err != nil || !found {
		return err
	}
	inWork := place{userID: userID, scope: scopeWork, scopeID: workID, axis: kept.languageAxis}
	return service.set(ctx, queries, replacing, inWork, kept.language)
}

// KeepUsed keeps what a playback of a file used, once the person has started it, where the file
// keeps nothing yet: a started file counts as watched, and plays again with what was last used in
// it (D230 §2), while a choice made in it is what it keeps. The audio is nil when the file has
// none.
func (service *Service) KeepUsed(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID int64, subtitle Used, audio *Used,
) error {
	inFile := place{userID: userID, scope: scopeFile, scopeID: fileID, axis: axisSubtitleUsed}
	if err := service.set(ctx, queries, unlessKept, inFile, subtitle); err != nil {
		return err
	}
	if audio == nil {
		return nil
	}
	inFile.axis = axisAudioUsed
	return service.set(ctx, queries, unlessKept, inFile, *audio)
}

// KeepSubtitleOffset keeps the person's subtitle offset for a file, through queries.
func (service *Service) KeepSubtitleOffset(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID, offsetMs int64,
) error {
	inFile := place{userID: userID, scope: scopeFile, scopeID: fileID, axis: axisSubtitleOffsetMs}
	return service.setRaw(ctx, queries, replacing, inFile, strconv.FormatInt(offsetMs, 10))
}

// SetDialogueBoost keeps whether the person plays with the dialogue boost, for every file (D83).
func (service *Service) SetDialogueBoost(ctx context.Context, userID int64, isOn bool) error {
	return service.setPersonalSwitch(ctx, userID, axisDialogueBoost, isOn)
}

// SetLevelling keeps whether the person plays with loudness levelled, for every file (D226 §5).
func (service *Service) SetLevelling(ctx context.Context, userID int64, isOn bool) error {
	return service.setPersonalSwitch(ctx, userID, axisLevelling, isOn)
}

// SetAutoplay keeps whether the next episode plays by itself as one ends, for the person
// (plans/up-next.md).
func (service *Service) SetAutoplay(ctx context.Context, userID int64, isOn bool) error {
	return service.setPersonalSwitch(ctx, userID, axisAutoplay, isOn)
}

// SetDebugTools keeps whether the person's pages show the tools marked as debug tools, on every
// device they sign in on (the owner, 2026-10-29).
func (service *Service) SetDebugTools(ctx context.Context, userID int64, isShown bool) error {
	return service.setPersonalSwitch(ctx, userID, axisDebugTools, isShown)
}

// setPersonalSwitch keeps one of the person's own choices that is on or off, for every file.
func (service *Service) setPersonalSwitch(ctx context.Context, userID int64, axis string, isOn bool) error {
	value := valueFalse
	if isOn {
		value = valueTrue
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return service.setRaw(ctx, queries, replacing, place{userID: userID, scope: scopeUser, axis: axis}, value)
	})
	if err != nil {
		return fmt.Errorf("keep user %d's %s: %w", userID, axis, err)
	}
	return nil
}

// ChooseEpisodeOrdering keeps the ordering a person watches a series in, by its name (D159 §1): a
// choice of theirs on the series' page, never one inferred from what they watch (D159 §4). The
// caller has found the series has it; one it no longer has is read as no choice.
func (service *Service) ChooseEpisodeOrdering(ctx context.Context, userID, seriesID int64, ordering string) error {
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		inSeries := place{userID: userID, scope: scopeWork, scopeID: seriesID, axis: AxisEpisodeOrdering}
		return service.set(ctx, queries, replacing, inSeries, ordering)
	})
	if err != nil {
		return fmt.Errorf("keep the ordering user %d watches series %d in: %w", userID, seriesID, err)
	}
	return nil
}

// writeMode is whether a preference replaces one already kept at its place.
type writeMode bool

// The write modes.
const (
	replacing  writeMode = true
	unlessKept writeMode = false
)

// place is where a preference is kept: a person's axis at a scope.
type place struct {
	userID  int64
	scope   string
	scopeID int64
	axis    string
}

// set keeps value, as JSON, at a place.
func (service *Service) set(
	ctx context.Context, queries *sqlcgen.Queries, mode writeMode, where place, value any,
) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode a %s: %w", where.axis, err)
	}
	return service.setRaw(ctx, queries, mode, where, string(encoded))
}

func (service *Service) setRaw(
	ctx context.Context, queries *sqlcgen.Queries, mode writeMode, where place, valueJSON string,
) error {
	setAt := service.clock.Now().UnixMilli()
	var err error
	if mode == replacing {
		err = queries.SetPreference(ctx, sqlcgen.SetPreferenceParams{
			UserID: where.userID, Scope: where.scope, ScopeID: where.scopeID, Axis: where.axis,
			ValueJSON: valueJSON, SetAt: setAt,
		})
	} else {
		err = queries.KeepPreferenceUnlessSet(ctx, sqlcgen.KeepPreferenceUnlessSetParams{
			UserID: where.userID, Scope: where.scope, ScopeID: where.scopeID, Axis: where.axis,
			ValueJSON: valueJSON, SetAt: setAt,
		})
	}
	if err != nil {
		return fmt.Errorf("keep the %s of %s %d: %w", where.axis, where.scope, where.scopeID, err)
	}
	return nil
}
