// SPDX-License-Identifier: GPL-3.0-or-later

package tmdb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// ImageBaseURL is TMDB's image server, at each image's original size: the node cuts our sizes
// from it (D212).
const ImageBaseURL = "https://image.tmdb.org/t/p/original"

// maxImageBytes bounds a download: TMDB's largest originals are a few megabytes.
const maxImageBytes = 32 << 20

// imageRef is the shape of TMDB's image paths: one name at the root. A reference is the
// provider's text, so nothing else is fetched: no other path on the host, no query, no other
// host.
var imageRef = regexp.MustCompile(`^/[A-Za-z0-9_-]{1,64}\.(jpg|jpeg|png|webp)$`)

// ErrBadImageRef is a reference that is not the shape of TMDB's image paths.
var ErrBadImageRef = errors.New("not a TMDB image path")

// Images downloads TMDB's images. It needs no key.
type Images struct {
	http *http.Client
	base string
}

var _ artwork.Fetcher = (*Images)(nil)

// NewImages returns TMDB's image downloader calling base (ImageBaseURL, or a test's fake)
// through httpClient, which comes from internal/platform/outbound.
func NewImages(httpClient *http.Client, base string) *Images {
	return &Images{http: httpClient, base: strings.TrimSuffix(base, "/")}
}

// notFoundKept is how long TMDB's image CDN serves a 404 from its cache: max-age=43200
// (provider-etiquette.md, measured).
const notFoundKept = 12 * time.Hour

// RecheckAfter is how long TMDB's CDN keeps a 404.
func (*Images) RecheckAfter() time.Duration { return notFoundKept }

// Fetch downloads one image by its path, as an unsanitised value for the node to re-encode.
func (images *Images) Fetch(ctx context.Context, ref string) (artwork.Unsanitised, error) {
	if !imageRef.MatchString(ref) {
		return artwork.Unsanitised{}, fmt.Errorf("%q: %w: %w", ref, ErrBadImageRef, artwork.ErrNoImage)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, images.base+ref, http.NoBody)
	if err != nil {
		return artwork.Unsanitised{}, fmt.Errorf("tmdb image %s: %w", ref, err)
	}
	response, err := images.http.Do(request)
	if err != nil {
		if urlErr, isURL := errors.AsType[*url.Error](err); isURL {
			err = urlErr.Err
		}
		return artwork.Unsanitised{}, fmt.Errorf("tmdb image %s: %w", ref, err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // a read-only body
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return artwork.Unsanitised{}, fmt.Errorf("tmdb image %s: %w", ref, artwork.ErrNoImage)
	case http.StatusTooManyRequests:
		if after, found := outbound.RetryAfter(response.Header); found {
			return artwork.Unsanitised{}, fmt.Errorf("tmdb image %s: %w", ref, &identify.RateLimitedError{After: after})
		}
		return artwork.Unsanitised{}, fmt.Errorf("tmdb image %s: status 429", ref)
	default:
		return artwork.Unsanitised{}, fmt.Errorf("tmdb image %s: status %d", ref, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxImageBytes+1))
	if err != nil {
		return artwork.Unsanitised{}, fmt.Errorf("tmdb image %s: read it: %w", ref, err)
	}
	if len(body) > maxImageBytes {
		return artwork.Unsanitised{}, fmt.Errorf("tmdb image %s: larger than %d bytes: %w", ref, maxImageBytes,
			artwork.ErrNoImage)
	}
	return artwork.Received(body), nil
}
