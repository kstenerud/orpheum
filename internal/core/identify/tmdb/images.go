// SPDX-License-Identifier: GPL-3.0-or-later

package tmdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/platform/bounded"
)

// ImageBaseURL is TMDB's image server, at each image's original size: the node cuts our sizes
// from it (D212).
const ImageBaseURL = "https://image.tmdb.org/t/p/original"

// maxImageBytes bounds a download: TMDB's largest originals are a few megabytes.
const maxImageBytes = 32 << 20

// imageRefPattern is the shape of TMDB's image paths: one name at the root. A reference is the
// provider's text, so nothing else is fetched: no other path on the host, no query, no other
// host.
var imageRefPattern = regexp.MustCompile(`^/[A-Za-z0-9_-]{1,64}\.(jpg|jpeg|png|webp)$`)

// ErrBadImageRef is a reference that is not the shape of TMDB's image paths.
var ErrBadImageRef = errors.New("not a TMDB image path")

// ImageFetcher downloads TMDB's images. It needs no key.
type ImageFetcher struct {
	http    *http.Client
	baseURL string
}

var _ artwork.Fetcher = (*ImageFetcher)(nil)

// NewImageFetcher returns TMDB's image downloader calling baseURL (ImageBaseURL, or a test's fake)
// through httpClient, which comes from internal/platform/outbound.
func NewImageFetcher(httpClient *http.Client, baseURL string) *ImageFetcher {
	return &ImageFetcher{http: httpClient, baseURL: strings.TrimSuffix(baseURL, "/")}
}

// notFoundCachedFor is how long TMDB's image CDN serves a 404 from its cache: max-age=43200
// (provider-etiquette.md, measured).
const notFoundCachedFor = 12 * time.Hour

// RecheckAfter is how long TMDB's CDN keeps a 404.
func (*ImageFetcher) RecheckAfter() time.Duration { return notFoundCachedFor }

// Fetch downloads one image by its path, as an unsanitised value for the node to re-encode.
func (images *ImageFetcher) Fetch(ctx context.Context, ref string) (artwork.Unsanitised, error) {
	if !imageRefPattern.MatchString(ref) {
		return artwork.Unsanitised{}, fmt.Errorf("%q: %w: %w", ref, ErrBadImageRef, artwork.ErrNoImage)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, images.baseURL+ref, http.NoBody)
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
	if refused := statusError("tmdb image "+ref, response, nil, artwork.ErrNoImage); refused != nil {
		return artwork.Unsanitised{}, refused
	}
	body, err := bounded.ReadAll(response.Body, maxImageBytes)
	if errors.Is(err, bounded.ErrTooLarge) {
		return artwork.Unsanitised{}, fmt.Errorf("tmdb image %s: %w: %w", ref, err, artwork.ErrNoImage)
	}
	if err != nil {
		return artwork.Unsanitised{}, fmt.Errorf("tmdb image %s: %w", ref, err)
	}
	return artwork.Received(body), nil
}
