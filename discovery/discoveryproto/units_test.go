// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// A duration names its unit (the owner, 2026-11-09): a session's, a locate hash's and a device
// code's lifetimes are expiresInSeconds, and each side reads the other's.
func TestDurations_NameTheirUnit(t *testing.T) {
	t.Parallel()
	for _, body := range []any{
		discoveryproto.SessionResponse{ExpiresInSeconds: 3600},
		discoveryproto.LocateHash{ExpiresInSeconds: 3600},
		discoveryproto.DeviceCodeResponse{ExpiresInSeconds: 3600},
	} {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(encoded), `"expiresInSeconds":3600`) {
			t.Errorf("%T written %s; want expiresInSeconds", body, encoded)
		}
	}
}
