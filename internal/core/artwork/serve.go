// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/jpeg"

	"golang.org/x/image/webp"

	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// The formats a size is served in (D212 §4): WebP always, JPEG cut by the core when first asked
// for.
const (
	FormatWebP = "webp"
	FormatJPEG = "jpeg"
)

// Refusals a request for an image can meet.
var (
	// ErrNoArt is an image this server has no size of, or a name that is not one of ours.
	ErrNoArt = errors.New("no such image")
)

// makePriority puts a size someone is waiting for ahead of fetching.
const makePriority = 10

// Served is an answer to a request for one size of one image: its bytes, or the size to go to
// meanwhile.
type Served struct {
	Bytes []byte
	// RedirectName names the size and format to fetch instead, for now; the requested one is being
	// made, or cannot be.
	RedirectName string
}

// Serve answers a request for one size of one image in a format. A stored one is read; a JPEG
// not yet cut is cut from the WebP of its size; a size not made is answered with the nearest one
// there, and one that is made only on request is queued to be made (D212 §3–5). The node is never
// on this path.
func (service *Service) Serve(ctx context.Context, sourceSha256Hex string, name nodeproto.ArtSizeName, format string) (
	Served, error,
) {
	size, known := name.Size()
	if !known || (format != FormatWebP && format != FormatJPEG) {
		return Served{}, ErrNoArt
	}
	art := service.parts.ArtFiles
	stored, err := art.ReadArt(sourceSha256Hex, string(name)+"."+format)
	if err == nil {
		service.noteServed(sourceSha256Hex)
		return Served{Bytes: stored}, nil
	}
	if errors.Is(err, instancedir.ErrBadArtName) {
		return Served{}, ErrNoArt
	}
	if !isMissing(err) {
		return Served{}, fmt.Errorf("read art %s %s: %w", sourceSha256Hex, name, err)
	}
	if format == FormatJPEG {
		cut, cutErr := service.cutJPEG(ctx, art, sourceSha256Hex, name)
		if cutErr == nil {
			service.noteServed(sourceSha256Hex)
		}
		if cutErr == nil || !isMissing(cutErr) {
			return Served{Bytes: cut}, cutErr
		}
	}
	return service.serveNearest(ctx, sourceSha256Hex, size, format)
}

// serveNearest answers for a size not held: a redirect to the nearest size held, with the size
// queued to be made; or, with nothing of the image held — the cache was deleted — no image, and
// the image queued to be made again, if an item shows it.
func (service *Service) serveNearest(
	ctx context.Context, sourceSha256Hex string, size nodeproto.ArtSize, format string,
) (Served, error) {
	art := service.parts.ArtFiles
	nearest, err := service.nearest(art, sourceSha256Hex, size)
	if errors.Is(err, ErrNoArt) {
		if makeErr := service.queueMake(ctx, sourceSha256Hex, size.Shape, fetchedSizes(size.Shape)); makeErr != nil &&
			!errors.Is(makeErr, ErrNoArt) {
			return Served{}, makeErr
		}
		return Served{}, ErrNoArt
	}
	if err != nil {
		return Served{}, err
	}
	if err := service.queueMakeUnlessNoted(ctx, art, sourceSha256Hex, size); err != nil {
		return Served{}, err
	}
	return Served{RedirectName: string(nearest) + "." + format}, nil
}

// isMissing is a size the store does not have.
func isMissing(err error) bool {
	return errors.Is(err, instancedir.ErrNotFound)
}

// cutJPEG decodes the WebP of a size — our own encoder's output, and nothing else (D212 §5) —
// and keeps it as JPEG.
func (service *Service) cutJPEG(ctx context.Context, art ArtFiles, sourceSha256Hex string, name nodeproto.ArtSizeName) (
	[]byte, error,
) {
	source, err := art.ReadArt(sourceSha256Hex, string(name)+"."+FormatWebP)
	if err != nil {
		return nil, err
	}
	config, err := webp.DecodeConfig(bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("read the size of art %s %s: %w", sourceSha256Hex, name, err)
	}
	maxSourcePixels, err := service.parts.Settings.Int(ctx, settings.ArtworkMaxSourcePixels)
	if err != nil {
		return nil, fmt.Errorf("read artwork_max_source_pixels: %w", err)
	}
	if int64(config.Width)*int64(config.Height) > maxSourcePixels {
		return nil, fmt.Errorf("art %s %s is %d×%d, over the pixel limit", sourceSha256Hex, name, config.Width,
			config.Height)
	}
	quality, err := service.parts.Settings.Int(ctx, settings.ArtworkJPEGQuality)
	if err != nil {
		return nil, fmt.Errorf("read artwork_jpeg_quality: %w", err)
	}
	decoded, err := webp.Decode(bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("decode art %s %s: %w", sourceSha256Hex, name, err)
	}
	var cut bytes.Buffer
	if err := jpeg.Encode(&cut, decoded, &jpeg.Options{Quality: int(quality)}); err != nil {
		return nil, fmt.Errorf("cut JPEG of art %s %s: %w", sourceSha256Hex, name, err)
	}
	if err := service.writeArt(ctx, sourceSha256Hex, string(name)+"."+FormatJPEG, cut.Bytes()); err != nil {
		return nil, err
	}
	return cut.Bytes(), nil
}

// nearest is the stored WebP size of the shape closest to the one asked for: the largest below
// it, else the smallest above it.
func (service *Service) nearest(
	art ArtFiles, sourceSha256Hex string, want nodeproto.ArtSize,
) (nodeproto.ArtSizeName, error) {
	var below, above nodeproto.ArtSizeName
	for _, size := range want.Shape.Sizes() {
		if size.Name == want.Name {
			continue
		}
		found, err := art.HasArt(sourceSha256Hex, string(size.Name)+"."+FormatWebP)
		if err != nil {
			return "", fmt.Errorf("look for art %s %s: %w", sourceSha256Hex, size.Name, err)
		}
		switch {
		case !found:
		case size.Width < want.Width:
			below = size.Name
		case above == "":
			above = size.Name
		}
	}
	switch {
	case below != "":
		return below, nil
	case above != "":
		return above, nil
	}
	return "", ErrNoArt
}

// queueMakeUnlessNoted queues a size not held to be made, unless it is noted as one the source cannot
// make: a size not held and not noted was evicted, or is made only on request.
func (service *Service) queueMakeUnlessNoted(
	ctx context.Context, art ArtFiles, sourceSha256Hex string, want nodeproto.ArtSize,
) error {
	noted, err := art.HasArt(sourceSha256Hex, string(want.Name)+cannotMakeSuffix)
	if err != nil {
		return fmt.Errorf("look for art %s %s: %w", sourceSha256Hex, want.Name, err)
	}
	if noted {
		return nil
	}
	return service.queueMake(ctx, sourceSha256Hex, want.Shape, []nodeproto.ArtSizeName{want.Name})
}
