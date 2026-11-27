// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package artwork

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

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

func newDeriver(t *testing.T) *Deriver {
	t.Helper()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	return NewDeriver(runner, 30*time.Second)
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
	deriver := newDeriver(t)
	poster := withComment(t, testutil.GenerateImage(t, "900x1350", "mjpeg", "mjpeg"), "SECRET-COMMENT")
	result := deriver.Derive(t.Context(), nodeproto.Artwork{
		SourceImage: poster, Shape: nodeproto.ShapePortrait, MaxSourcePixels: 40_000_000, WebPQuality: 82,
		SizeNames: []nodeproto.ArtSizeName{"portrait-s", "portrait-m", "portrait-l"},
	})
	if result.Refusal != "" || result.SourceWidth != 900 || result.SourceHeight != 1350 {
		t.Fatalf("result: refused %q, %d×%d", result.Refusal, result.SourceWidth, result.SourceHeight)
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
	deriver := newDeriver(t)
	// 2:1, wider than 16:9: its sides are cut, never its shape.
	backdrop := testutil.GenerateImage(t, "2000x1000", "png", "image2pipe")
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
	deriver := newDeriver(t)
	notImage := deriver.Derive(t.Context(), nodeproto.Artwork{
		SourceImage: []byte("<html><script>alert(1)</script></html>"), Shape: nodeproto.ShapePortrait,
		MaxSourcePixels: 40_000_000, WebPQuality: 82, SizeNames: []nodeproto.ArtSizeName{"portrait-s"},
	})
	if notImage.Refusal == "" || len(notImage.Images) != 0 {
		t.Errorf("an HTML file: refused %q, %d images; want refused, none", notImage.Refusal, len(notImage.Images))
	}
	bomb := deriver.Derive(t.Context(), nodeproto.Artwork{
		SourceImage: testutil.GenerateImage(t, "2000x3000", "png", "image2pipe"), Shape: nodeproto.ShapePortrait,
		MaxSourcePixels: 1_000_000, WebPQuality: 82, SizeNames: []nodeproto.ArtSizeName{"portrait-s"},
	})
	if bomb.Refusal == "" || len(bomb.Images) != 0 {
		t.Errorf("6 megapixels against a 1 megapixel limit: refused %q, %d images", bomb.Refusal, len(bomb.Images))
	}
}

// generateVideo writes a video with the pinned FFmpeg into the test's directory: the filter graph's
// picture, as H.264 in Matroska, with a title tag the still must not carry.
func generateVideo(t *testing.T, name, graph string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	testutil.GenerateMedia(t, path, "-filter_complex", graph, "-map", "[v]", "-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-metadata", "title=SECRET-TITLE", "-f", "matroska")
	return path
}

// averageLuma is an encoded image's mean brightness, 0 to 255: FFmpeg scales it to one gray pixel.
func averageLuma(t *testing.T, runner *execx.Runner, encoded []byte) byte {
	t.Helper()
	pixel, err := runner.RunInput(t.Context(), execx.FFmpeg, confine.Access{}, append(append([]string{"-v", "error"},
		execx.PipeInput()...), "-vf", "scale=1:1:flags=area,format=gray", "-f", "rawvideo", "pipe:1"), encoded, 16)
	if err != nil || len(pixel) != 1 {
		t.Fatalf("the image's brightness: %v, %d bytes", err, len(pixel))
	}
	return pixel[0]
}

// A video's still, taken at a tenth of it inside a black lead-in, skips the black for the picture
// after it, and is re-encoded as any image is: our own WebP at the landscape sizes the frame is large
// enough for, with none of the video's metadata. A video black throughout has no frame (D362 §1, §5).
func TestStill_AVideosStillSkipsItsBlackLeadInAndBecomesOurOwnWebP(t *testing.T) {
	t.Parallel()
	deriver := newDeriver(t)
	video := generateVideo(t, "lead-in.mkv", "color=c=black:s=1280x720:r=25:d=4[b];"+
		"testsrc2=s=1280x720:r=25:d=8[p];[b][p]concat=n=2:v=1:a=0[v]")
	// 10% of its 12 s is 1.2 s: 2.8 s of black before the picture.
	still, err := deriver.Still(t.Context(), execx.Film{Path: video}, 0, 1200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	result := deriver.Derive(t.Context(), nodeproto.Artwork{
		SourceImage: still, Shape: nodeproto.ShapeLandscape, MaxSourcePixels: 40_000_000, WebPQuality: 82,
		SizeNames: []nodeproto.ArtSizeName{nodeproto.LandscapeS, nodeproto.LandscapeM, nodeproto.LandscapeL},
	})
	if result.Refusal != "" || result.SourceWidth != 1280 || result.SourceHeight != 720 {
		t.Fatalf("the still re-encoded: refused %q, %d×%d; want its 1280×720 frame", result.Refusal,
			result.SourceWidth, result.SourceHeight)
	}
	small := imageOf(result, nodeproto.LandscapeS)
	if width, height, codec := dimensionsOf(t, deriver, small); width != 480 || height != 270 || codec != "webp" {
		t.Errorf("landscape-s: %d×%d %s, want 480×270 webp", width, height, codec)
	}
	if luma := averageLuma(t, deriver.runner, small); luma <= 32 {
		t.Errorf("the still's brightness is %d of 255: a black frame of the lead-in, not the picture after it", luma)
	}
	if imageOf(result, nodeproto.LandscapeL) != nil {
		t.Error("landscape-l was made from a 1280×720 frame: an upscale")
	}
	if bytes.Contains(still, []byte("SECRET-TITLE")) || bytes.Contains(small, []byte("SECRET-TITLE")) {
		t.Error("the still carries the video's metadata")
	}
	black := generateVideo(t, "black.mkv", "color=c=black:s=640x360:r=25:d=6[v]")
	if still, err := deriver.Still(t.Context(), execx.Film{Path: black}, 0, 600*time.Millisecond); !errors.Is(err,
		ErrNoFrame) {
		t.Errorf("a video black throughout: %d bytes, %v; want ErrNoFrame", len(still), err)
	}
}
