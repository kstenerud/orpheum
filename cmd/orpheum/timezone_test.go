// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"testing"
)

// fakeZoneSources is a machine whose TZ, /etc/localtime link and /etc/timezone say what the test
// says; an empty one is not there.
func fakeZoneSources(tz, link, file string) machineZoneSources {
	missing := errors.New("not there")
	return machineZoneSources{
		lookupEnv: func(name string) (string, bool) { return tz, name == timeZoneVariable && tz != "" },
		readLink: func(string) (string, error) {
			if link == "" {
				return "", missing
			}
			return link, nil
		},
		readFile: func(string) ([]byte, error) {
			if file == "" {
				return nil, missing
			}
			return []byte(file), nil
		},
	}
}

// The machine's time zone is TZ's, else the zone /etc/localtime links to, else /etc/timezone's,
// skipping one that names no zone; UTC when none does (D246 §7).
func TestMachineZone_IsTZsElseTheSystemsElseUTC(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		what           string
		tz, link, file string
		want           string
	}{
		{"TZ", "Europe/Berlin", "/usr/share/zoneinfo/Asia/Tokyo", "", "Europe/Berlin"},
		{"TZ with a colon", ":America/New_York", "", "", "America/New_York"},
		{"TZ as a path", "/usr/share/zoneinfo/Europe/Oslo", "", "", "Europe/Oslo"},
		{"TZ misspelt, the link", "Europe/Berln", "/usr/share/zoneinfo/Asia/Tokyo", "", "Asia/Tokyo"},
		{"the link", "", "../usr/share/zoneinfo/Europe/Rome", "", "Europe/Rome"},
		{"/etc/timezone", "", "", "Europe/Paris\n", "Europe/Paris"},
		{"none", "", "", "", "UTC"},
	} {
		if got := machineZone(fakeZoneSources(check.tz, check.link, check.file)).Name(); got != check.want {
			t.Errorf("%s: the machine's zone = %s, want %s", check.what, got, check.want)
		}
	}
}
