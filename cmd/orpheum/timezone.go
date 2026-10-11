// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"os"
	"strings"

	"github.com/kstenerud/orpheum/internal/platform/localtime"
)

// timeZoneVariable is the environment variable that names the machine's time zone.
const timeZoneVariable = "TZ"

// zoneDatabaseDir is where a zone's file lives in the system's zone database: /etc/localtime
// links to a file under it, which names the zone.
const zoneDatabaseDir = "zoneinfo/"

// machineZoneSources are where the machine says its time zone is: the environment, the link
// /etc/localtime, and the file /etc/timezone. Tests give their own.
type machineZoneSources struct {
	lookupEnv func(string) (string, bool)
	readLink  func(string) (string, error)
	readFile  func(string) ([]byte, error)
}

// systemZoneSources are the machine's own.
func systemZoneSources(lookupEnv func(string) (string, bool)) machineZoneSources {
	return machineZoneSources{lookupEnv: lookupEnv, readLink: os.Readlink, readFile: os.ReadFile}
}

// machineZone is the machine's own time zone, which is the server's until the admin chooses one
// (D246 §7): TZ's, else the zone /etc/localtime links to, else /etc/timezone's; UTC when none of
// them names a zone the server knows. It is read once, at start; the process's own zone stays UTC
// (D204), and this is only a default.
func machineZone(sources machineZoneSources) localtime.Zone {
	var candidates []string
	if value, isSet := sources.lookupEnv(timeZoneVariable); isSet {
		candidates = append(candidates, zoneNameOf(strings.TrimPrefix(value, ":")))
	}
	if target, err := sources.readLink("/etc/localtime"); err == nil {
		candidates = append(candidates, zoneNameOf(target))
	}
	if text, err := sources.readFile("/etc/timezone"); err == nil {
		candidates = append(candidates, strings.TrimSpace(string(text)))
	}
	for _, name := range candidates {
		if zone, err := localtime.Named(name); err == nil {
			return zone
		}
	}
	return localtime.UTC()
}

// zoneNameOf is the zone a path into the zone database names, "Europe/Berlin" for
// "/usr/share/zoneinfo/Europe/Berlin", or name itself when it is not such a path.
func zoneNameOf(name string) string {
	if _, after, found := strings.Cut(name, zoneDatabaseDir); found {
		return after
	}
	return name
}
