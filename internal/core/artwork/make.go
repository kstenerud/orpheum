// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
)

// makeSizes makes sizes of a stored image: cut by the node from its base when the base is large
// enough — our own WebP, so nothing is fetched (D212 §2) — else from its source, fetched again.
// A size the source is too small for, or a source that is gone or changed, is noted so no
// request asks again.
func (service *Service) makeSizes(ctx context.Context, fetcher Fetcher, want wantedImage, args artworkJobArgs) error {
	shape, known := shapeOf(want.ref.Kind)
	if !known {
		return nil
	}
	command := nodeproto.Artwork{Shape: shape, SizeNames: args.SizeNames}
	if base, found := service.baseNameFor(args.SourceSha256, args.SizeNames); found {
		stored, err := service.parts.ArtFiles.ReadArt(args.SourceSha256, base)
		if err == nil {
			nodeID, nodeErr := service.anyNode(ctx)
			if nodeErr != nil {
				return nodeErr
			}
			command.SourceImage, command.SourceSha256 = stored, args.SourceSha256
			return service.cutFrom(ctx, nodeID, want.job, command, args, false)
		}
	}
	if want.source == SourceLocal {
		nodeID, named, err := service.localFile(ctx, want.ref.SourceRef)
		if errors.Is(err, ErrNoImage) {
			return service.noteCannotMake(ctx, args.SourceSha256, args.SizeNames)
		}
		if err != nil {
			return err
		}
		command.AbsolutePath = named
		return service.cutFrom(ctx, nodeID, want.job, command, args, true)
	}
	download, err := service.fetch(ctx, fetcher, want.ref.SourceRef)
	if errors.Is(err, ErrNoImage) || (err == nil && download.sourceSha256Hex() != args.SourceSha256) {
		service.parts.Logger.WarnContext(ctx, "an image's source is gone or changed; its sizes are not made",
			"sourceSha256", args.SourceSha256, "source", want.source, "sourceRef", want.ref.SourceRef)
		return service.noteCannotMake(ctx, args.SourceSha256, args.SizeNames)
	}
	if err != nil {
		return err
	}
	nodeID, err := service.anyNode(ctx)
	if err != nil {
		return err
	}
	command.SourceImage, command.SourceSha256 = download.bytes, args.SourceSha256
	return service.cutFrom(ctx, nodeID, want.job, command, args, true)
}

// cutFrom has the node cut the sizes asked for from a source and stores them. Only the image's
// own source can say a size cannot be made; a size a base did not yield is an error, retried.
func (service *Service) cutFrom(
	ctx context.Context, nodeID int64, job jobs.Job, command nodeproto.Artwork, args artworkJobArgs, isFromSource bool,
) error {
	result, err := service.reEncode(ctx, nodeID, job, command)
	if err != nil {
		return err
	}
	if command.AbsolutePath != nil && result.Refusal == "" && result.SourceSha256 != args.SourceSha256 {
		service.parts.Logger.WarnContext(ctx, "an image beside the media changed; its sizes are not made",
			"sourceSha256", args.SourceSha256)
		return service.noteCannotMake(ctx, args.SourceSha256, args.SizeNames)
	}
	var unmade []nodeproto.ArtSizeName
	for _, size := range args.SizeNames {
		madeIndex := slices.IndexFunc(result.Images,
			func(image nodeproto.ArtImage) bool { return image.SizeName == size })
		if madeIndex < 0 || !isWebP(result.Images[madeIndex].WebP) {
			unmade = append(unmade, size)
			continue
		}
		if err := service.writeArt(ctx, args.SourceSha256, string(size)+webpSuffix,
			result.Images[madeIndex].WebP); err != nil {
			return err
		}
	}
	if len(unmade) > 0 && !isFromSource {
		return fmt.Errorf("the node cut no %v from art %s's base", unmade, args.SourceSha256)
	}
	return service.noteCannotMake(ctx, args.SourceSha256, unmade)
}

// baseNameFor is the held WebP an image's sizes can be cut from: its largest, when it is at least
// as large as every size asked for.
func (service *Service) baseNameFor(sourceSha256Hex string, sizes []nodeproto.ArtSizeName) (string, bool) {
	held := service.holdings
	held.mutex.Lock()
	base := baseNameOf(held.sizeBytesByNameBySourceSha256[sourceSha256Hex])
	held.mutex.Unlock()
	baseSize, known := nodeproto.ArtSizeName(strings.TrimSuffix(base, webpSuffix)).Size()
	if !known {
		return "", false
	}
	for _, name := range sizes {
		size, sized := name.Size()
		if !sized || size.Width > baseSize.Width {
			return "", false
		}
	}
	return base, true
}

// noteCannotMake notes sizes an image cannot be made at, as empty .none files beside its sizes.
func (service *Service) noteCannotMake(
	ctx context.Context, sourceSha256Hex string, sizes []nodeproto.ArtSizeName,
) error {
	for _, size := range sizes {
		if err := service.writeArt(ctx, sourceSha256Hex, string(size)+cannotMakeSuffix, nil); err != nil {
			return err
		}
	}
	return nil
}

// sizesTooLarge are the sizes of a shape a fetched source is too small for: the smallest the node did
// not make at fetch time, and every larger size, made at fetch time or on request.
func sizesTooLarge(shape nodeproto.Shape, made []nodeproto.ArtImage) []nodeproto.ArtSizeName {
	var unmade []nodeproto.ArtSizeName
	for _, size := range shape.Sizes() {
		isMade := slices.ContainsFunc(made, func(image nodeproto.ArtImage) bool { return image.SizeName == size.Name })
		if len(unmade) > 0 || (!isMade && !size.IsMadeOnRequest) {
			if !isMade {
				unmade = append(unmade, size.Name)
			}
		}
	}
	return unmade
}

// queueMake queues sizes of a stored image to be made, for the item that shows it, if one does
// and the sizes are of its shape.
func (service *Service) queueMake(ctx context.Context, sourceSha256Hex string, wantShape nodeproto.Shape,
	sizes []nodeproto.ArtSizeName,
) error {
	sourceSha256, err := hex.DecodeString(sourceSha256Hex)
	if err != nil {
		return ErrNoArt
	}
	origin, err := store.OrNotFound(service.parts.Database.Read().GetArtworkSource(ctx, sourceSha256))
	if errors.Is(err, store.ErrNotFound) {
		return nil // an image no item shows any more
	}
	if err != nil {
		return fmt.Errorf("find where art %s came from: %w", sourceSha256Hex, err)
	}
	if shape, known := shapeOf(origin.Kind); !known || shape != wantShape {
		return nil
	}
	args, err := json.Marshal(artworkJobArgs{
		ArtworkSource: origin.Source, Refs: []Ref{{Kind: origin.Kind, SourceRef: *origin.SourceRef}},
		SourceSha256: sourceSha256Hex,
		SizeNames:    sizes,
	})
	if err != nil {
		return fmt.Errorf("encode the job: %w", err)
	}
	_, err = service.parts.Queue.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindMakeArtworkSizes, Target: jobs.Target{ItemID: origin.ItemID}, ArgsJSON: string(args),
		Priority: makePriority, Destination: service.destinationOf(origin.Source),
		Reason: fmt.Sprintf("sizes someone asked for: %v", sizes),
	})
	if err != nil {
		return fmt.Errorf("queue %v of art %s: %w", sizes, sourceSha256Hex, err)
	}
	return nil
}
