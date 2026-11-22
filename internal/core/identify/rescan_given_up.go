// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Every rescan, discarding or not, forgets the attempts to gather information given up on in its
// scope, so nothing stands in the way of trying each again fresh (D535 §8): a provider's failure to
// fetch its films, series and episodes; a failed loudness measurement, picture timeline, repeat
// measurement, subtitle text and whole-file hash of its files; and the failures of unknown cause
// recorded against them. What succeeded stays (D535 §5) unless the rescan discards (§6). A scan
// forgets none of it. A device's player giving up a file's picture (own_picture_given_up) is how a
// playback went, not an attempt to gather information, and stays.

// GivenUpForgetters are how a rescan forgets what was given up on that another package keeps, each
// by the package that keeps it: set by the server's wiring (SetGivenUpForgetters). One left nil
// forgets nothing.
type GivenUpForgetters struct {
	// FFmpegOfFiles each forget, in the rescan's write, one package's attempts FFmpeg gave up on of the
	// files named, a JSON array of ids, and say how many: readthrough.ForgetFailuresOfFiles,
	// repeats.ForgetFailuresOfFiles. A node's new FFmpeg forgets these of its files too, with
	// SubtitleText (ForgetFFmpegFailuresOfFiles, D535 §9). One package writes each table.
	FFmpegOfFiles []FileFailuresForgetter
	// OfFiles each forget, in the rescan's write, one package's other attempts given up on of the
	// files named, as FFmpegOfFiles do: library.ForgetHashFailuresOfFiles, a whole-file hash not being
	// FFmpeg's.
	OfFiles         []FileFailuresForgetter
	SubtitleText    func(ctx context.Context, fileIDs []int64) error            // subtitles.Service.ForgetTextFailures
	UnknownFailures func(ctx context.Context, subjects []string) (int64, error) // unexpected.Recorder.Forget
}

// ForgetFFmpegFailuresOfFiles forgets, in a write, the attempts FFmpeg gave up on of the files named,
// a JSON array of ids, by each FFmpegOfFiles, and says how many: a rescan's (D535 §8) and a node's
// new FFmpeg's (§9). What succeeded stays.
func (forgetters GivenUpForgetters) ForgetFFmpegFailuresOfFiles(
	ctx context.Context, queries *sqlcgen.Queries, fileIDsJSON string,
) (int64, error) {
	var forgotten int64
	for _, forget := range forgetters.FFmpegOfFiles {
		count, err := forget(ctx, queries, fileIDsJSON)
		if err != nil {
			return forgotten, err
		}
		forgotten += count
	}
	return forgotten, nil
}

// ForgetFFmpegSubtitleFailures forgets, in a write of its own, why the subtitle texts of files could
// not be made (SubtitleText): FFmpeg's, which a node's new FFmpeg forgets of its files (D535 §9).
func (forgetters GivenUpForgetters) ForgetFFmpegSubtitleFailures(ctx context.Context, fileIDs []int64) error {
	if forgetters.SubtitleText == nil {
		return nil
	}
	return forgetters.SubtitleText(ctx, fileIDs)
}

// FileFailuresForgetter forgets, in a rescan's write, one kind of attempt given up on of the files
// named, a JSON array of ids, and says how many it forgot.
type FileFailuresForgetter func(ctx context.Context, queries *sqlcgen.Queries, fileIDsJSON string) (int64, error)

// SetGivenUpForgetters sets how a rescan forgets what another package keeps of what was given up on.
func (rescans *Rescans) SetGivenUpForgetters(forgetters GivenUpForgetters) {
	rescans.givenUp = forgetters
}

