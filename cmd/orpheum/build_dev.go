// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build dev

package main

import (
	"os"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
)

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

// relaxForDevelopment lets the WebRTC server check any address a discovery's offer names, the
// machine's own too: a development build's browser tests run the browser, the discovery and the core
// on one machine (webrtc-transport.md W4b.6). A release refuses them (build_release.go).
func relaxForDevelopment(rtc *rtcapi.Server) *rtcapi.Server {
	rtc.AcceptAnyRelayedAddress()
	return rtc
}
