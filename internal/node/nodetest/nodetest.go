// SPDX-License-Identifier: AGPL-3.0-or-later

package nodetest

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/hash"
)

// OpenMediaRoot opens directory as a media root, closed when the test ends.
func OpenMediaRoot(tb testing.TB, directory string) *confine.MediaRoot {
	tb.Helper()
	root, err := confine.OpenMediaRoot(directory)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := root.Close(); err != nil {
			tb.Error(err)
		}
	})
	return root
}

// PayloadHash is the payload hash of a file that holds contents.
func PayloadHash(tb testing.TB, contents string) []byte {
	tb.Helper()
	identity, err := hash.PayloadIdentity(strings.NewReader(contents), int64(len(contents)))
	if err != nil {
		tb.Fatal(err)
	}
	return identity.PayloadHash[:]
}

// PayloadHashHex is PayloadHash in hex, as a command names a file by it.
func PayloadHashHex(tb testing.TB, contents string) string {
	tb.Helper()
	return hex.EncodeToString(PayloadHash(tb, contents))
}
