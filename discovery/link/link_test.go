// SPDX-License-Identifier: AGPL-3.0-or-later

package link

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

const fingerprint = "Zmh6rfhivXdsj8GLjp-OIAiXFIVu4jOzkCpZHQ1fKSU"

func TestFragment_IsReadBackAsWrittenAndAnythingElseIsRefused(t *testing.T) {
	t.Parallel()
	written := Fragment{CoreID: "db8hem8bf2zsbs3xchpcc3fa02", Secret: "abc_DEF-123", Fingerprint: fingerprint}
	for _, text := range []string{written.String(), "#" + written.String()} {
		if read, err := Read(text); err != nil || read != written {
			t.Errorf("Read(%q) = %+v, %v; want %+v", text, read, err, written)
		}
	}
	for _, text := range []string{
		"", "abc", "core.secret", "core.secret." + fingerprint[:42], "core..", "core.sec ret." + fingerprint,
		"core.secret." + fingerprint + ".more", ".secret." + fingerprint, "co/re.secret." + fingerprint,
	} {
		if _, err := Read(text); err == nil {
			t.Errorf("Read(%q) took it as a link", text)
		}
	}
}

func TestLocateHash_IsTheSecretsHashUnderItsOwnLabel(t *testing.T) {
	t.Parallel()
	digest := sha256.Sum256([]byte("orpheum-locate" + "abc_DEF-123"))
	if got, want := LocateHash("abc_DEF-123"), base64.RawURLEncoding.EncodeToString(digest[:]); got != want {
		t.Errorf("LocateHash = %q; want %q", got, want)
	}
	if LocateHash("abc_DEF-123") == LocateHash("abc_DEF-124") {
		t.Error("two secrets hash alike")
	}
}
