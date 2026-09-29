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

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
)

// makeSizes makes sizes of a stored image: cut by the node from its base when the base is large
// enough — our own WebP, so nothing is fetched (D212 §2) — else from its source, fetched again.
// A size the source is too small for, or a source that is gone or changed, is noted so no
// request asks again.
func (service *Service) makeSizes(ctx context.Context, fetcher Fetcher, image wanted, args owed) error {
	shape, known := shapeOf(image.ref.Kind)
	if !known {
		return nil
	}
	command := nodeproto.Artwork{Shape: shape, SizeNames: args.SizeNames}
	if base, found := service.baseFor(args.ImageHash, args.SizeNames); found {
		stored, err := service.parts.Store.ReadArt(args.ImageHash, base)
		if err == nil {
			nodeID, nodeErr := service.anyNode()
			if nodeErr != nil {
				return nodeErr
			}
			command.SourceImage = stored
			return service.cutFrom(ctx, nodeID, command, args, false)
		}
	}
	if image.source == identify.SourceLocal {
		nodeID, named, err := service.localFile(ctx, image.ref.Ref)
		if errors.Is(err, ErrNoImage) {
			return service.cannotMake(ctx, args.ImageHash, args.SizeNames)
		}
		if err != nil {
			return err
		}
		command.AbsolutePath = named
		return service.cutFrom(ctx, nodeID, command, args, true)
	}
	source, err := service.fetch(ctx, fetcher, image.source, image.ref.Ref)
	if errors.Is(err, ErrNoImage) || (err == nil && source.hash() != args.ImageHash) {
		service.parts.Logger.WarnContext(ctx, "an image's source is gone or changed; its sizes are not made",
			"hash", args.ImageHash, "source", image.source, "ref", image.ref.Ref)
		return service.cannotMake(ctx, args.ImageHash, args.SizeNames)
	}
	if err != nil {
		return err
	}
	nodeID, err := service.anyNode()
	if err != nil {
		return err
	}
	command.SourceImage = source.bytes
	return service.cutFrom(ctx, nodeID, command, args, true)
}

// cutFrom has the node cut the sizes asked for from a source and stores them. Only the image's
// own source can say a size cannot be made; a size a base did not yield is an error, retried.
func (service *Service) cutFrom(
	ctx context.Context, nodeID int64, command nodeproto.Artwork, args owed, fromSource bool,
) error {
	result, err := service.reEncode(ctx, nodeID, command)
	if err != nil {
		return err
	}
	if command.AbsolutePath != nil && result.Refusal == "" && result.SourceSha256 != args.ImageHash {
		service.parts.Logger.WarnContext(ctx, "an image beside the media changed; its sizes are not made",
			"hash", args.ImageHash)
		return service.cannotMake(ctx, args.ImageHash, args.SizeNames)
	}
	var unmade []nodeproto.ArtSizeName
	for _, size := range args.SizeNames {
		made := slices.IndexFunc(result.Images, func(image nodeproto.ArtImage) bool { return image.SizeName == size })
		if made < 0 || !isWebP(result.Images[made].WebP) {
			unmade = append(unmade, size)
			continue
		}
		if err := service.writeArt(ctx, args.ImageHash, string(size)+webpFormat, result.Images[made].WebP); err != nil {
			return err
		}
	}
	if len(unmade) > 0 && !fromSource {
		return fmt.Errorf("the node cut no %v from art %s's base", unmade, args.ImageHash)
	}
	return service.cannotMake(ctx, args.ImageHash, unmade)
}

// baseFor is the held WebP an image's sizes can be cut from: its largest, when it is at least
// as large as every size asked for.
func (service *Service) baseFor(hash string, sizes []nodeproto.ArtSizeName) (string, bool) {
	held := service.holdings
	held.mutex.Lock()
	base := baseOf(held.files[hash])
	held.mutex.Unlock()
	baseSize, known := nodeproto.ArtSizeName(strings.TrimSuffix(base, webpFormat)).Size()
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

// cannotMake notes sizes an image cannot be made at, as empty .none files beside its sizes.
func (service *Service) cannotMake(ctx context.Context, hash string, sizes []nodeproto.ArtSizeName) error {
	for _, size := range sizes {
		if err := service.writeArt(ctx, hash, string(size)+noneMark, nil); err != nil {
			return err
		}
	}
	return nil
}

// tooLarge are the sizes of a shape a fetched source is too small for: the smallest the node did
// not make at fetch time, and every larger size, made at fetch time or on request.
func tooLarge(shape nodeproto.Shape, made []nodeproto.ArtImage) []nodeproto.ArtSizeName {
	var unmade []nodeproto.ArtSizeName
	for _, size := range shape.Sizes() {
		isMade := slices.ContainsFunc(made, func(image nodeproto.ArtImage) bool { return image.SizeName == size.Name })
		if len(unmade) > 0 || (!isMade && !size.OnRequest) {
			if !isMade {
				unmade = append(unmade, size.Name)
			}
		}
	}
	return unmade
}

// queueMake queues sizes of a stored image to be made, for the item that shows it, if one does
// and the sizes are of its shape.
func (service *Service) queueMake(ctx context.Context, hash string, want nodeproto.Shape,
	sizes []nodeproto.ArtSizeName,
) error {
	raw, err := hex.DecodeString(hash)
	if err != nil {
		return ErrNoArt
	}
	origin, err := store.One(service.parts.Database.Read().GetArtworkSource(ctx, raw))
	if errors.Is(err, store.ErrNotFound) {
		return nil // an image no item shows any more
	}
	if err != nil {
		return fmt.Errorf("find where art %s came from: %w", hash, err)
	}
	if shape, known := shapeOf(origin.Kind); !known || shape != want {
		return nil
	}
	args, err := json.Marshal(owed{
		FollowedSource: origin.Source, Refs: []owedRef{{Kind: origin.Kind, Ref: *origin.SourceRef}}, ImageHash: hash,
		SizeNames: sizes,
	})
	if err != nil {
		return fmt.Errorf("encode the job: %w", err)
	}
	_, err = service.parts.Queue.Enqueue(ctx, jobs.New{
		Kind: jobs.KindArtwork, Target: jobs.Target{ItemID: origin.ItemID}, Args: string(args), Priority: makePriority,
		Reason: fmt.Sprintf("sizes someone asked for: %v", sizes),
	})
	if err != nil {
		return fmt.Errorf("queue %v of art %s: %w", sizes, hash, err)
	}
	return nil
}
