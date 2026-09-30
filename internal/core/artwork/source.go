// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
)

// ErrNoImage is a source that has no image at a reference: gone from the provider, or a
// reference it never made.
var ErrNoImage = errors.New("no image there")

// Unsanitised is an image as someone else wrote it: a provider's download, a person's upload
// (D212 §8). Its bytes cannot be read outside this package, and its one use is being sent to the
// node to be re-encoded (principles §22).
type Unsanitised struct {
	bytes []byte
}

// Received holds bytes that arrived from outside as an unsanitised image. The caller keeps no
// other copy.
func Received(image []byte) Unsanitised {
	return Unsanitised{bytes: image}
}

// Len is how many bytes arrived.
func (source Unsanitised) Len() int {
	return len(source.bytes)
}

// sourceSha256Hex names the image's sizes in the store: the SHA-256 of its source in hex, so the
// same image fetched for two items, or twice, is stored once.
func (source Unsanitised) sourceSha256Hex() string {
	return hex.EncodeToString(source.sourceSha256())
}

// sourceSha256 is the source's SHA-256 as its 32 bytes, as the artwork table's source_sha256 holds
// it; sourceSha256Hex spells it in hex.
func (source Unsanitised) sourceSha256() []byte {
	sum := sha256.Sum256(source.bytes)
	return sum[:]
}

// Fetcher downloads a source's images by the references its metadata names: TMDB's image
// server, by a poster's path.
type Fetcher interface {
	Fetch(ctx context.Context, ref string) (Unsanitised, error)
	// RecheckAfter is how long the source's CDN answers a missing image from its cache: a
	// recheck sooner would see the same 404 (D213 §1).
	RecheckAfter() time.Duration
	// Destination is the source's image server as the job queue names it: its jobs park on it
	// while it is down (D231 §5).
	Destination() jobs.Destination
}
