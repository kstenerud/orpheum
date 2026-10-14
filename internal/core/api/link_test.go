// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/link"
	"github.com/kstenerud/orpheum/internal/core/discovery"
)

// A link made while a discovery has claimed the core is the discovery's, naming the core and its key's
// fingerprint around the secret; before, the client makes it on its own origin (the protocol's §3.1).
func TestDiscoveryLink_IsTheDiscoverysFormOnlyWhileItHasClaimedTheCore(t *testing.T) {
	t.Parallel()
	fingerprint := "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw"
	claimed := discovery.Status{
		State: discoveryproto.StateClaimed, CoreID: "k7xq2", ClientOrigin: "https://app.orpheum-app.com",
		Fingerprint: fingerprint,
	}
	got := discoveryLink(claimed, "/register", "a-secret")
	if got != "https://app.orpheum-app.com/register#k7xq2.a-secret."+fingerprint {
		t.Errorf("a claimed core's link is %q; want the discovery's form", got)
	}
	read, err := link.Read(got[len("https://app.orpheum-app.com/register"):])
	if err != nil || read.Secret != "a-secret" || read.CoreID != "k7xq2" {
		t.Errorf("the link reads back as %+v, %v; want its core and secret", read, err)
	}
	for name, status := range map[string]discovery.Status{
		"pending": {
			State: discoveryproto.StatePending, CoreID: "k7xq2", ClientOrigin: "https://app.example",
			Fingerprint: fingerprint,
		},
		"no client origin":   {State: discoveryproto.StateClaimed, CoreID: "k7xq2", Fingerprint: fingerprint},
		"enrolled with none": {},
	} {
		if got := discoveryLink(status, "/register", "a-secret"); got != "" {
			t.Errorf("a core %s makes the link %q; want none, the client making it on its own origin", name, got)
		}
	}
}
