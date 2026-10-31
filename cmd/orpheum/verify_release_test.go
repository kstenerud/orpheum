// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
	"github.com/kstenerud/orpheum/internal/core/discovery"
)

// The install script's check: an image is installed only when a signed release record that a root
// key signed names its digest as a release's; a record a stranger signed, or a digest no release
// has, fails, and with no root keys given, the suggested discovery's are taken, never none (D391 §6).
func TestVerifyRelease_AnImageIsAReleaseOnlyByASignedRecordThatNamesItsDigest(t *testing.T) {
	t.Parallel()
	root := trusttest.NewRoot(t)
	digest := "sha256:" + strings.Repeat("a", 64)
	record := release.Record{
		IssuedAt: 1, Latest: "v1.2.3", Locations: []string{"registry.example/orpheum"},
		Releases: map[string]release.Note{"v1.2.3": {Digest: digest}},
	}
	encoded := func(signed trust.Signed) string {
		text, err := json.Marshal(signed)
		if err != nil {
			t.Fatal(err)
		}
		return string(text)
	}
	signed := encoded(root.Signs(t, trust.KindReleaseRecord, record))
	verify := func(input, digest string, arguments ...string) (int, string) {
		var stdout, stderr bytes.Buffer
		status := commandVerifyRelease(t.Context(), invocation{
			arguments: append([]string{"--digest", digest}, arguments...), stdin: strings.NewReader(input),
			stdout: &stdout, stderr: &stderr,
		})
		return status, stdout.String() + stderr.String()
	}
	if status, said := verify(signed, digest, "--root-key", root.Line); status != exitSuccess ||
		!strings.Contains(said, "is release v1.2.3") {
		t.Errorf("the release's own digest: %d %q; want it verified as v1.2.3", status, said)
	}
	other := "sha256:" + strings.Repeat("b", 64)
	if status, said := verify(signed, other, "--root-key", root.Line); status != exitFailure ||
		!strings.Contains(said, "names no release") {
		t.Errorf("a digest no release has: %d %q; want refused", status, said)
	}
	stranger := trusttest.NewRoot(t)
	forged := encoded(stranger.Signs(t, trust.KindReleaseRecord, record))
	if status, said := verify(forged, digest, "--root-key", root.Line); status != exitFailure {
		t.Errorf("a record a stranger signed: %d %q; want refused", status, said)
	}
	if len(discovery.Suggested().RootKeys) == 0 {
		if status, said := verify(signed, digest); status != exitFailure || !strings.Contains(said, "no root keys") {
			t.Errorf("no root keys given, none suggested: %d %q; want refused, saying so", status, said)
		}
	}
}
