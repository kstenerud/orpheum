// SPDX-License-Identifier: AGPL-3.0-or-later

package trickplay

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// ErrNoPictures is a file none of whose scrub-bar pictures are made yet, or one that has none: a
// cover alone, or bytes not read.
var ErrNoPictures = errors.New("no scrub-bar pictures are made of the file")

// ErrNoSheet is a sheet that is not one of the file's.
var ErrNoSheet = errors.New("no such sheet of the file's scrub-bar pictures")

// Pictures are a file's scrub-bar pictures as made so far (trickplay.md section 5): the version's
// sizes, how many pictures the whole film has, and each sheet made, in order. A sheet not among them
// is a stretch with no pictures yet.
type Pictures struct {
	IntervalMs    int64
	PictureWidth  int
	PictureHeight int
	Columns       int
	Rows          int
	// PictureCount is the whole film's: a picture at 0 and at each interval before its end.
	PictureCount int
	Sheets       []Sheet
}

// Sheet is one sheet made: its index from the film's start, and its image's SHA-256, its address.
type Sheet struct {
	Index     int
	Sha256Hex string
}

// PicturesOf are the scrub-bar pictures of a file at the current version; ErrNoPictures while none
// is made. Whether the asker reaches the file is the caller's check.
func (service *Service) PicturesOf(ctx context.Context, fileID int64) (Pictures, error) {
	file, err := library.PlayableFile(ctx, service.database.Read(), fileID)
	if err != nil {
		return Pictures{}, fmt.Errorf("%w: %w", ErrNoPictures, err)
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		return Pictures{}, fmt.Errorf("%w: %w", ErrNoPictures, err)
	}
	rows, err := service.cache.Read().ListTrickplaySheets(ctx, cachegen.ListTrickplaySheetsParams{
		PayloadHash: file.PayloadHash, Version: StoredVersion(),
	})
	if err != nil {
		return Pictures{}, fmt.Errorf("read the trickplay sheets of file %d: %w", fileID, err)
	}
	if len(rows) == 0 {
		return Pictures{}, fmt.Errorf("file %d: %w", fileID, ErrNoPictures)
	}
	pictures := Pictures{
		IntervalMs: IntervalMs, PictureWidth: PictureWidth, PictureHeight: int(rows[0].PictureHeight),
		Columns: SheetColumns, Rows: SheetRows, PictureCount: pictureCount(probe.Format.DurationMs),
	}
	for _, row := range rows {
		pictures.Sheets = append(pictures.Sheets, Sheet{Index: int(row.SheetIndex), Sha256Hex: row.SheetSha256Hex})
	}
	return pictures, nil
}

// pictureCount is how many pictures a film of durationMs has: one at its start and one at each
// interval before its end, as the node's fps filter marks them (internal/node/trickplay).
func pictureCount(durationMs int64) int {
	return int(max((durationMs+IntervalMs-1)/IntervalMs, 1))
}

// SheetOf is the image of one of a file's sheets by its SHA-256, at any version; ErrNoSheet for one
// that is not the file's, or is no longer kept. Whether the asker reaches the file is the caller's
// check.
func (service *Service) SheetOf(ctx context.Context, fileID int64, sha256Hex string) ([]byte, error) {
	file, err := library.PlayableFile(ctx, service.database.Read(), fileID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoSheet, err)
	}
	isFiles, err := service.cache.Read().HasTrickplaySheetFile(ctx, cachegen.HasTrickplaySheetFileParams{
		PayloadHash: file.PayloadHash, SheetSha256Hex: sha256Hex,
	})
	if err != nil {
		return nil, fmt.Errorf("read whether sheet %s is file %d's: %w", sha256Hex, fileID, err)
	}
	if !isFiles {
		return nil, fmt.Errorf("sheet %s of file %d: %w", sha256Hex, fileID, ErrNoSheet)
	}
	contents, err := service.sheets.ReadTrickplaySheet(sha256Hex)
	if errors.Is(err, instancedir.ErrNotFound) {
		// Thrown out with its row on its way out: the sheet is no longer kept.
		return nil, fmt.Errorf("sheet %s of file %d: %w: %w", sha256Hex, fileID, ErrNoSheet, err)
	}
	if err != nil {
		return nil, fmt.Errorf("read sheet %s of file %d: %w", sha256Hex, fileID, err)
	}
	return contents, nil
}
