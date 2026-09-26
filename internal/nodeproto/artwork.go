// SPDX-License-Identifier: GPL-3.0-or-later

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
	// OnRequest sizes are made when first asked for, not when the image is fetched.
	OnRequest bool
}

// Sizes are the shape's sizes, smallest first.
func (shape Shape) Sizes() []ArtSize {
	switch shape {
	case ShapePortrait:
		return []ArtSize{
			{Name: PortraitS, Shape: shape, Width: 240, Height: 360},
			{Name: PortraitM, Shape: shape, Width: 480, Height: 720},
			{Name: PortraitL, Shape: shape, Width: 960, Height: 1440, OnRequest: true},
		}
	case ShapeLandscape:
		return []ArtSize{
			{Name: LandscapeS, Shape: shape, Width: 480, Height: 270},
			{Name: LandscapeM, Shape: shape, Width: 960, Height: 540},
			{Name: LandscapeL, Shape: shape, Width: 1920, Height: 1080},
			{Name: LandscapeXL, Shape: shape, Width: 3840, Height: 2160, OnRequest: true},
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

// CommandArtwork asks the node to re-encode an image (D212 §1).
const CommandArtwork CommandType = "artwork"

// Artwork is a source image the node sanitises and cuts into sizes. The source is anyone's
// bytes — a provider's download, a file beside the media — and is decoded only here, under
// FFmpeg's confinement.
type Artwork struct {
	// Job names the core's job the answer belongs to.
	Job    int64  `json:"job"`
	Source []byte `json:"source,omitempty"`
	// AbsolutePath, in place of Source, names an image beside the media for the node to read
	// itself, under one of its roots, so its bytes never reach the core (D215). Unlike every other
	// Path on the wire it is absolute: the node finds the root it is under.
	AbsolutePath *Path `json:"absolutePath,omitempty"`
	Shape        Shape `json:"shape"`
	// Sizes are the names to make; one larger than the source is not made.
	Sizes []ArtSizeName `json:"sizes"`
	// MaxPixels refuses a source larger than this before it is decoded: a decompression bomb.
	MaxPixels int64 `json:"maxPixels"`
	// Quality is libwebp's, 0 to 100.
	Quality int `json:"quality"`
}

// PathArtwork is where the node posts an artwork command's answer.
const PathArtwork = "/node/v1/artwork"

// ArtImage is one size the node made: our own encoder's WebP.
type ArtImage struct {
	Size  ArtSizeName `json:"size"`
	Bytes []byte      `json:"bytes"`
}

// ArtworkResult is the node's answer: every size it made, as our own encoder wrote it, or why it
// made none.
type ArtworkResult struct {
	Job    int64      `json:"job"`
	Width  int        `json:"width"`
	Height int        `json:"height"`
	Images []ArtImage `json:"images"`
	// SourceHash is the SHA-256, in hex, of the file a Path command named, as the node read it:
	// what the core stores the sizes under.
	SourceHash string `json:"sourceHash,omitempty"`
	// Refused says why no size was made: over the pixel limit, not an image, a failed encode.
	Refused string `json:"refused,omitempty"`
}
