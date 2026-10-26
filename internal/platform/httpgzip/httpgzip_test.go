// SPDX-License-Identifier: AGPL-3.0-or-later

package httpgzip_test

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/httpgzip"
)

func TestAccepts_TakesGzipByNameOrStarUnlessItsWeightIsZero(t *testing.T) {
	t.Parallel()
	for header, want := range map[string]bool{
		"gzip": true, "gzip, deflate, br": true, "br;q=1.0, GZIP;q=0.5": true, "*": true,
		"gzip;q=0.001": true, "": false, "br, zstd": false, "gzip;q=0": false, "gzip; q=0.000": false,
		"*;q=0": false, "identity": false,
	} {
		if got := httpgzip.Accepts(header); got != want {
			t.Errorf("Accepts(%q) = %v, want %v", header, got, want)
		}
	}
}
