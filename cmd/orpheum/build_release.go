// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !dev

package main

// isDevBuild is false in every release build; see build_dev.go.
const isDevBuild = false

// devCommand is nil in a release build: there is no dev subcommand to run.
func devCommand() command {
	return nil
}

// environment is empty in a release build: the shipped binary is configured by its config file,
// its flags and its database, and by nothing it happens to inherit (D204).
func environment() func(string) (string, bool) {
	return func(string) (string, bool) { return "", false }
}
