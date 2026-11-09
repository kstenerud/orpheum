// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto_test

import (
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// Every value the discovery protocol names is camelCase, as its keys are (the owner, 2026-11-09):
// its refusals, what a probe found and the signal of a refused copy.
func TestNames_TheValuesTheProtocolSaysAreCamelCase(t *testing.T) {
	t.Parallel()
	for _, each := range []struct{ name, want string }{
		{discoveryproto.RefusalNotFound, "notFound"},
		{discoveryproto.RefusalTooManyCodes, "tooManyCodes"},
		{discoveryproto.RefusalTooManyClaims, "tooManyClaims"},
		{discoveryproto.RefusalSlowDown, "slowDown"},
		{discoveryproto.RefusalEnrolAgain, "enrolAgain"},
		{discoveryproto.RefusalAnotherInstance, "anotherInstance"},
		{discoveryproto.RefusalHolderAway, "holderAway"},
		{discoveryproto.RefusalNotClaimed, "notClaimed"},
		{discoveryproto.RefusalVersion, "unsupportedVersion"},
		{discoveryproto.RefusalWrongKey, "wrongKey"},
		{discoveryproto.ProbeTimedOut, "timedOut"},
		{discoveryproto.ProbeWrongCore, "wrongCore"},
		{discoveryproto.SignalCopyRefused, "copyRefused"},
	} {
		if each.name != each.want {
			t.Errorf("the protocol says %q; want %q", each.name, each.want)
		}
	}
}
