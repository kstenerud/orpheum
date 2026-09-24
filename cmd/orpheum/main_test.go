// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"strings"
	"testing"
)

// noEnvironment is an empty environment.
func noEnvironment(string) (string, bool) {
	return "", false
}

func TestRun_PlannedSubcommandsAreNotImplementedYet(t *testing.T) {
	t.Parallel()
	unbuilt := 0
	for _, planned := range plannedSubcommands() {
		if planned.run != nil {
			continue
		}
		unbuilt++
		t.Run(planned.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			status := run(t.Context(), []string{planned.name}, noEnvironment, &stdout, &stderr)
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

func TestRun_BuiltSubcommandsNeedTheirConfig(t *testing.T) {
	t.Parallel()
	for _, planned := range plannedSubcommands() {
		if planned.run == nil || planned.name == "openapi" || planned.name == "dev" {
			continue
		}
		t.Run(planned.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			status := run(t.Context(), []string{planned.name}, noEnvironment, &stdout, &stderr)
			if status == exitSuccess || !strings.Contains(stderr.String(), "--config is required") {
				t.Errorf("without --config: status %d, stderr %q; want a failure naming --config",
					status, stderr.String())
			}
		})
	}
}

func TestRun_Version(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	status := run(t.Context(), []string{"version"}, noEnvironment, &stdout, &stderr)
	if status != exitSuccess {
		t.Errorf("exit status = %d, want %d", status, exitSuccess)
	}
	if got := strings.TrimSpace(stdout.String()); got != version {
		t.Errorf("stdout = %q, want the linker-set version %q", got, version)
	}
}

func TestRun_UsageErrors(t *testing.T) {
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
			status := run(t.Context(), tc.arguments, noEnvironment, &stdout, &stderr)
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

func TestRun_DevSubcommandExistsOnlyInDevBuilds(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	run(t.Context(), []string{"dev"}, noEnvironment, &stdout, &stderr)
	unknown := strings.Contains(stderr.String(), "unknown subcommand")
	if unknown == isDevBuild {
		t.Errorf("isDevBuild = %v but stderr = %q: the dev subcommand must exist exactly in dev builds",
			isDevBuild, stderr.String())
	}
}
