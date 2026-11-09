// SPDX-License-Identifier: AGPL-3.0-or-later

package localtime

import (
	"errors"
	"fmt"
	"time"
	// The zone database, built in, so a zone is known on a machine or in a container that has none
	// of its own (D204: the server reads nothing of the machine's).
	_ "time/tzdata"
)

// ErrNotAZone is a name that is not a time zone the server knows.
var ErrNotAZone = errors.New("not a time zone name, such as Europe/Berlin or UTC")

// Zone is a time zone the admin named: the server's own (the timeZone setting, D246 §7). It is
// the one way into wall-clock time; everything else is UTC.
type Zone struct {
	location *time.Location
}

// UTC is the zone of no offset.
func UTC() Zone { return Zone{location: time.UTC} }

// Named is the zone named name, an IANA name such as Europe/Berlin, or UTC. "Local", which Go reads
// as the machine's own, and empty, which it reads as UTC, are not names (D204).
func Named(name string) (Zone, error) {
	if name == "" || name == "Local" {
		return Zone{}, fmt.Errorf("%q: %w", name, ErrNotAZone)
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return Zone{}, fmt.Errorf("%q: %w", name, ErrNotAZone)
	}
	return Zone{location: location}, nil
}

// IsName is whether Named takes name.
func IsName(name string) bool {
	_, err := Named(name)
	return err == nil
}

// Name is the zone's name, as Named takes it.
func (zone Zone) Name() string {
	if zone.location == nil {
		return "UTC"
	}
	return zone.location.String()
}

// Of is moment as the wall clock in the zone reads it: the conversion from UTC, explicit.
func (zone Zone) Of(moment time.Time) Time {
	location := zone.location
	if location == nil {
		location = time.UTC
	}
	return Time{wall: moment.In(location)}
}

// Time is a moment as a wall clock in a Zone reads it. It is not a time.Time, so it cannot be
// stored, compared or passed where a UTC one is meant; UTC turns it back into one.
type Time struct {
	wall time.Time
}

// Hour is the hour on the wall clock, 0 to 23.
func (wall Time) Hour() int { return wall.wall.Hour() }

// Date is the date on the wall clock.
func (wall Time) Date() (year int, month time.Month, day int) { return wall.wall.Date() }

// UTC is the moment again, in UTC: the conversion back, explicit.
func (wall Time) UTC() time.Time { return wall.wall.UTC() }
