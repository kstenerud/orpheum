// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/strictjson"
)

// The discovery's answer is read strictly (D456): a nonce said twice is refused, not read as the
// last of them.
func TestCall_AnAnswerWithAKeySaidTwiceIsRefused(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	twice := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"nonce":"one","nonce":"two"}`)) //nolint:errcheck // the test reads the outcome
	}))
	t.Cleanup(twice.Close)
	if _, err := built.service.challenge(t.Context(), twice.URL); !errors.Is(err, strictjson.ErrRefused) {
		t.Errorf("an answer with a duplicate key: %v; want strictjson.ErrRefused", err)
	}
}
