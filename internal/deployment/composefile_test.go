// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"regexp"
	"strconv"
	"testing"
)

// composeVersionArgument is a compose file's --compose-version and the number after it.
var composeVersionArgument = regexp.MustCompile(`(?m)^\s+- --compose-version\n\s+- "?(\d+)"?$`)

// The compose files a household is sent to paste are the version this release says they are, a file
// naming none being version 1, so a household's server compares its file with the right one; and
// each version after the first says what it brings, for the System page to tell a household whose
// file is older (D461 §3).
func TestComposeFileVersion_IsThePublishedFilesAndEachLaterOneSaysWhatItBrings(t *testing.T) {
	t.Parallel()
	for _, name := range []string{omvComposeFile, synologyCompose} {
		version := 1
		matches := composeVersionArgument.FindAllStringSubmatch(householdCompose(t, name), -1)
		if len(matches) > 1 {
			t.Errorf("%s passes --compose-version %d times; want once at most", name, len(matches))
		}
		if len(matches) == 1 {
			version, _ = strconv.Atoi(matches[0][1]) //nolint:errcheck // the pattern matched digits
		}
		if version != ComposeFileVersion {
			t.Errorf("%s is compose file version %d; want ComposeFileVersion, %d", name, version, ComposeFileVersion)
		}
	}
	changes := ComposeFileChanges()
	for version := 2; version <= ComposeFileVersion; version++ {
		if changes[version] == "" {
			t.Errorf("compose file version %d does not say what it brings in ComposeFileChanges", version)
		}
	}
	for version := range changes {
		if version < 2 || version > ComposeFileVersion {
			t.Errorf("ComposeFileChanges names version %d; want only 2 to %d", version, ComposeFileVersion)
		}
	}
}
