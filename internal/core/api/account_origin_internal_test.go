// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/discovery"
)

// The System page is told where the discovery's households keep their admin accounts, as its signed
// document names it, so it can link the admin there for a key; a discovery with none sends none.
func TestDiscoveryStatusBody_CarriesTheAccountOrigin(t *testing.T) {
	t.Parallel()
	if body := discoveryStatusBody(discovery.Status{AccountOrigin: "https://account.example"}); body.AccountOrigin !=
		"https://account.example" {
		t.Errorf("the body's account origin = %q; want https://account.example", body.AccountOrigin)
	}
	if none := discoveryStatusBody(discovery.Status{}); none.AccountOrigin != "" {
		t.Errorf("with none, the body's account origin = %q; want none", none.AccountOrigin)
	}
}
