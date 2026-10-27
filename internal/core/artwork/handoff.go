// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Handle runs one make_artwork_sizes job as a scan is run (the owner, 2026-10-02): it fetches what
// each image it names needs from the image's provider, which only the core reaches, and records
// what is missing or refused there; then gives every image it still wants to one node in one
// command, which re-encodes each and answers once for all (Assignee). Images beside the media on
// another node than the first's are left for the next pass to ask for again.
func (service *Service) Handle(ctx context.Context, job jobs.Job) error {
	ctx = providers.WithImportance(ctx, providers.JobImportance(job.Priority))
	args, err := argsOf(job)
	if err != nil {
		return err
	}
	fetcher, known := service.parts.Fetchers[args.ArtworkSource]
	if !known && args.ArtworkSource != SourceLocal && args.ArtworkSource != SourceFrame {
		return fmt.Errorf("make_artwork_sizes job %d: %w", job.ID, jobs.MarkPermanent(fmt.Errorf("no fetcher for %q",
			args.ArtworkSource)))
	}
	images, nodeID, err := service.gather(ctx, fetcher, job, args)
	if err != nil || len(images) == 0 {
		return err
	}
	maxSourcePixels, err := service.parts.Settings.Int(ctx, settings.ArtworkMaxSourcePixels)
	if err != nil {
		return fmt.Errorf("read artwork_max_source_pixels: %w", err)
	}
	quality, err := service.parts.Settings.Int(ctx, settings.ArtworkWebPQuality)
	if err != nil {
		return fmt.Errorf("read artwork_webp_quality: %w", err)
	}
	for index := range images {
		images[index].MaxSourcePixels, images[index].WebPQuality = maxSourcePixels, int(quality)
	}
	return service.exchanges.Assign(ctx, nodeID, job, nodeproto.Command{Work: nodeproto.Work{
		Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: &nodeproto.ArtworkBatch{Images: images},
	}})
}

// argsOf is a make_artwork_sizes job's arguments.
func argsOf(job jobs.Job) (artworkJobArgs, error) {
	var args artworkJobArgs
	if err := json.Unmarshal([]byte(job.ArgsJSON), &args); err != nil {
		return args, fmt.Errorf("make_artwork_sizes job %d: %w", job.ID, jobs.MarkPermanent(err))
	}
	return args, nil
}

// gather is every image the job still wants re-encoded, each named by its key, and the node to do
// it.
func (service *Service) gather(
	ctx context.Context, fetcher Fetcher, job jobs.Job, args artworkJobArgs,
) ([]nodeproto.Artwork, int64, error) {
	var images []nodeproto.Artwork
	var nodeID int64
	for index, ref := range args.Refs {
		want := wantedImage{job: job, itemID: job.ItemID, source: args.ArtworkSource, ref: ref}
		var found gathered
		var err error
		if len(args.SizeNames) > 0 {
			found, err = service.sizesToMake(ctx, fetcher, want, args)
		} else {
			found, err = service.imageToFetch(ctx, fetcher, want)
		}
		if err != nil {
			return nil, 0, err
		}
		if found.image == nil || (nodeID != 0 && found.nodeID != nodeID) {
			continue
		}
		nodeID = found.nodeID
		found.image.Key = imageKey(index, found.isFromBase)
		images = append(images, *found.image)
	}
	return images, nodeID, nil
}

// gathered is one image a job wants re-encoded, the node that is to, and whether what it is cut from
// is a size already made, its base; no image when nothing is to be re-encoded.
type gathered struct {
	image      *nodeproto.Artwork
	nodeID     int64
	isFromBase bool
}

// imageKey names one of a job's images to the node: its place among the job's refs, and whether
// what it is cut from is a size already made, its base.
func imageKey(index int, isFromBase bool) string {
	key := strconv.Itoa(index)
	if isFromBase {
		key += "/base"
	}
	return key
}

// keyOf is the place and the base an image's key names, and whether it names one of refCount.
func keyOf(key string, refCount int) (index int, isFromBase, isKey bool) {
	place, base, hasBase := strings.Cut(key, "/")
	index, err := strconv.Atoi(place)
	if err != nil || index < 0 || index >= refCount || (hasBase && base != "base") {
		return 0, false, false
	}
	return index, hasBase, true
}

