// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !dev

package main

import (
	"os"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
)

// isDevBuild is false in every release build; see build_dev.go.
const isDevBuild = false

// devCommand is nil in a release build: there is no dev subcommand to run.
func devCommand() command {
	return nil
}

// environmentLookup finds nothing in a release build but TZ: the shipped binary is configured by
// its config file, its flags and its database, and by nothing it happens to inherit (D204), save
// the machine's time zone, which only says what the server's time zone is until the admin chooses
// one (D246 §7).
func environmentLookup() func(string) (string, bool) {
	return func(name string) (string, bool) {
		if name == timeZoneVariable {
			return os.LookupEnv(timeZoneVariable) // the default time zone's alone (D246 §7)
		}
		return "", false
	}
}

// relaxForDevelopment changes nothing in a release: a discovery's offers make connections that
// check no address of the core's own networks, so no one holding a link can turn the core on them.
func relaxForDevelopment(rtc *rtcapi.Server) *rtcapi.Server {
	return rtc
}
