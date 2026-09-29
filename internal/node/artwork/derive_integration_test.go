// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package artwork

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// generateSourceImage makes an image with the pinned FFmpeg: a test pattern of the size, in the codec.
func generateSourceImage(t *testing.T, runner *execx.Runner, size, codec, format string) []byte {
	t.Helper()
	image, err := runner.Run(t.Context(), execx.FFmpeg, confine.Access{}, []string{
		"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=" + size + ":duration=1", "-frames:v", "1",
		"-c:v", codec, "-f", format, "pipe:1",
	}, 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	return image
}

// withComment puts a JPEG comment segment (COM, FF FE) right after the start-of-image marker,
// where every decoder reads past it: metadata the output must not carry.
func withComment(t *testing.T, jpeg []byte, comment string) []byte {
	t.Helper()
	if len(jpeg) < 2 || jpeg[0] != 0xFF || jpeg[1] != 0xD8 {
		t.Fatalf("not a JPEG: % x", jpeg[:min(len(jpeg), 4)])
	}
	length := uint16(len(comment) + 2) //nolint:gosec // a short test comment
	segment := append([]byte{0xFF, 0xFE}, binary.BigEndian.AppendUint16(nil, length)...)
	segment = append(segment, comment...)
	return append(append(append([]byte{}, jpeg[:2]...), segment...), jpeg[2:]...)
}

// imageOf is the size's bytes in a result, or nil.
func imageOf(result nodeproto.ArtworkResult, size nodeproto.ArtSizeName) []byte {
	for _, image := range result.Images {
		if image.SizeName == size {
			return image.WebP
		}
	}
	return nil
}

func newDeriver(t *testing.T) (*Deriver, *execx.Runner) {
	t.Helper()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	return NewDeriver(runner, 30*time.Second), runner
}

// dimensionsOf reads an encoded size back with the pinned ffprobe.
func dimensionsOf(t *testing.T, deriver *Deriver, encoded []byte) (width, height int, codec string) {
	t.Helper()
	output, err := deriver.runner.RunInput(t.Context(), execx.FFprobe, confine.Access{}, []string{
		"-v", "error", "-protocol_whitelist", "pipe", "-show_entries", "stream=width,height,codec_name",
		"-of", "json", "pipe:0",
	}, encoded, 1<<16)
	if err != nil {
		t.Fatal(err)
	}
	var probed struct {
		Streams []struct {
			Width  int    `json:"width"`
			Height int    `json:"height"`
			Codec  string `json:"codec_name"` //nolint:tagliatelle // ffprobe's name
		} `json:"streams"`
	}
	if err := json.Unmarshal(output, &probed); err != nil || len(probed.Streams) != 1 {
		t.Fatalf("probe of the output: %s, %v", output, err)
	}
	return probed.Streams[0].Width, probed.Streams[0].Height, probed.Streams[0].Codec
}

func TestDerive_AJPEGPosterBecomesOurOwnWebPAtEverySizeItIsLargeEnoughFor(t *testing.T) {
	t.Parallel()
	deriver, runner := newDeriver(t)
	poster := withComment(t, generateSourceImage(t, runner, "900x1350", "mjpeg", "mjpeg"), "SECRET-COMMENT")
	result := deriver.Derive(t.Context(), nodeproto.Artwork{
		JobID: 7, SourceImage: poster, Shape: nodeproto.ShapePortrait, MaxSourcePixels: 40_000_000, WebPQuality: 82,
		SizeNames: []nodeproto.ArtSizeName{"portrait-s", "portrait-m", "portrait-l"},
	})
	if result.Refusal != "" || result.JobID != 7 || result.SourceWidth != 900 || result.SourceHeight != 1350 {
		t.Fatalf("result: refused %q, job %d, %d×%d", result.Refusal, result.JobID, result.SourceWidth,
			result.SourceHeight)
	}
	for _, want := range []struct {
		name          nodeproto.ArtSizeName
		width, height int
	}{{nodeproto.PortraitS, 240, 360}, {nodeproto.PortraitM, 480, 720}} {
		name := want.name
		width, height, codec := dimensionsOf(t, deriver, imageOf(result, name))
		if width != want.width || height != want.height || codec != "webp" {
			t.Errorf("%s: %d×%d %s, want %d×%d webp", name, width, height, codec, want.width, want.height)
		}
		if bytes.Contains(imageOf(result, name), []byte("SECRET-COMMENT")) {
			t.Errorf("%s carries the source's metadata", name)
		}
	}
	if imageOf(result, nodeproto.PortraitL) != nil {
		t.Error("portrait-l was made from a 900×1350 source: an upscale")
	}
}

func TestDerive_ALandscapeSourceIsCroppedToTheExactShape(t *testing.T) {
	t.Parallel()
	deriver, runner := newDeriver(t)
	// 2:1, wider than 16:9: its sides are cut, never its shape.
	backdrop := generateSourceImage(t, runner, "2000x1000", "png", "image2pipe")
	result := deriver.Derive(t.Context(), nodeproto.Artwork{
		SourceImage: backdrop, Shape: nodeproto.ShapeLandscape, MaxSourcePixels: 40_000_000, WebPQuality: 82,
		SizeNames: []nodeproto.ArtSizeName{"landscape-m"},
	})
	width, height, _ := dimensionsOf(t, deriver, imageOf(result, nodeproto.LandscapeM))
	if result.Refusal != "" || width != 960 || height != 540 {
		t.Errorf("landscape-m: %d×%d (refused %q), want 960×540", width, height, result.Refusal)
	}
}

func TestDerive_RefusesWhatIsNotAnImageOrTooLarge(t *testing.T) {
	t.Parallel()
	deriver, runner := newDeriver(t)
	notImage := deriver.Derive(t.Context(), nodeproto.Artwork{
		SourceImage: []byte("<html><script>alert(1)</script></html>"), Shape: nodeproto.ShapePortrait,
		MaxSourcePixels: 40_000_000, WebPQuality: 82, SizeNames: []nodeproto.ArtSizeName{"portrait-s"},
	})
	if notImage.Refusal == "" || len(notImage.Images) != 0 {
		t.Errorf("an HTML file: refused %q, %d images; want refused, none", notImage.Refusal, len(notImage.Images))
	}
	bomb := deriver.Derive(t.Context(), nodeproto.Artwork{
		SourceImage: generateSourceImage(t, runner, "2000x3000", "png", "image2pipe"), Shape: nodeproto.ShapePortrait,
		MaxSourcePixels: 1_000_000, WebPQuality: 82, SizeNames: []nodeproto.ArtSizeName{"portrait-s"},
	})
	if bomb.Refusal == "" || len(bomb.Images) != 0 {
		t.Errorf("6 megapixels against a 1 megapixel limit: refused %q, %d images", bomb.Refusal, len(bomb.Images))
	}
}
