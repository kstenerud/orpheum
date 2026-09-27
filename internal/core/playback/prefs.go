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
// next playback starts with them. A sync offset is in seconds of one file; a subtitle choice is
// per file so one broken file can be chosen around (D219).
type FilePrefs struct {
	// SubtitleOffsetMs shows the subtitles this many milliseconds later, or earlier when negative.
	SubtitleOffsetMs int64
	// Subtitle is the file's subtitle choice, or nil when it has none.
	Subtitle *SubtitleChoice
	// IsDialogueBoost is whether the person plays with the dialogue boost: theirs, for every file,
	// off until they turn it on (D83, D222).
	IsDialogueBoost bool
}

// SubtitleChoice is a subtitle choice for one file: a track in it, or off (D219).
type SubtitleChoice struct {
	// StreamID is the chosen track; 0 when IsOff.
	StreamID int64 `json:"streamId,omitempty"`
	IsOff    bool  `json:"isOff,omitempty"`
	// IsCarried is a choice made in another file of the work and carried here, which a later
	// choice elsewhere carries over; a choice made in this file is its own and is kept.
	IsCarried bool `json:"isCarried,omitempty"`
}

// The preference table's names for what FilePrefs holds.
const (
	scopeFile            = "file"
	scopeUser            = "user"
	axisSubtitleOffsetMs = "subtitle_offset_ms"
	axisSubtitleTrack    = "subtitle_track"
	axisDialogueBoost    = "dialogue_boost"
	valueOn              = "on"
	valueOff             = "off"
)

// ErrNoTrack is a subtitle choice naming a track the file does not have.
var ErrNoTrack = errors.New("no such subtitle track in this file")

// FilePrefs are the person's choices for the file, at their starting values where they made
// none.
func (service *Service) FilePrefs(ctx context.Context, userID, fileID int64) (FilePrefs, error) {
	stored, err := service.database.Read().ListPreferencesAt(ctx, sqlcgen.ListPreferencesAtParams{
		UserID: userID, Scope: scopeFile, ScopeID: fileID,
	})
	if err != nil {
		return FilePrefs{}, fmt.Errorf("read the preferences for file %d: %w", fileID, err)
	}
	var prefs FilePrefs
	for _, preference := range stored {
		switch preference.Axis {
		case axisSubtitleOffsetMs:
			prefs.SubtitleOffsetMs, err = strconv.ParseInt(preference.Value, 10, 64)
		case axisSubtitleTrack:
			prefs.Subtitle = &SubtitleChoice{}
			err = json.Unmarshal([]byte(preference.Value), prefs.Subtitle)
		}
		if err != nil {
			return FilePrefs{}, fmt.Errorf("read the %s of file %d: %w", preference.Axis, fileID, err)
		}
	}
	personal, err := service.database.Read().ListPreferencesAt(ctx, sqlcgen.ListPreferencesAtParams{
		UserID: userID, Scope: scopeUser, ScopeID: 0,
	})
	if err != nil {
		return FilePrefs{}, fmt.Errorf("read user %d's preferences: %w", userID, err)
	}
	for _, preference := range personal {
		if preference.Axis == axisDialogueBoost {
			prefs.IsDialogueBoost = preference.Value == valueOn
		}
	}
	return prefs, nil
}

// SetDialogueBoost keeps whether the person plays with the dialogue boost, for every file (D83).
func (service *Service) SetDialogueBoost(ctx context.Context, userID int64, isOn bool) error {
	value := valueOff
	if isOn {
		value = valueOn
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.SetPreference(ctx, sqlcgen.SetPreferenceParams{
			UserID: userID, Scope: scopeUser, ScopeID: 0, Axis: axisDialogueBoost, Value: value,
			SetAt: service.clock.Now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("keep user %d's dialogue boost: %w", userID, err)
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
		UserID: userID, Scope: scopeFile, ScopeID: fileID, Axis: axis, Value: value,
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
		chosen = trackByID(tracks, choice.StreamID)
		if chosen == nil {
			return ErrNoTrack
		}
	}
	titleID, err := store.One(service.database.Read().GetTitleOfFile(ctx, fileID))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("read the title of file %d: %w", fileID, err)
	}
	siblings := []int64{}
	if err == nil {
		if siblings, err = service.database.Read().ListFilesOfWork(ctx, titleID); err != nil {
			return fmt.Errorf("list the files of title %d: %w", titleID, err)
		}
	}
	own := SubtitleChoice{StreamID: choice.StreamID, IsOff: choice.IsOff}
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

// carryChoice gives a file the choice carried from another, unless it has one of its own: the track
// that matches chosen, off when chosen is nil, or no choice when it has no match.
func (service *Service) carryChoice(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID int64, chosen *SubtitleTrack,
) error {
	current, err := service.FilePrefs(ctx, userID, fileID)
	if err != nil {
		return err
	}
	if current.Subtitle != nil && !current.Subtitle.IsCarried {
		return nil // the file's own choice stands
	}
	carried := &SubtitleChoice{IsOff: true, IsCarried: true}
	if chosen != nil {
		tracks, err := service.textTracksOf(ctx, fileID)
		if err != nil {
			return err
		}
		match := matchingTrack(tracks, *chosen)
		carried = nil
		if match != nil {
			carried = &SubtitleChoice{StreamID: match.StreamID, IsCarried: true}
		}
	}
	return service.keepChoice(ctx, queries, userID, fileID, carried)
}

// keepChoice stores a file's subtitle choice, or removes it when choice is nil.
func (service *Service) keepChoice(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID int64, choice *SubtitleChoice,
) error {
	if choice == nil {
		err := queries.DeletePreference(ctx, sqlcgen.DeletePreferenceParams{
			UserID: userID, Scope: scopeFile, ScopeID: fileID, Axis: axisSubtitleTrack,
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
	if err := service.setPreference(ctx, queries, userID, fileID, axisSubtitleTrack, string(encoded)); err != nil {
		return fmt.Errorf("keep the subtitle choice of file %d: %w", fileID, err)
	}
	return nil
}

func trackByID(tracks []SubtitleTrack, streamID int64) *SubtitleTrack {
	for index := range tracks {
		if tracks[index].StreamID == streamID {
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
