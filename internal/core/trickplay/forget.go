// SPDX-License-Identifier: AGPL-3.0-or-later

package trickplay

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// Forget throws out the pictures made of a file's bytes, at every version, once the probe's facts
// they were made from change (library.DerivedDroppers.Trickplay, D392 §3): their rows, then each
// sheet's image no other row names. It says whether there were any.
func (service *Service) Forget(ctx context.Context, of library.DerivedOf) (bool, error) {
	return service.forget(ctx, of.PayloadHash)
}

// forget throws out what is kept of payloadHash's pictures.
func (service *Service) forget(ctx context.Context, payloadHash []byte) (bool, error) {
	service.files.Lock()
	defer service.files.Unlock()
	var images []string
	var forgotten int64
	err := service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		var err error
		if images, err = queries.ListTrickplaySheetFilesOf(ctx, payloadHash); err != nil {
			return err
		}
		sheets, err := queries.DeleteTrickplaySheetsOf(ctx, payloadHash)
		if err != nil {
			return err
		}
		rows, err := queries.DeleteTrickplayOf(ctx, payloadHash)
		forgotten = sheets + rows
		return err
	})
	if err != nil {
		return false, fmt.Errorf("forget the trickplay of %s: %w", hex.EncodeToString(payloadHash), err)
	}
	for _, image := range images {
		if err := service.removeUnnamed(ctx, image); err != nil {
			return true, err
		}
	}
	return forgotten > 0, nil
}

// removeUnnamed removes a sheet's image once no row names it; the caller holds service.files.
func (service *Service) removeUnnamed(ctx context.Context, image string) error {
	named, err := service.cache.Read().CountTrickplaySheetsOfFile(ctx, image)
	if err != nil {
		return fmt.Errorf("count the trickplay sheets of image %s: %w", image, err)
	}
	if named > 0 {
		return nil
	}
	if err := service.sheets.RemoveTrickplaySheet(image); err != nil {
		return fmt.Errorf("remove trickplay image %s: %w", image, err)
	}
	return nil
}

// ForgetTheRemoved throws out the pictures of bytes no file names any more, a file removed or
// changed, and every image in the cache directory no row names, as a cache.db thrown out leaves them
// (D231 §3). It queues nothing: a sweep's Derive, which says how many it queued.
func (service *Service) ForgetTheRemoved(ctx context.Context) (int, error) {
	named, err := service.database.Read().ListPayloadHashesOfFiles(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the files' payload hashes: %w", err)
	}
	isNamed := make(map[string]bool, len(named))
	for _, payloadHash := range named {
		isNamed[string(payloadHash)] = true
	}
	kept, err := service.cache.Read().ListTrickplayPayloadHashes(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the trickplays kept: %w", err)
	}
	for _, payloadHash := range kept {
		if isNamed[string(payloadHash)] {
			continue
		}
		if _, forgetErr := service.forget(ctx, payloadHash); forgetErr != nil {
			return 0, forgetErr
		}
	}
	service.files.Lock()
	defer service.files.Unlock()
	images, err := service.sheets.ListTrickplaySheets()
	if err != nil {
		return 0, fmt.Errorf("list the trickplay images: %w", err)
	}
	for _, image := range images {
		if err := service.removeUnnamed(ctx, image); err != nil {
			return 0, err
		}
	}
	return 0, nil
}
