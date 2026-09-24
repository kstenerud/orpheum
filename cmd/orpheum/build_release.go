// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !dev

package main

// isDevBuild is false in every release build; see build_dev.go.
const isDevBuild = false

// devCommand is nil in a release build: there is no dev subcommand to run.
func devCommand() command {
	return nil
}
