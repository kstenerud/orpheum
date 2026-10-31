// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// SourceFrame is a frame still the node took from an item's own video (D362): the last resort of a
// film, an episode or an other video that no other source gives a picture of its kind. It is named
// as an image beside the media is, by its video's file, size and modification time (localRef), so
// a changed video is owed a still again, and it is kept as the item's still.
const SourceFrame = library.ArtSourceFrame

// frameAtPercent is where in its video a still is taken: about a tenth in, past the titles of most
// and the black of their start (the owner, 2026-10-27).
const frameAtPercent = 10

// pictureKindsOf are the kinds of picture that show a title of titleKind, any of which, from any
// source but a frame, makes a still of its video needless (D362 §2): a film's poster or backdrop,
// an episode's still, an other video's still or backdrop (D349 §3).
func pictureKindsOf(titleKind string) []string {
	switch titleKind {
	case library.TitleKindMovie:
		return []string{library.ArtPoster, library.ArtBackdrop}
	case library.TitleKindEpisode:
		return []string{library.ArtStill}
	case library.TitleKindOther:
		return []string{library.ArtStill, library.ArtBackdrop}
	}
	return nil
}

// planFrameStills queues, after a pass over a library, a frame still for each film, episode and other
// video that shows no picture of its kind once every other source has been tried (D362 §2, §3): the
// query's settled items, less those still awaiting a match or a season's listing, and those an image
// its sources name is still to come for. They are queued at the background's priority, where a look
// at their tiles raises them (D324). It logs a failure; the next pass plans the same again.
func (service *Service) planFrameStills(ctx context.Context, libraryID int64, found Found, named []ItemImages) {
	queued, err := service.queueFrameStills(ctx, libraryID, found, named)
	if err != nil {
		service.parts.Logger.ErrorContext(ctx, "could not plan the frame stills of a library", "libraryId", libraryID,
			"err", err)
		return
	}
	if queued > 0 {
		service.parts.Logger.InfoContext(ctx, "queued frame stills", "libraryId", libraryID, "queuedJobs", queued)
	}
}

// queueFrameStills is planFrameStills, saying how many jobs it queued.
func (service *Service) queueFrameStills(
	ctx context.Context, libraryID int64, found Found, named []ItemImages,
) (int, error) {
	candidates, err := service.parts.Database.Read().ListFrameStillCandidates(ctx, &libraryID)
	if err != nil {
		return 0, fmt.Errorf("list library %d's videos with no picture: %w", libraryID, err)
	}
	queued := 0
	for _, candidate := range candidates {
		if slices.Contains(found.AwaitingMatch, candidate.TitleID) ||
			(candidate.SeriesID != 0 && slices.Contains(found.AwaitingListings, candidate.SeriesID)) {
			continue
		}
		isToCome, err := service.isPictureToCome(ctx, candidate, named)
		if err != nil {
			return queued, err
		}
		if isToCome {
			continue
		}
		added, err := service.queueFrameStill(ctx, candidate)
		if err != nil {
			return queued, err
		}
		if added {
			queued++
		}
	}
	return queued, nil
}

// isPictureToCome is whether an image a source names of a kind that shows the candidate is still to
// come: owed and not yet asked for, or missing at its source and not yet looked at again (D213 §1).
// One refused for good has been tried.
func (service *Service) isPictureToCome(
	ctx context.Context, candidate sqlcgen.ListFrameStillCandidatesRow, named []ItemImages,
) (bool, error) {
	kinds := pictureKindsOf(candidate.TitleKind)
	var itsOwn []ItemImages
	for _, item := range named {
		if item.ItemID != candidate.TitleID {
			continue
		}
		var refs []Ref
		for _, ref := range item.Refs {
			if slices.Contains(kinds, ref.Kind) {
				refs = append(refs, ref)
			}
		}
		if len(refs) > 0 {
			itsOwn = append(itsOwn, ItemImages{ItemID: item.ItemID, Source: item.Source, Refs: refs})
		}
	}
	if len(itsOwn) == 0 {
		return false, nil
	}
	owed, err := service.owed(ctx, itsOwn)
	if err != nil || len(owed) > 0 {
		return len(owed) > 0, err
	}
	rows, err := service.parts.Database.Read().ListLiveArtwork(ctx, candidate.TitleID)
	if err != nil {
		return false, fmt.Errorf("list item %d's artwork: %w", candidate.TitleID, err)
	}
	return slices.ContainsFunc(rows, func(row sqlcgen.ListLiveArtworkRow) bool {
		return row.IsRecheckOwed == 1 && slices.Contains(kinds, row.Kind)
	}), nil
}

