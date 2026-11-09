// SPDX-License-Identifier: AGPL-3.0-or-later

// Package localtime is the one place a moment becomes wall-clock time in a time zone, and back.
// The server's times are UTC (D204); only a few things are about a wall clock, such as the hour a
// scheduled backup is made (D246 §7), and they take a Zone, the server's own from the timeZone
// setting, and read a Time, a type of its own that cannot be mistaken for a UTC time.Time.
//
// # Invariants
//
//   - Only this package converts between zones: time.LoadLocation, time.FixedZone and
//     time.Time.In are banned elsewhere, and time.Local is set to UTC once by cmd/orpheum. Enforced
//     by the forbidigo rules tagged [local-time] and [ambient].
//   - A zone is named; the machine's own ("Local") and none are refused, and a wall clock follows
//     summer time. Enforced by TestZone_ReadsTheWallClockOfANamedZoneAndConvertsBack.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D204: no ambient configuration; the server's times are UTC.
//   - D246 §7: the server's time zone, and the settings in it, whose keys end _local_tz.
//
// # Where to change
//
//   - localtime.go: Zone, Time, and the two conversions.
package localtime
