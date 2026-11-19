// SPDX-License-Identifier: AGPL-3.0-or-later

package tmdb

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/secrets"
)

// emptyKey is a key set to nothing: Settings' field cleared.
type emptyKey struct{}

func (emptyKey) Open(context.Context, secrets.Name) (secrets.Value, error) {
	return secrets.Value{}, nil
}

func TestWithShippedKey_AKeyNeverSetOrSetEmptyIsOrpheumsOwn(t *testing.T) {
	t.Parallel()
	for name, keys := range map[string]Keys{"never set": fixedKey(""), "set empty": emptyKey{}} {
		value, err := WithShippedKey(keys).Open(t.Context(), secrets.TMDBAPIKey)
		if err != nil || value.Reveal() != shippedKey {
			t.Errorf("a TMDB key %s opens as (%v, %v); want Orpheum's own", name, value.IsZero(), err)
		}
	}
	if !strings.HasPrefix(shippedKey, "eyJ") {
		t.Error("the shipped key is not a read-access token, which TMDB takes as a bearer token")
	}
}

func TestWithShippedKey_AKeySetIsTheAdminsAndOtherNamesAreUntouched(t *testing.T) {
	t.Parallel()
	value, err := WithShippedKey(fixedKey("the-admins-key")).Open(t.Context(), secrets.TMDBAPIKey)
	if err != nil || value.Reveal() != "the-admins-key" {
		t.Errorf("a TMDB key set opens as (%v, %v); want the admin's", value.IsZero(), err)
	}
	if _, err := WithShippedKey(fixedKey("")).Open(t.Context(), secrets.OpenSubtitlesUsername); !errors.Is(
		err, secrets.ErrNotSet) {
		t.Errorf("another name never set opens with %v; want ErrNotSet, as its store said", err)
	}
}
