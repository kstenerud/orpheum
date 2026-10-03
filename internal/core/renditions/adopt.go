// SPDX-License-Identifier: GPL-3.0-or-later

package renditions

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// AdoptRenditions squares the index with the renditions a node's hello lists, by their manifests
// (D238 §2), and returns the directories of those it keeps, which the node does not evict. One whose
// making the index names is kept, and recorded complete if the index had not heard it end: a node
// that started again lost its word on it, but not the rendition. One the index does not know, as
// after a cache.db discard, is adopted when its source is still that file on that node. One the
// index knows under another making is not kept.
func (service *Service) AdoptRenditions(
	ctx context.Context, nodeID int64, renditions []nodeproto.RenditionManifest,
) ([]string, error) {
	kept := make([]string, 0, len(renditions))
	adopted := 0
	for _, manifest := range renditions {
		isKept, isAdopted, err := service.adopt(ctx, nodeID, manifest)
		if err != nil {
			return nil, err
		}
		if isKept {
			kept = append(kept, manifest.Prepare.RenditionDir)
		}
		if isAdopted {
			adopted++
		}
	}
	if adopted > 0 {
		service.logger.InfoContext(ctx, "adopted the renditions a node holds that the index did not know",
			"nodeId", nodeID, "adopted", adopted)
	}
	return kept, nil
}

// adopt squares the index with one rendition a node holds, as AdoptRenditions says.
func (service *Service) adopt(
	ctx context.Context, nodeID int64, manifest nodeproto.RenditionManifest,
) (isKept, isAdopted bool, err error) {
	run, err := parseRenditionDir(manifest.Prepare.RenditionDir)
	if err != nil || run.profile != string(manifest.Prepare.Profile) {
		return false, false, nil //nolint:nilerr // a directory that names no making is not a rendition to keep
	}
	key := cachegen.GetRenditionOfFileAndProfileParams{SourceFile: run.fileID, Profile: run.profile}
	existing, err := store.OrNotFound(service.cache.Read().GetRenditionOfFileAndProfile(ctx, key))
	switch {
	case errors.Is(err, store.ErrNotFound):
		isAdopted, err = service.adoptUnknown(ctx, nodeID, run, manifest)
		return isAdopted, isAdopted, err
	case err != nil:
		return false, false, fmt.Errorf("read rendition %s: %w", run.dir(), err)
	case existing.RunID != run.runID:
		return false, false, nil
	case existing.State != renditionComplete:
		_, err = service.finish(ctx, run, &nodeproto.PrepareReport{
			State: nodeproto.PrepareDone, SizeBytes: manifest.SizeBytes, VideoEncoder: manifest.VideoEncoder,
			RealtimeFactor: manifest.RealtimeFactor,
		}, "")
		return err == nil, false, err
	}
	return true, false, nil
}

// adoptUnknown adopts a rendition the index does not know, complete, when its source is still the
// file it names, on the node that holds it, and reports whether it did.
func (service *Service) adoptUnknown(
	ctx context.Context, nodeID int64, run renditionRun, manifest nodeproto.RenditionManifest,
) (bool, error) {
	file, err := store.OrNotFound(service.database.Read().GetPlayableFile(ctx, run.fileID))
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read file %d: %w", run.fileID, err)
	}
	if file.NodeID != nodeID || hex.EncodeToString(file.PayloadHash) != manifest.Prepare.Video.PayloadHash {
		return false, nil
	}
	var inserted int64
	err = service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		var insertErr error
		inserted, insertErr = queries.InsertAdoptedRendition(ctx, cachegen.InsertAdoptedRenditionParams{
			SourceFile: run.fileID, SourcePayloadHash: file.PayloadHash, Profile: run.profile, NodeID: nodeID,
			SegmentLengthSeconds: int64(manifest.Prepare.SegmentLengthSeconds), RunID: run.runID,
			SizeBytes: &manifest.SizeBytes, VideoEncoder: ptr.NilIfZero(manifest.VideoEncoder),
			RealtimeFactor:     ptr.NilIfZero(manifest.RealtimeFactor),
			CarriedGainsDBJSON: decide.CarriedGains(manifest.Prepare.AudioTracks),
			AudioLanguagesJSON: audioLanguagesOf(manifest.Prepare.AudioTracks), MadeAt: manifest.MadeAtMs,
		})
		return insertErr
	})
	if err != nil {
		return false, fmt.Errorf("adopt rendition %s: %w", run.dir(), err)
	}
	return inserted > 0, nil
}