// imageToFetch is the image a job fetches for an item, to re-encode, and its node; nil for one the
// item holds, a kind this release does not show, or one missing or refused at its source, which is
// recorded so.
func (service *Service) imageToFetch(ctx context.Context, fetcher Fetcher, want wantedImage) (gathered, error) {
	shape, known := shapeOf(want.ref.Kind)
	if !known {
		return gathered{}, nil
	}
	held, err := service.isHeld(ctx, want)
	if err != nil || held {
		return gathered{}, err
	}
	if want.source == SourceFrame {
		return service.frameToTake(ctx, want)
	}
	image := &nodeproto.Artwork{Shape: shape, SizeNames: fetchedSizes(shape)}
	var nodeID int64
	if want.source == SourceLocal {
		nodeID, image.AbsolutePath, err = service.localFile(ctx, want.ref.SourceRef)
		if errors.Is(err, ErrNoImage) {
			return gathered{}, service.recordRefusal(ctx, want, err.Error())
		}
		return gathered{image: image, nodeID: nodeID}, err
	}
	download, err := service.fetch(ctx, fetcher, want.ref.SourceRef)
	if errors.Is(err, ErrNoImage) {
		return gathered{}, service.recordMissing(ctx, want, err)
	}
	if err != nil {
		return gathered{}, err
	}
	image.SourceImage, image.SourceSha256 = download.bytes, download.sourceSha256Hex()
	nodeID, err = service.anyNode(ctx)
	return gathered{image: image, nodeID: nodeID}, err
}

// sizesToMake is a stored image's sizes to make, from its base when the base is large enough — our
// own WebP, so nothing is fetched (D212 §2) — else from its source, fetched again, and its node; nil
// when a source gone or changed is noted so no request asks again.
func (service *Service) sizesToMake(
	ctx context.Context, fetcher Fetcher, want wantedImage, args artworkJobArgs,
) (gathered, error) {
	shape, known := shapeOf(want.ref.Kind)
	if !known {
		return gathered{}, nil
	}
	image := &nodeproto.Artwork{Shape: shape, SizeNames: args.SizeNames, SourceSha256: args.SourceSha256}
	if base, found := service.baseNameFor(args.SourceSha256, args.SizeNames); found {
		if stored, readErr := service.parts.ArtFiles.ReadArt(args.SourceSha256, base); readErr == nil {
			image.SourceImage = stored
			nodeID, err := service.anyNode(ctx)
			return gathered{image: image, nodeID: nodeID, isFromBase: true}, err
		}
	}
	if want.source == SourceFrame {
		// A still is not taken again for a size its base is too small for: the frame it was taken
		// from is all there is of it (D362 §1).
		return gathered{}, service.noteCannotMake(ctx, args.SourceSha256, args.SizeNames)
	}
	if want.source == SourceLocal {
		image.SourceSha256 = ""
		nodeID, path, err := service.localFile(ctx, want.ref.SourceRef)
		if errors.Is(err, ErrNoImage) {
			return gathered{}, service.noteCannotMake(ctx, args.SourceSha256, args.SizeNames)
		}
		image.AbsolutePath = path
		return gathered{image: image, nodeID: nodeID}, err
	}
	download, err := service.fetch(ctx, fetcher, want.ref.SourceRef)
	if errors.Is(err, ErrNoImage) || (err == nil && download.sourceSha256Hex() != args.SourceSha256) {
		service.parts.Logger.WarnContext(ctx, "an image's source is gone or changed; its sizes are not made",
			"sourceSha256", args.SourceSha256, "source", want.source, "sourceRef", want.ref.SourceRef)
		return gathered{}, service.noteCannotMake(ctx, args.SourceSha256, args.SizeNames)
	}
	if err != nil {
		return gathered{}, err
	}
	image.SourceImage = download.bytes
	nodeID, err := service.anyNode(ctx)
	return gathered{image: image, nodeID: nodeID}, err
}

// Assignee is how a make_artwork_sizes job given to a node is resolved: each image's answer is kept
// as its own was, the item's image recorded, sizes it cannot be made at noted; a node that started
// again lost the job, the tool's failure (D238 §3).
func (service *Service) Assignee(runner nodes.Resolver) nodes.Assignee {
	return nodes.Assignee{
		Runner: runner,
		Answer: service.takeAnswer,
		Unanswered: func(_ context.Context, job jobs.Job, _ nodes.Attempt, err error) error {
			return nodes.UnansweredOutcome(job, err, nil)
		},
	}
}

