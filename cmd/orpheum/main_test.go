// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/providers/opensubtitles"
	"github.com/kstenerud/orpheum/internal/core/providers/tmdb"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// noEnvironment is an empty environment.
func noEnvironment(string) (string, bool) {
	return "", false
}

// Not parallel: it sets time.Local, and Go runs every parallel test after the sequential ones.
func TestUseUTC_TimesAreUTCWhateverTZSays(t *testing.T) { //nolint:paralleltest // see above
	useUTC()
	if location := (clock.System{}).Now().Location(); location != time.UTC {
		t.Errorf("the system clock is in %v, want UTC (D204)", location)
	}
}

func TestDispatch_PlannedSubcommandsAreNotImplementedYet(t *testing.T) {
	t.Parallel()
	unbuilt := 0
	for _, planned := range plannedSubcommands() {
		if planned.implementation != nil {
			continue
		}
		unbuilt++
		t.Run(planned.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			status := dispatch(t.Context(), []string{planned.name}, noEnvironment, nil, &stdout, &stderr)
			if status != exitUsageError {
				t.Errorf("exit status = %d, want %d", status, exitUsageError)
			}
			if !strings.Contains(stderr.String(), "not implemented yet") {
				t.Errorf("stderr = %q, want it to say the subcommand is not implemented yet", stderr.String())
			}
		})
	}
	if unbuilt == 0 {
		t.Log("every planned subcommand is built; this test has nothing left to check and can go")
	}
}

// Every subcommand that runs on an instance needs its directory, and has no default for it (D242
// §2); default-config needs the directory to make one in, write-deployment the image its
// deployment runs, and verify-release, which runs on none, the digest it checks.
func TestDispatch_BuiltSubcommandsNeedTheirInstance(t *testing.T) {
	t.Parallel()
	for _, planned := range plannedSubcommands() {
		// openapi and install-script print what the binary holds, needing no instance.
		if planned.implementation == nil || planned.name == "openapi" || planned.name == "dev" ||
			planned.name == "install-script" {
			continue
		}
		want := "--instance is required"
		if planned.name == "default-config" {
			want = "usage: orpheum default-config [--node] [--run-dir <dir>] <instance directory>"
		}
		if planned.name == "write-deployment" {
			want = "usage: orpheum write-deployment --image <image>"
		}
		if planned.name == "verify-release" {
			want = "usage: orpheum verify-release --digest <sha256:...>"
		}
		t.Run(planned.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			status := dispatch(t.Context(), []string{planned.name}, noEnvironment, nil, &stdout, &stderr)
			if status == exitSuccess || !strings.Contains(stderr.String(), want) {
				t.Errorf("without its instance: status %d, stderr %q; want a failure saying %q",
					status, stderr.String(), want)
			}
		})
	}
}

func TestDispatch_Version(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	status := dispatch(t.Context(), []string{"version"}, noEnvironment, nil, &stdout, &stderr)
	if status != exitSuccess {
		t.Errorf("exit status = %d, want %d", status, exitSuccess)
	}
	if got := strings.TrimSpace(stdout.String()); got != version {
		t.Errorf("stdout = %q, want the build Go recorded, %q", got, version)
	}
}

func TestDispatch_UsageErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		arguments []string
	}{
		{name: "no subcommand", arguments: nil},
		{name: "unknown subcommand", arguments: []string{"transcode"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			status := dispatch(t.Context(), tc.arguments, noEnvironment, nil, &stdout, &stderr)
			if status != exitUsageError {
				t.Errorf("exit status = %d, want %d", status, exitUsageError)
			}
			for _, planned := range plannedSubcommands() {
				if !strings.Contains(stderr.String(), planned.name) {
					t.Errorf("usage on stderr does not list %q:\n%s", planned.name, stderr.String())
				}
			}
		})
	}
}

func TestDispatch_DevSubcommandExistsOnlyInDevBuilds(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	dispatch(t.Context(), []string{"dev"}, noEnvironment, nil, &stdout, &stderr)
	unknown := strings.Contains(stderr.String(), "unknown subcommand")
	if unknown == isDevBuild {
		t.Errorf("isDevBuild = %v but stderr = %q: the dev subcommand must exist exactly in dev builds",
			isDevBuild, stderr.String())
	}
}

func TestShimIfAsked_OnlyTheShimsArgumentMakesThisProcessTheShim(t *testing.T) {
	t.Parallel()
	var stderr strings.Builder
	for _, arguments := range [][]string{{"orpheum"}, {"orpheum", "run"}, {"orpheum", "node", "--instance"}} {
		if _, isShim := shimIfAsked(arguments, &stderr); isShim {
			t.Errorf("%q made this process the shim", arguments)
		}
	}
	// A spec the shim cannot read returns, rather than becoming a tool, and says why.
	status, isShim := shimIfAsked([]string{"orpheum", confine.ShimArgument, "not a spec"}, &stderr)
	if !isShim || status != exitFailure || !strings.Contains(stderr.String(), "read a confined run") {
		t.Errorf("a bad spec: %d, %v, %q; want the shim to fail and say why", status, isShim, stderr.String())
	}
}

func TestReleaseWiring_ReachesTheProvidersRealAddresses(t *testing.T) {
	t.Parallel()
	outside := releaseWiring()
	if outside.tmdbBaseURL != tmdb.BaseURL || outside.tmdbImagesBaseURL != tmdb.ImageBaseURL ||
		outside.openSubtitlesBaseURL != opensubtitles.BaseURL {
		t.Errorf("a release is wired to %+v; want TMDB's and OpenSubtitles' real addresses", outside)
	}
}
