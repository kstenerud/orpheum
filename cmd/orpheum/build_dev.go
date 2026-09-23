// SPDX-License-Identifier: GPL-3.0-or-later

//go:build dev

package main

// isDevBuild is true only in binaries built with -tags dev (`make dev`), which alone offer the
// `dev` subcommand: it seeds known users and passwords, so a release must not contain it.
const isDevBuild = true