// takeAnswer takes a node's answer on a job's images: each is kept, or its failure is the job's,
// tried again for what it still wants.
func (service *Service) takeAnswer(
	ctx context.Context, job jobs.Job, attempt nodes.Attempt, result nodeproto.Result,
) error {
	sent := attempt.Work.EncodeArtwork
	if result.Artwork == nil || sent == nil {
		return fmt.Errorf("node %d answered re-encoding job %d's images with something else", attempt.NodeID, job.ID)
	}
	args, err := argsOf(job)
	if err != nil {
		return err
	}
	var errs []error
	for _, made := range result.Artwork.Images {
		index, isFromBase, isKey := keyOf(made.Key, len(args.Refs))
		sentIndex := slices.IndexFunc(sent.Images, func(image nodeproto.Artwork) bool { return image.Key == made.Key })
		if !isKey || sentIndex < 0 {
			errs = append(errs, fmt.Errorf("node %d answered for an image job %d did not send: %q", attempt.NodeID,
				job.ID, made.Key))
			continue
		}
		want := wantedImage{job: job, itemID: job.ItemID, source: args.ArtworkSource, ref: args.Refs[index]}
		image := sent.Images[sentIndex]
		switch {
		case len(args.SizeNames) > 0:
			errs = append(errs, service.takeCut(ctx, image, args, !isFromBase, made))
		case image.Video != nil:
			errs = append(errs, service.keepStill(ctx, want, image, made))
		default:
			errs = append(errs, service.keep(ctx, want, cmp.Or(image.SourceSha256, made.SourceSha256), made))
		}
	}
	return errors.Join(errs...)
}

// takeCut stores the sizes the node cut from a source. Only the image's own source can say a size
// cannot be made; a size a base did not yield is an error, retried.
func (service *Service) takeCut(
	ctx context.Context, image nodeproto.Artwork, args artworkJobArgs, isFromSource bool, made nodeproto.ArtworkResult,
) error {
	if image.AbsolutePath != nil && made.Refusal == "" && made.SourceSha256 != args.SourceSha256 {
		service.parts.Logger.WarnContext(ctx, "an image beside the media changed; its sizes are not made",
			"sourceSha256", args.SourceSha256)
		return service.noteCannotMake(ctx, args.SourceSha256, args.SizeNames)
	}
	var unmade []nodeproto.ArtSizeName
	for _, size := range args.SizeNames {
		madeIndex := slices.IndexFunc(made.Images, func(each nodeproto.ArtImage) bool { return each.SizeName == size })
		if madeIndex < 0 || !isWebP(made.Images[madeIndex].WebP) {
			unmade = append(unmade, size)
			continue
		}
		webp := made.Images[madeIndex].WebP
		if err := service.writeArt(ctx, args.SourceSha256, string(size)+webpSuffix, webp); err != nil {
			return err
		}
	}
	if len(unmade) > 0 && !isFromSource {
		return fmt.Errorf("the node cut no %v from art %s's base", unmade, args.SourceSha256)
	}
	return service.noteCannotMake(ctx, args.SourceSha256, unmade)
}

// recordArtwork stores the sizes a re-encode made, as it arrives and whether or not its job still
// waits (D238 §2): for each image, each size asked for, in WebP, under the SHA-256 of the image it
// is of, the command's or, for an image beside the media, the node's. The item's image is its job's
// to record.
func (service *Service) recordArtwork(ctx context.Context, _ int64, result nodeproto.Result) error {
	answer, sent := result.Artwork, result.Work.EncodeArtwork
	if answer == nil || sent == nil {
		return fmt.Errorf("a re-encode's result carries no images, or names no re-encode: %w", nodes.ErrMalformedResult)
	}
	for _, made := range answer.Images {
		sentIndex := slices.IndexFunc(sent.Images, func(image nodeproto.Artwork) bool { return image.Key == made.Key })
		if sentIndex < 0 || made.Refusal != "" {
			continue
		}
		image := sent.Images[sentIndex]
		sourceSha256Hex := cmp.Or(image.SourceSha256, made.SourceSha256)
		if !isSha256Hex(sourceSha256Hex) {
			continue
		}
		for _, size := range made.Images {
			if !slices.Contains(image.SizeNames, size.SizeName) || !isWebP(size.WebP) {
				continue
			}
			if err := service.writeArt(ctx, sourceSha256Hex, string(size.SizeName)+webpSuffix, size.WebP); err != nil {
				return err
			}
		}
	}
	return nil
}
