// SPDX-License-Identifier: GPL-3.0-or-later

// Package clock is the one place that reads the wall clock. Everything else takes a Clock,
// so tests control time with clocktest.Fake and no test sleeps to make time pass.
//
// Stored times are integer milliseconds since the epoch on the server's clock, from
// Now().UnixMilli(); a client's monotonic clock is never mixed with it (D165, D66).
//
// # Invariants
//
//   - Only this package calls time.Now, time.Since or time.Until. Enforced by the forbidigo
//     rule tagged [clock] in .golangci.yml.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D66: no clock synchronisation; monotonic local time and bounded round trips.
//   - D165: the play-state log records state changes, and an offline device must not rewind.
//   - docs/contributors/design/plans/schema.md, Conventions: times are integer milliseconds.
//   - docs/contributors/standards/go.md, Time.
//
// # Where to change
//
//   - clock.go: the Clock interface, what time it is and waiting for time to pass, and the
//     System implementation.
//   - clocktest/: the Fake that tests use. Add a method to Clock only with a production caller
//     and the matching Fake behaviour in the same change.
package clock