// givenUpScope is what a rescan forgets the attempts given up on of: the live items filed in its
// library and the files at a live place under its roots, or its one item's, a series' episodes with
// it, and their files.
func (matcher *Matcher) givenUpScope(
	ctx context.Context, libraryID int64, itemID *int64,
) (items, files []int64, err error) {
	queries := matcher.database.Read()
	if itemID == nil {
		if items, err = queries.ListLiveItemIDsOfLibrary(ctx, &libraryID); err != nil {
			return nil, nil, fmt.Errorf("list library %d's items: %w", libraryID, err)
		}
		if files, err = queries.ListLiveFileIDsOfLibrary(ctx, libraryID); err != nil {
			return nil, nil, fmt.Errorf("list library %d's files: %w", libraryID, err)
		}
		return items, files, nil
	}
	rows, err := queries.ListFilesOfWork(ctx, sqlcgen.ListFilesOfWorkParams{ItemID: *itemID, MaxRows: math.MaxInt32})
	if err != nil {
		return nil, nil, fmt.Errorf("list item %d's files: %w", *itemID, err)
	}
	items = []int64{*itemID}
	for _, row := range rows {
		if !slices.Contains(items, row.WorkID) {
			items = append(items, row.WorkID)
		}
		if !slices.Contains(files, row.ID) {
			files = append(files, row.ID)
		}
	}
	return items, files, nil
}

// forgetGivenUp forgets the attempts given up on in a rescan's scope, a library or one item in it.
func (rescans *Rescans) forgetGivenUp(ctx context.Context, libraryID int64, itemID *int64) error {
	matcher := rescans.matcher
	items, files, err := matcher.givenUpScope(ctx, libraryID, itemID)
	if err != nil {
		return err
	}
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("name the rescan's items: %w", err)
	}
	filesJSON, err := json.Marshal(files)
	if err != nil {
		return fmt.Errorf("name the rescan's files: %w", err)
	}
	var forgotten int64
	err = matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var forgetErr error
		if forgotten, forgetErr = queries.ForgetFetchFailuresOfItems(ctx, string(itemsJSON)); forgetErr != nil {
			return fmt.Errorf("forget the failures to fetch: %w", forgetErr)
		}
		ffmpeg, ffmpegErr := rescans.givenUp.ForgetFFmpegFailuresOfFiles(ctx, queries, string(filesJSON))
		if ffmpegErr != nil {
			return ffmpegErr
		}
		forgotten += ffmpeg
		for _, forget := range rescans.givenUp.OfFiles {
			count, ofFilesErr := forget(ctx, queries, string(filesJSON))
			if ofFilesErr != nil {
				return ofFilesErr
			}
			forgotten += count
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("forget the failures in library %d's rescan: %w", libraryID, err)
	}
	if subtitleErr := rescans.givenUp.ForgetFFmpegSubtitleFailures(ctx, files); subtitleErr != nil {
		return subtitleErr
	}
	unknown, err := rescans.forgetUnknownFailures(ctx, items, files, itemsJSON, filesJSON)
	if err != nil {
		return err
	}
	matcher.logger.InfoContext(ctx, "a rescan forgot the attempts given up on in its scope", "libraryId", libraryID,
		"itemCount", len(items), "fileCount", len(files), "failures", forgotten, "unknownFailures", unknown)
	return nil
}

// forgetUnknownFailures forgets the failures of unknown cause of a rescan's items and files, of the
// subtitles fetched for those files and of the seasons its episodes are in, so their work runs again
// this start (D511 §3).
func (rescans *Rescans) forgetUnknownFailures(
	ctx context.Context, items, files []int64, itemsJSON, filesJSON []byte,
) (int64, error) {
	if rescans.givenUp.UnknownFailures == nil {
		return 0, nil
	}
	queries := rescans.matcher.database.Read()
	fetched, err := queries.ListFetchedSubtitleIDsOfFiles(ctx, string(filesJSON))
	if err != nil {
		return 0, fmt.Errorf("list the subtitles fetched for the rescan's files: %w", err)
	}
	groups, err := queries.ListAiredGroupIDsOfWorks(ctx, string(itemsJSON))
	if err != nil {
		return 0, fmt.Errorf("list the seasons of the rescan's episodes: %w", err)
	}
	subjects := make([]string, 0, len(items)+len(files)+len(fetched)+len(groups))
	for _, id := range items {
		subjects = append(subjects, jobs.Subject(jobs.Target{ItemID: id}))
	}
	for _, id := range files {
		subjects = append(subjects, jobs.Subject(jobs.Target{FileID: id}))
	}
	for _, id := range fetched {
		subjects = append(subjects, jobs.Subject(jobs.Target{FetchedSubtitleID: id}))
	}
	for _, id := range groups {
		subjects = append(subjects, jobs.Subject(jobs.Target{GroupID: id}))
	}
	return rescans.givenUp.UnknownFailures(ctx, subjects)
}
