// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"

	"golang.org/x/image/webp"

	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
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

// jpegQuality is what the core cuts JPEG at: a fallback for clients without WebP.
const jpegQuality = 85

// makePriority puts a size someone is waiting for ahead of fetching.
const makePriority = 10

// Served is an answer to a request for one size of one image: its bytes, or the size to go to
// meanwhile.
type Served struct {
	Bytes []byte
	// Redirect names the size and format to fetch instead, for now; the requested one is being
	// made, or cannot be.
	Redirect string
}

// Serve answers a request for one size of one image in a format. A stored one is read; a JPEG
// not yet cut is cut from the WebP of its size; a size not made is answered with the nearest one
// there, and one that is made only on request is queued to be made (D212 §3–5). The node is never
// on this path.
func (service *Service) Serve(ctx context.Context, hash string, name nodeproto.ArtSizeName, format string) (
	Served, error,
) {
	size, known := name.Size()
	if !known || (format != FormatWebP && format != FormatJPEG) {
		return Served{}, ErrNoArt
	}
	art := service.parts.Store
	stored, err := art.ReadArt(hash, string(name)+"."+format)
	if err == nil {
		return Served{Bytes: stored}, nil
	}
	if errors.Is(err, datadir.ErrBadArtName) {
		return Served{}, ErrNoArt
	}
	if !isMissing(err) {
		return Served{}, fmt.Errorf("read art %s %s: %w", hash, name, err)
	}
	if format == FormatJPEG {
		cut, cutErr := service.cutJPEG(ctx, art, hash, name)
		if cutErr == nil || !isMissing(cutErr) {
			return Served{Bytes: cut}, cutErr
		}
	}
	nearest, err := service.nearest(art, hash, size)
	if err != nil {
		return Served{}, err
	}
	if err := service.makeIfWanted(ctx, art, hash, size); err != nil {
		return Served{}, err
	}
	return Served{Redirect: string(nearest) + "." + format}, nil
}

// isMissing is a size the store does not have.
func isMissing(err error) bool {
	return errors.Is(err, datadir.ErrNotFound)
}

// cutJPEG decodes the WebP of a size — our own encoder's output, and nothing else (D212 §5) —
// and keeps it as JPEG.
func (service *Service) cutJPEG(ctx context.Context, art Store, hash string, name nodeproto.ArtSizeName) (
	[]byte, error,
) {
	source, err := art.ReadArt(hash, string(name)+"."+FormatWebP)
	if err != nil {
		return nil, err
	}
	config, err := webp.DecodeConfig(bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("read the size of art %s %s: %w", hash, name, err)
	}
	limit, err := service.parts.Settings.Int(ctx, settings.ArtworkMaxSourcePixels)
	if err != nil {
		return nil, fmt.Errorf("read artwork_max_source_pixels: %w", err)
	}
	if int64(config.Width)*int64(config.Height) > limit {
		return nil, fmt.Errorf("art %s %s is %d×%d, over the pixel limit", hash, name, config.Width, config.Height)
	}
	decoded, err := webp.Decode(bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("decode art %s %s: %w", hash, name, err)
	}
	var cut bytes.Buffer
	if err := jpeg.Encode(&cut, decoded, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("cut JPEG of art %s %s: %w", hash, name, err)
	}
	if err := art.WriteArt(hash, string(name)+"."+FormatJPEG, cut.Bytes()); err != nil {
		return nil, fmt.Errorf("keep JPEG of art %s %s: %w", hash, name, err)
	}
	return cut.Bytes(), nil
}

// nearest is the stored WebP size of the shape closest to the one asked for: the largest below
// it, else the smallest above it.
func (service *Service) nearest(art Store, hash string, want nodeproto.ArtSize) (nodeproto.ArtSizeName, error) {
	var below, above nodeproto.ArtSizeName
	for _, size := range want.Shape.Sizes() {
		if size.Name == want.Name {
			continue
		}
		found, err := art.HasArt(hash, string(size.Name)+"."+FormatWebP)
		if err != nil {
			return "", fmt.Errorf("look for art %s %s: %w", hash, size.Name, err)
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

// makeIfWanted queues a size made only on request, when its source may be large enough: the
// largest size made at fetch time is there, and no attempt found the source too small.
func (service *Service) makeIfWanted(ctx context.Context, art Store, hash string, want nodeproto.ArtSize) error {
	if !want.OnRequest {
		return nil // made at fetch time, when the source was large enough; it was not
	}
	fetched := fetchedSizes(want.Shape)
	largest := fetched[len(fetched)-1]
	for _, check := range []struct {
		name string
		want bool
	}{{string(largest) + "." + FormatWebP, true}, {string(want.Name) + ".none", false}} {
		found, err := art.HasArt(hash, check.name)
		if err != nil {
			return fmt.Errorf("look for art %s %s: %w", hash, check.name, err)
		}
		if found != check.want {
			return nil
		}
	}
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
	args, err := json.Marshal(owed{
		Source: origin.Source, Refs: []owedRef{{Kind: origin.Kind, Ref: *origin.SourceRef}},
		Hash: hash, Sizes: []nodeproto.ArtSizeName{want.Name},
	})
	if err != nil {
		return fmt.Errorf("encode the job: %w", err)
	}
	_, err = service.parts.Queue.Enqueue(ctx, jobs.New{
		Kind: jobs.KindArtwork, Subject: origin.ItemID, Args: string(args), Priority: makePriority,
		Reason: "a size someone asked for: " + string(want.Name),
	})
	if err != nil {
		return fmt.Errorf("queue %s of art %s: %w", want.Name, hash, err)
	}
	return nil
}
