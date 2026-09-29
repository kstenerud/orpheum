// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// maxEncodedBytes bounds one encoded size: a 3840×2160 WebP is a few megabytes at most.
const maxEncodedBytes = 32 << 20

// maxProbeBytes bounds ffprobe's answer about one image.
const maxProbeBytes = 64 << 10

// Deriver re-encodes images with the bundled FFmpeg, one run at a time, each bounded by the
// timeout (D212 §1).
type Deriver struct {
	runner        *execx.Runner
	perRunTimeout time.Duration
}

// NewDeriver returns the deriver.
func NewDeriver(runner *execx.Runner, timeout time.Duration) *Deriver {
	return &Deriver{runner: runner, perRunTimeout: timeout}
}

// Derive answers an artwork command: every size it asked for that the source is large enough
// for, each our own encode, or why none was made. The source is anyone's bytes; it reaches
// FFmpeg on a pipe, never as a name, and only the pipe protocol is allowed.
func (deriver *Deriver) Derive(ctx context.Context, command nodeproto.Artwork) nodeproto.ArtworkResult {
	result := nodeproto.ArtworkResult{JobID: command.JobID, Images: []nodeproto.ArtImage{}}
	width, height, err := deriver.dimensions(ctx, command.SourceImage)
	switch {
	case err != nil:
		result.Refusal = "not an image FFmpeg reads: " + err.Error()
		return result
	case int64(width)*int64(height) > command.MaxSourcePixels:
		result.Refusal = fmt.Sprintf("%d×%d is over the limit of %d pixels", width, height, command.MaxSourcePixels)
		return result
	}
	result.SourceWidth, result.SourceHeight = width, height
	for _, size := range plan(command.Shape, command.SizeNames, width, height) {
		encoded, err := deriver.encode(ctx, command.SourceImage, size.filter, command.WebPQuality)
		if err != nil {
			result.Images = []nodeproto.ArtImage{}
			result.Refusal = fmt.Sprintf("encoding %s: %v", size.name, err)
			return result
		}
		result.Images = append(result.Images, nodeproto.ArtImage{SizeName: size.name, WebP: encoded})
	}
	return result
}

// dimensions are the first video stream's, read from the header on the pipe.
func (deriver *Deriver) dimensions(ctx context.Context, source []byte) (width, height int, err error) {
	ctx, cancel := context.WithTimeout(ctx, deriver.perRunTimeout)
	defer cancel()
	output, runErr := deriver.runner.RunInput(ctx, execx.FFprobe, confine.Access{}, []string{
		"-v", "error", "-protocol_whitelist", "pipe", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "json", "pipe:0",
	}, source, maxProbeBytes)
	if runErr != nil {
		return 0, 0, fmt.Errorf("probe: %w", runErr)
	}
	var probed struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	if decodeErr := json.Unmarshal(output, &probed); decodeErr != nil {
		return 0, 0, fmt.Errorf("read the probe: %w", decodeErr)
	}
	if len(probed.Streams) == 0 || probed.Streams[0].Width <= 0 || probed.Streams[0].Height <= 0 {
		return 0, 0, errors.New("no picture in it")
	}
	return probed.Streams[0].Width, probed.Streams[0].Height, nil
}

// encode makes one size: the first frame only, every metadata block dropped, our WebP out.
func (deriver *Deriver) encode(ctx context.Context, source []byte, filter string, quality int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, deriver.perRunTimeout)
	defer cancel()
	encoded, err := deriver.runner.RunInput(ctx, execx.FFmpeg, confine.Access{},
		append(append([]string{"-v", "error"}, execx.PipeInput()...),
			"-frames:v", "1", "-map_metadata", "-1", "-vf", filter,
			"-c:v", "libwebp", "-lossless", "0", "-quality", strconv.Itoa(quality), "-f", "webp", "pipe:1"),
		source, maxEncodedBytes)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return encoded, nil
}

// plannedSize is one size to make, and the filter that makes it.
type plannedSize struct {
	name   nodeproto.ArtSizeName
	filter string
}

// plan chooses what to make of a source: the sizes asked for, of the command's shape, that the
// source is large enough for — never an upscale — and always the shape's smallest, so every
// image has one size, cut without scaling up when the source is smaller than it.
func plan(shape nodeproto.Shape, names []nodeproto.ArtSizeName, width, height int) []plannedSize {
	var sizes []plannedSize
	isNextSmallest := true
	for _, size := range shape.Sizes() {
		isShapeSmallest := isNextSmallest
		isNextSmallest = false
		if !slices.Contains(names, size.Name) {
			continue
		}
		isLargeEnough := width >= size.Width && height >= size.Height
		switch {
		case shape == nodeproto.ShapeLogo:
			if isLargeEnough || isShapeSmallest {
				sizes = append(sizes, plannedSize{name: size.Name, filter: fitWithin(size)})
			}
		case isLargeEnough:
			sizes = append(sizes, plannedSize{name: size.Name, filter: coverCrop(size)})
		case isShapeSmallest:
			sizes = append(sizes, plannedSize{name: size.Name, filter: cropToShape(size)})
		}
	}
	return sizes
}

// coverCrop scales a source down until it covers the size, and crops the overhang: the exact
// shape (D212 §3).
func coverCrop(size nodeproto.ArtSize) string {
	return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase:flags=lanczos,crop=%d:%d",
		size.Width, size.Height, size.Width, size.Height)
}

// cropToShape crops a source smaller than the size to the size's shape, without scaling it.
func cropToShape(size nodeproto.ArtSize) string {
	return fmt.Sprintf("crop='min(iw,ih*%d/%d)':'min(ih,iw*%d/%d)'", size.Width, size.Height, size.Height,
		size.Width)
}

// fitWithin scales a logo down to fit the box, keeping its own shape and never scaling up.
func fitWithin(size nodeproto.ArtSize) string {
	return fmt.Sprintf("scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease:flags=lanczos",
		size.Width, size.Height)
}
