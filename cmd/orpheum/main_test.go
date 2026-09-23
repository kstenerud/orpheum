// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_PlannedSubcommandsAreNotImplementedYet(t *testing.T) {
	t.Parallel()
	for _, planned := range plannedSubcommands() {
		t.Run(planned.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			status := run([]string{planned.name}, &stdout, &stderr)
			if status != exitUsageError {
				t.Errorf("exit status = %d, want %d", status, exitUsageError)
			}
			if !strings.Contains(stderr.String(), "not implemented yet") {
				t.Errorf("stderr = %q, want it to say the subcommand is not implemented yet", stderr.String())
			}
		})
	}
}

func TestRun_Version(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	status := run([]string{"version"}, &stdout, &stderr)
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
			status := run(tc.arguments, &stdout, &stderr)
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
	run([]string{"dev"}, &stdout, &stderr)
	unknown := strings.Contains(stderr.String(), "unknown subcommand")
	if unknown == isDevBuild {
		t.Errorf("isDevBuild = %v but stderr = %q: the dev subcommand must exist exactly in dev builds",
			isDevBuild, stderr.String())
	}
}
