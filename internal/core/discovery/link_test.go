// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// An enrolled core asks its discovery for a link token under its session, sending the page to come
// back to and the server's name, and answers the account page the admin's browser goes to; a core
// enrolled nowhere asks nothing, and a page that is not https is never handed on (D527 §2.1).
func TestLinkToken_AnEnrolledCoreAsksItsDiscoveryAndAnswersTheAccountPage(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	const returnTo = "https://home.example/system"
	if _, err := built.service.LinkToken(ctx, returnTo, "the house"); !errors.Is(err, ErrNotEnrolled) {
		t.Errorf("a link token before connecting: %v; want ErrNotEnrolled", err)
	}
	if _, err := built.service.Connect(ctx, built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	token, err := built.service.LinkToken(ctx, returnTo, "the house")
	if err != nil || token.LinkURL != "https://account.orpheum-core.example/link?token=a-link-token" {
		t.Fatalf("LinkToken = %+v, %v; want the fake's account page", token, err)
	}
	built.fake.mutex.Lock()
	asks := slices.Clone(built.fake.linkAsks)
	built.fake.linkPage = "javascript:alert(1)"
	built.fake.mutex.Unlock()
	want := []discoveryproto.LinkTokenRequest{{ReturnTo: returnTo, Name: "the house"}}
	if !slices.Equal(asks, want) {
		t.Errorf("the discovery was asked %+v; want %+v", asks, want)
	}
	if _, err = built.service.LinkToken(ctx, returnTo, "the house"); !errors.Is(err, ErrNoLinkPage) {
		t.Errorf("a link page that is not https: %v; want ErrNoLinkPage", err)
	}
}
