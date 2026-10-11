// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build dev

package main

import "os"

// isDevBuild is true only in binaries built with -tags dev (`make dev`), which alone offer the
// `dev` subcommand: it seeds known users and passwords, so a release must not contain it.
const isDevBuild = true

// devCommand is the dev subcommand, compiled only into -tags dev builds.
func devCommand() command {
	return commandDev
}

// environmentLookup looks names up in the process environment, which only a dev build reads:
// `dev seed` seals the ORPHEUM_DEV_* secrets from it (D204).
func environmentLookup() func(string) (string, bool) {
	return os.LookupEnv
}
