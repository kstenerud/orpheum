// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// Shape is an artwork size's shape (D212): the frame an image is cropped to, or for a logo, the
// box it fits in.
type Shape string

// The shapes.
const (
	ShapePortrait  Shape = "portrait"  // 2:3: posters, people
	ShapeLandscape Shape = "landscape" // 16:9: backdrops, stills
	ShapeLogo      Shape = "logo"      // transparent, kept at its own shape within a box
)

// ArtSizeName names one of the closed set of sizes (D212, artwork.md): a URL names one, and no
// request can ask for any other.
type ArtSizeName string

// The sizes, smallest first within each shape (artwork-sizes.md).
const (
	PortraitS   ArtSizeName = "portrait-s"
	PortraitM   ArtSizeName = "portrait-m"
	PortraitL   ArtSizeName = "portrait-l"
	LandscapeS  ArtSizeName = "landscape-s"
	LandscapeM  ArtSizeName = "landscape-m"
	LandscapeL  ArtSizeName = "landscape-l"
	LandscapeXL ArtSizeName = "landscape-xl"
	LogoS       ArtSizeName = "logo-s"
	LogoM       ArtSizeName = "logo-m"
)

// ArtSize is one size: its shape, its pixels, and whether it is made only when asked for.
type ArtSize struct {
	Name   ArtSizeName
	Shape  Shape
	Width  int
	Height int
	// IsMadeOnRequest says the size is made when first asked for, not when the image is fetched.
	IsMadeOnRequest bool
}

// Sizes are the shape's sizes, smallest first.
func (shape Shape) Sizes() []ArtSize {
	switch shape {
	case ShapePortrait:
		return []ArtSize{
			{Name: PortraitS, Shape: shape, Width: 240, Height: 360},
			{Name: PortraitM, Shape: shape, Width: 480, Height: 720},
			{Name: PortraitL, Shape: shape, Width: 960, Height: 1440, IsMadeOnRequest: true},
		}
	case ShapeLandscape:
		return []ArtSize{
			{Name: LandscapeS, Shape: shape, Width: 480, Height: 270},
			{Name: LandscapeM, Shape: shape, Width: 960, Height: 540},
			{Name: LandscapeL, Shape: shape, Width: 1920, Height: 1080},
			{Name: LandscapeXL, Shape: shape, Width: 3840, Height: 2160, IsMadeOnRequest: true},
		}
	case ShapeLogo:
		return []ArtSize{
			{Name: LogoS, Shape: shape, Width: 400, Height: 155},
			{Name: LogoM, Shape: shape, Width: 800, Height: 310},
		}
	}
	return nil
}

// Size is the size a name names, and whether it names one.
func (name ArtSizeName) Size() (ArtSize, bool) {
	for _, shape := range []Shape{ShapePortrait, ShapeLandscape, ShapeLogo} {
		for _, size := range shape.Sizes() {
			if size.Name == name {
				return size, true
			}
		}
	}
	return ArtSize{}, false
}

// CommandEncodeArtwork asks the node to re-encode the images of one artwork job (D212 §1), each in
// turn, and answer once for all.
const CommandEncodeArtwork CommandType = "encode_artwork"

// ArtworkBatch is one artwork job's images, every one the job still wants, as a scan is one root's
// files: the core has fetched what its providers hold, and the node re-encodes each.
type ArtworkBatch struct {
	Images []Artwork `json:"images"`
}

// ArtworkBatchResult is the node's answer for an ArtworkBatch: one result for each image, each
// naming its image's Key.
type ArtworkBatchResult struct {
	Images []ArtworkResult `json:"images"`
}

// Artwork is a source image the node sanitises and cuts into sizes. The source is anyone's
// bytes — a provider's download, a file beside the media — and is decoded only here, under
// FFmpeg's confinement.
type Artwork struct {
	// Key names the image to the core among its batch's, and its answer echoes it.
	Key         string `json:"key"`
	SourceImage []byte `json:"sourceImage,omitempty"`
	// SourceSha256 is, with SourceImage, the hex SHA-256 of the image the sizes are of, as the core
	// stores them: its source's, also when SourceImage is a size already made of it (D212 §2). It is
	// what an answer echoes in SourceImage's place (Result.Command), and the sizes are recorded under.
	SourceSha256 string `json:"sourceSha256,omitempty"`
	// AbsolutePath, in place of SourceImage, names an image beside the media for the node to read
	// itself, under one of its roots, so its bytes never reach the core (D215). Unlike every other
	// Path on the wire it is absolute: the node finds the root it is under.
	AbsolutePath *Path `json:"absolutePath,omitempty"`
	// Video, in place of SourceImage and AbsolutePath, is a video to take a frame still from
	// (D362): the node reads it as it reads a film for any run (D330), checked to be the file
	// named, and the image it re-encodes is the most representative frame of a stretch of the
	// picture stream at FrameStreamIndex from FrameAtMs, black frames skipped.
	Video            *BytesFile `json:"video,omitempty"`
	FrameStreamIndex int        `json:"frameStreamIndex,omitempty"`
	FrameAtMs        int64      `json:"frameAtMs,omitempty"`
	Shape            Shape      `json:"shape"`
	// SizeNames are the names to make; one larger than the source is not made.
	SizeNames []ArtSizeName `json:"sizeNames"`
	// MaxSourcePixels refuses a source larger than this before it is decoded: a decompression bomb.
	MaxSourcePixels int64 `json:"maxSourcePixels"`
	// WebPQuality is libwebp's, 0 to 100.
	WebPQuality int `json:"webpQuality"`
}

// ArtImage is one size the node made: our own encoder's WebP.
type ArtImage struct {
	SizeName ArtSizeName `json:"sizeName"`
	WebP     []byte      `json:"webp"`
}

// ArtworkResult is the node's answer: every size it made, as our own encoder wrote it, or why it
// made none.
type ArtworkResult struct {
	// Key is the image's (Artwork.Key), echoed.
	Key          string     `json:"key"`
	SourceWidth  int        `json:"sourceWidth"`
	SourceHeight int        `json:"sourceHeight"`
	Images       []ArtImage `json:"images"`
	// SourceSha256 is the SHA-256, in hex, of the file an AbsolutePath command named, as the node read it,
	// or of the still it took from a Video: what the core stores the sizes under.
	SourceSha256 string `json:"sourceSha256,omitempty"`
	// Refusal says why no size was made: over the pixel limit, not an image, a failed encode.
	Refusal string `json:"refusal,omitempty"`
	// HasNoFrame, with Refusal, is a Video whose stretch holds no picture that is not black: the
	// video's own answer, which the core keeps and does not ask again (D362 §5).
	HasNoFrame bool `json:"hasNoFrame,omitempty"`
}
