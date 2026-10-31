// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/internal/core/discovery"
)

// maxSignedRecordBytes bounds the signed release record verify-release reads.
const maxSignedRecordBytes = 256 << 10

// commandVerifyRelease is `orpheum verify-release --digest <sha256:…> [--root-key <line>]...`: whether
// the signed release record on standard input holds under the root keys given, the suggested
// discovery's when none is, and names the image digest as a release's (D391 §6). It says the release
// and succeeds, or says why not and fails. The install script runs it before it installs an image.
func commandVerifyRelease(_ context.Context, call invocation) int {
	flags := flag.NewFlagSet("verify-release", flag.ContinueOnError)
	flags.SetOutput(call.stderr)
	digest := flags.String("digest", "", "the image's digest, sha256:... (required)")
	var rootKeys []string
	flags.Func("root-key", "a root key the record may be signed by, as an authorised key's line; repeated for each; "+
		"none takes the suggested discovery's", func(line string) error {
		rootKeys = append(rootKeys, line)
		return nil
	})
	if err := flags.Parse(call.arguments); err != nil || *digest == "" || flags.NArg() != 0 {
		return write(call.stderr, "usage: orpheum verify-release --digest <sha256:...> [--root-key <line>]... "+
			"< signed-record.json\n", exitUsageError)
	}
	tag, err := verifyRelease(call.stdin, *digest, rootKeys)
	if err != nil {
		return call.fail(err)
	}
	return write(call.stdout, fmt.Sprintf("verified: %s is release %s\n", *digest, tag), exitSuccess)
}

// ErrNotAReleasesDigest is an image whose digest the record names for no release.
var ErrNotAReleasesDigest = errors.New("the signed release record names no release with that image digest")

// verifyRelease is the release digest is, by the signed record read from signedRecord, which a key
// of rootKeys must have signed: the suggested discovery's root keys when none is given.
func verifyRelease(signedRecord io.Reader, digest string, rootKeys []string) (string, error) {
	suggested := discovery.Suggested()
	if len(rootKeys) == 0 {
		rootKeys = suggested.RootKeys
	}
	pinned, err := trust.Anchor(suggested.RootSetVersion, rootKeys)
	if errors.Is(err, trust.ErrNoRootKeys) {
		return "", fmt.Errorf("no root keys to check the record by: this build names none for %s; give --root-key: %w",
			suggested.Issuer, err)
	}
	if err != nil {
		return "", err
	}
	var signed trust.Signed
	if err = json.NewDecoder(io.LimitReader(signedRecord, maxSignedRecordBytes)).Decode(&signed); err != nil {
		return "", fmt.Errorf("read the signed release record: %w", err)
	}
	record, err := release.Open(signed, pinned)
	if err != nil {
		return "", fmt.Errorf("the release record: %w", err)
	}
	tag, isNamed := record.VersionOf(digest)
	if !isNamed {
		return "", fmt.Errorf("%s: %w", digest, ErrNotAReleasesDigest)
	}
	return tag, nil
}