// queueFrameStill queues the job that takes a candidate's still, after retiring a still of its video
// refused before a scan saw the video again, which is owed once more (G9).
func (service *Service) queueFrameStill(
	ctx context.Context, candidate sqlcgen.ListFrameStillCandidatesRow,
) (bool, error) {
	if candidate.StaleRefusalID != 0 {
		now := service.parts.Clock.Now().UnixMilli()
		err := service.parts.Database.Write(ctx, func(queries *sqlcgen.Queries) error {
			return queries.RetireArtworkRow(ctx, sqlcgen.RetireArtworkRowParams{
				RemovedAt: &now, ID: candidate.StaleRefusalID,
			})
		})
		if err != nil {
			return false, fmt.Errorf("ask again for item %d's frame still: %w", candidate.TitleID, err)
		}
	}
	ref := localRef(LocalImage{
		FileID: candidate.FileID, SizeBytes: ptr.ValueOrZero(candidate.SizeBytes),
		ModifiedAtMs: ptr.ValueOrZero(candidate.ModifiedAt),
	})
	args, err := json.Marshal(artworkJobArgs{
		ArtworkSource: SourceFrame, Refs: []Ref{{Kind: library.ArtStill, SourceRef: ref}},
	})
	if err != nil {
		return false, fmt.Errorf("encode item %d's frame still: %w", candidate.TitleID, err)
	}
	added, err := service.parts.Queue.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindMakeArtworkSizes, Target: jobs.Target{ItemID: candidate.TitleID}, ArgsJSON: string(args),
		Priority: jobs.PriorityBackground, Reason: "a still of its video: nothing else gives it a picture",
	})
	if err != nil {
		return false, fmt.Errorf("queue item %d's frame still: %w", candidate.TitleID, err)
	}
	return added, nil
}

// frameToTake is the still a job takes of an item's video, and its node: the video's picture stream,
// about a tenth in (D362 §1). Nothing is taken where the item now shows a picture of its kind from
// any other source (D362 §2), or of a video changed since it was queued, which the next pass plans
// again. A video with no picture stream has no frame, recorded so; a video gone is refused.
func (service *Service) frameToTake(ctx context.Context, want wantedImage) (gathered, error) {
	queries := service.parts.Database.Read()
	if isNeedless, err := service.isStillNeedless(ctx, want); err != nil || isNeedless {
		return gathered{}, err
	}
	fileID, _ := localRefFileID(want.ref.SourceRef)
	file, err := library.PlayableFile(ctx, queries, fileID)
	if errors.Is(err, library.ErrNoFile) {
		return gathered{}, service.recordRefusal(ctx, want, err.Error())
	}
	if err != nil {
		return gathered{}, err
	}
	if !isStillOfFileAsItIs(want.ref.SourceRef, file.SizeBytes) {
		return gathered{}, nil
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		return gathered{}, service.recordRefusal(ctx, want, "its video was never probed")
	}
	streamIndex, hasPicture := pictureStreamOf(probe)
	if !hasPicture {
		return gathered{}, service.recordNoFrame(ctx, want, "its video has no picture")
	}
	located := library.LocatedOf(file)
	if parked := library.ParkIfUnder(ctx, queries, file.NodeID, file.ReportedPathRaw, located.Subject); parked != nil {
		return gathered{}, parked
	}
	video, err := library.BytesFileOf(ctx, queries, located)
	if err != nil {
		return gathered{}, service.recordRefusal(ctx, want, err.Error())
	}
	image := &nodeproto.Artwork{
		Shape: nodeproto.ShapeLandscape, SizeNames: fetchedSizes(nodeproto.ShapeLandscape), Video: &video,
		FrameStreamIndex: streamIndex, FrameAtMs: probe.Format.DurationMs * frameAtPercent / 100,
	}
	return gathered{image: image, nodeID: file.NodeID}, nil
}

