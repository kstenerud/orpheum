// SPDX-License-Identifier: AGPL-3.0-or-later

package tmdb

import (
	"context"
	"errors"

	"github.com/kstenerud/orpheum/internal/core/secrets"
)

// shippedKey is Orpheum's read-access token, as TMDB registered the application. It ships with the
// program, as Jellyfin's and Kodi's keys do, so posters come with nothing to set; a key an admin
// sets in Settings is used in its place (D527 §1.3). Anyone can read it from a release, so it is no
// secret; it is opened as a secrets.Value all the same, so that it never prints (D194 §5).
const shippedKey = "eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJiMDg2ODEwMzNlODY2MTA5NWRlZTRmMmU5NzQ4MDcwYiIsIm5iZiI6MTc5MDkyNzU4OS43MzQwMDAyLCJzdWIiOiI2YWJmNjJlNThjN2ZmODY2YTdiMDkyOGQiLCJzY29wZXMiOlsiYXBpX3JlYWQiXSwidmVyc2lvbiI6MX0.tuEb27go1eJSQ2mCTQXmIOP1O1da8ZXC1-LMgj-aD_M" //nolint:lll // one token, which cannot be broken

// WithShippedKey is keys with Orpheum's own TMDB key in place of one never set or set empty. Every
// other name, and a TMDB key that is set, is keys' own.
func WithShippedKey(keys Keys) Keys {
	return shippedKeys{keys: keys}
}

type shippedKeys struct {
	keys Keys
}

// Open is keys' Open, with Orpheum's own in place of a TMDB key not set.
func (shipped shippedKeys) Open(ctx context.Context, name secrets.Name) (secrets.Value, error) {
	value, err := shipped.keys.Open(ctx, name)
	if name != secrets.TMDBAPIKey {
		return value, err //nolint:wrapcheck // keys' own answer, passed on as it came
	}
	if errors.Is(err, secrets.ErrNotSet) || (err == nil && value.IsZero()) {
		return secrets.NewValue(shippedKey), nil
	}
	return value, err //nolint:wrapcheck // keys' own answer, passed on as it came
}
