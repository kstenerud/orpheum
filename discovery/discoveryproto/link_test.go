// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto_test

import (
	"encoding/json"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// The paths and keys D527 adds are the ones orpheum-connect builds against: a link token, the key
// salt in each heartbeat and the salt a page asks for.
func TestLink_ThePathsAndKeysAreTheOnesConnectBuildsAgainst(t *testing.T) {
	t.Parallel()
	if discoveryproto.PathLinkToken != "/link-token" || discoveryproto.PathRealmSalt != "/realm-salt" {
		t.Errorf("paths %q and %q; want /link-token and /realm-salt",
			discoveryproto.PathLinkToken, discoveryproto.PathRealmSalt)
	}
	for _, each := range []struct {
		value any
		want  string
	}{
		{discoveryproto.LinkTokenRequest{ReturnTo: "r", Name: "n"}, `{"returnTo":"r","name":"n"}`},
		{
			discoveryproto.LinkTokenResponse{Token: "t", ExpiresAt: 1, LinkURL: "u"},
			`{"token":"t","expiresAt":1,"linkUrl":"u"}`,
		},
		{discoveryproto.RealmSaltRequest{Realm: "r"}, `{"realm":"r"}`},
		{discoveryproto.RealmSaltResponse{Salt: "s"}, `{"salt":"s"}`},
		{
			discoveryproto.AddressesRequest{KeySalt: "k"},
			`{"orpheumVersion":"","apiMajor":0,"candidates":null,"keySalt":"k"}`,
		},
	} {
		got, err := json.Marshal(each.value)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != each.want {
			t.Errorf("%T is %s; want %s", each.value, got, each.want)
		}
	}
}