// isStillOfFileAsItIs is whether a frame still's ref names its video at the size it has now: one
// changed since its job was queued is not taken, and the node would refuse its bytes by their hash
// too (D100); the next pass plans its still again.
func isStillOfFileAsItIs(ref string, sizeBytes *int64) bool {
	var fileID, refSizeBytes, modifiedAtMs int64
	_, err := fmt.Sscanf(ref, "file:%d:%d:%d", &fileID, &refSizeBytes, &modifiedAtMs)
	return err == nil && refSizeBytes == ptr.ValueOrZero(sizeBytes)
}

// isStillNeedless is whether the item now shows a picture of its kind from a source other than a
// frame: real artwork always wins, and no still is taken where it shows (D362 §2).
func (service *Service) isStillNeedless(ctx context.Context, want wantedImage) (bool, error) {
	queries := service.parts.Database.Read()
	titleKind, err := store.OrNotFound(queries.GetTitleKind(ctx, want.itemID))
	if errors.Is(err, store.ErrNotFound) {
		return true, nil // an item no longer a title, or gone
	}
	if err != nil {
		return false, fmt.Errorf("read title %d's kind: %w", want.itemID, err)
	}
	rows, err := queries.ListLiveArtwork(ctx, want.itemID)
	if err != nil {
		return false, fmt.Errorf("list item %d's artwork: %w", want.itemID, err)
	}
	kinds := pictureKindsOf(titleKind)
	return slices.ContainsFunc(rows, func(row sqlcgen.ListLiveArtworkRow) bool {
		return row.Source != SourceFrame && row.SourceSha256 != nil && slices.Contains(kinds, row.Kind)
	}), nil
}

// pictureStreamOf is a probe's first picture stream, not a cover attached to the file.
func pictureStreamOf(probe nodeproto.Probe) (int, bool) {
	for _, stream := range probe.Streams {
		if stream.Kind == "video" && !slices.Contains(stream.Disposition, "attached_pic") {
			return stream.StreamIndex, true
		}
	}
	return 0, false
}

// recordNoFrame records a video with no frame to take a still of, as a still missing at its source,
// so it is never asked again until the video changes (D362 §5).
func (service *Service) recordNoFrame(ctx context.Context, want wantedImage, why string) error {
	service.parts.Logger.InfoContext(ctx, "a video has no frame for a still", "itemId", want.itemID,
		"jobId", want.job.ID, "sourceRef", want.ref.SourceRef, "reason", why)
	return service.record(ctx, want, nil, &refusal{why: why, isMissingAtSource: true})
}

// keepStill keeps the still a node took of an item's video as any image is kept, and says so with
// where in the video it was taken; a video the node found no frame in is recorded so (D362 §5).
func (service *Service) keepStill(
	ctx context.Context, want wantedImage, image nodeproto.Artwork, made nodeproto.ArtworkResult,
) error {
	if made.HasNoFrame {
		return service.recordNoFrame(ctx, want, made.Refusal)
	}
	if made.Refusal == "" {
		service.parts.Logger.InfoContext(ctx, "a frame still was taken", "itemId", want.itemID, "jobId", want.job.ID,
			"sourceRef", want.ref.SourceRef, "atMs", image.FrameAtMs, "sourceSha256", made.SourceSha256)
	}
	return service.keep(ctx, want, made.SourceSha256, made)
}

// RetireFrameStills retires the frame stills taken from a file, once the facts of its probe they were
// taken by changed (library.DerivedDroppers, D392 §3), in the ingest's write that keeps the new
// probe: the pass after the scan takes one again where its item still shows no other picture. A
// person's choice stays. It says whether there was any.
func (service *Service) RetireFrameStills(
	ctx context.Context, queries *sqlcgen.Queries, of library.DerivedOf,
) (bool, error) {
	retired, err := queries.RetireFrameStillsOfFile(ctx, sqlcgen.RetireFrameStillsOfFileParams{
		RemovedAt: new(service.parts.Clock.Now().UnixMilli()), FileID: of.FileID,
	})
	if err != nil {
		return false, fmt.Errorf("retire the frame stills of file %d: %w", of.FileID, err)
	}
	return retired > 0, nil
}
