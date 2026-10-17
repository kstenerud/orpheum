// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/discovery/proof"
)

// Anyone may read the client's manifest, signed by the key the core's links name, which a loader
// checks before it runs the client (D308 §6); a server with no client signs none.
func TestClientManifest_IsSignedByTheKeyTheLinksName(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	wantError(t, harness.do(t, http.MethodGet, "/client-manifest", "", nil), http.StatusNotFound, CodeNotFound)
	manifest := []byte(`{"version":"test","script":"index-x.js","files":[]}`)
	harness.server.SetClientManifest(manifest)
	served := harness.do(t, http.MethodGet, "/client-manifest", "", nil)
	var got ClientManifestBody
	if err := json.Unmarshal([]byte(served.body), &got); err != nil || served.status != http.StatusOK {
		t.Fatalf("GET /client-manifest = %d %s (%v)", served.status, served.body, err)
	}
	decode := func(text string) []byte {
		decoded, err := base64.RawURLEncoding.DecodeString(text)
		if err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	publicKey := ed25519.PublicKey(decode(got.PublicKey))
	if string(decode(got.Manifest)) != string(manifest) ||
		!proof.VerifyClientManifest(publicKey, decode(got.Manifest), decode(got.Signature)) {
		t.Errorf("the manifest served is %+v; want the manifest set, signed under its key", got)
	}
	if proof.Fingerprint(publicKey) != harness.server.dependencies.Load().Discovery.Status().Fingerprint {
		t.Error("the manifest is signed by a key other than the one the core's links name")
	}
}
