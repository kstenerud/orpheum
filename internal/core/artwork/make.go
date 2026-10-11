// SPDX-License-Identifier: AGPL-3.0-or-later

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
