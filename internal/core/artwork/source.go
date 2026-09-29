// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
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

// hash names the image's sizes in the store: the SHA-256 of its source, so the same image
// fetched for two items, or twice, is stored once.
func (source Unsanitised) hash() string {
	return hex.EncodeToString(source.hashBytes())
}

// hashBytes is the source's SHA-256 as its 32 bytes, as the artwork table's source_sha256 holds
// it; hash spells it in hex.
func (source Unsanitised) hashBytes() []byte {
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
}
