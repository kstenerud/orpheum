// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// UncontrolledCapabilitiesVariable names the comma-separated list of capabilities a job may run
// without.
// It is set per CI step or lab job, never in a developer's shell profile (testing.md §3).
const UncontrolledCapabilitiesVariable = "ORPHEUM_TEST_UNCONTROLLED"

// Require declares that the test needs capability. The error whyAbsent is nil when the capability
// is present, and otherwise says why it is not; Require then fails the test, or skips it when
// the job lists capability in ORPHEUM_TEST_UNCONTROLLED.
func Require(tb testing.TB, capability string, whyAbsent error) {
	tb.Helper()
	require(tb, os.Getenv(UncontrolledCapabilitiesVariable), capability, whyAbsent)
}

func require(tb testing.TB, uncontrolled, capability string, whyAbsent error) {
	tb.Helper()
	if whyAbsent == nil {
		return
	}
	listed := uncontrolledList(uncontrolled)
	if slices.Contains(listed, capability) {
		tb.Skipf("%s is absent, and this job lists it in %s=%s: %v",
			capability, UncontrolledCapabilitiesVariable, uncontrolled, whyAbsent)
		return
	}
	tb.Fatalf("%s is absent: %v. A missing capability fails (D190 §2); only a CI step or lab job "+
		"that cannot provide it lists it in %s", capability, whyAbsent, UncontrolledCapabilitiesVariable)
}

func uncontrolledList(value string) []string {
	var names []string
	for name := range strings.SplitSeq(value, ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}
