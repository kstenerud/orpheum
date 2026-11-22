// SPDX-License-Identifier: AGPL-3.0-or-later

package preferences

import (
	"context"
	"fmt"
	"strconv"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A file's sound offset (D537) moves its sound from its picture, in milliseconds, later when positive
// and earlier when negative, as a device's sound offset does; a player plays the file with the two
// added. Every person's is an admin's, kept in file_sound_offset; a person's own is a preference at
// the file's scope, which overrides every person's for them until they forget it. Both are the
// file's, not its video's, and a rescan keeps both: a person entered them (D535 §6).

// axisSoundOffsetMs is a file's axis: the person's own sound offset for it (D537).
const axisSoundOffsetMs = "sound_offset_ms"

// SoundOffsetLimitMs is how far a file's sound offset, every person's or a person's own, moves its
// sound at most, either way: as far as a device's does.
const SoundOffsetLimitMs = 2000

// FileSoundOffset is a file's sound offset as one person's playback of it starts with (D537).
type FileSoundOffset struct {
	// EveryonesMs is every person's, an admin's; 0 for a file in step.
	EveryonesMs int64
	// OwnMs is the person's own, which overrides EveryonesMs for them; nil for none.
	OwnMs *int64
}

// Ms is the offset the file plays with for the person: their own, or else every person's.
func (offset FileSoundOffset) Ms() int64 {
	if offset.OwnMs != nil {
		return *offset.OwnMs
	}
	return offset.EveryonesMs
}

// fileSoundOffset reads a file's sound offset for a person through queries: every person's, and own,
// the person's preference at the file read with their others (ForFile).
func fileSoundOffset(ctx context.Context, queries *sqlcgen.Queries, fileID int64, own *int64) (FileSoundOffset, error) {
	everyones, err := queries.GetFileSoundOffset(ctx, fileID)
	if err != nil {
		return FileSoundOffset{}, fmt.Errorf("read the sound offset of file %d: %w", fileID, err)
	}
	return FileSoundOffset{EveryonesMs: everyones, OwnMs: own}, nil
}

// KeepEveryonesSoundOffset keeps a file's sound offset for every person, through queries, the
// transaction that found the file: an admin's (D537). 0 puts the file back in step. A person's own
// stays theirs.
func (service *Service) KeepEveryonesSoundOffset(
	ctx context.Context, queries *sqlcgen.Queries, fileID, offsetMs int64,
) error {
	var err error
	if offsetMs == 0 {
		err = queries.DeleteFileSoundOffset(ctx, fileID)
	} else {
		err = queries.SetFileSoundOffset(ctx, sqlcgen.SetFileSoundOffsetParams{
			FileID: fileID, OffsetMs: offsetMs, SetAt: service.clock.Now().UnixMilli(),
		})
	}
	if err != nil {
		return fmt.Errorf("keep every person's sound offset of file %d: %w", fileID, err)
	}
	return nil
}

// KeepSoundOffset keeps a person's own sound offset for a file, through queries, which overrides
// every person's for them (D537). 0 is a choice too: the file in step for them, whatever every
// person's is.
func (service *Service) KeepSoundOffset(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID, offsetMs int64,
) error {
	inFile := place{userID: userID, scope: scopeFile, scopeID: fileID, axis: axisSoundOffsetMs}
	return service.setRaw(ctx, queries, replacing, inFile, strconv.FormatInt(offsetMs, 10))
}

// ForgetSoundOffset forgets a person's own sound offset for a file, through queries, so it plays with
// every person's for them again (D537).
func (*Service) ForgetSoundOffset(ctx context.Context, queries *sqlcgen.Queries, userID, fileID int64) error {
	err := queries.DeletePreference(ctx, sqlcgen.DeletePreferenceParams{
		UserID: userID, Scope: scopeFile, ScopeID: fileID, Axis: axisSoundOffsetMs,
	})
	if err != nil {
		return fmt.Errorf("forget user %d's sound offset of file %d: %w", userID, fileID, err)
	}
	return nil
}
