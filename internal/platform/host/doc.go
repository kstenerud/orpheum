// SPDX-License-Identifier: AGPL-3.0-or-later

// Package host says what operating system a machine runs: what `lsb_release -a` shows, read
// from /etc/os-release, which is what lsb_release itself reads, and the kernel's release. Reading
// the file rather than running the program keeps the node to the two programs it runs (cut-a §7).
//
// # Invariants
//
//   - The distribution, description, release and codename are os-release's NAME, PRETTY_NAME,
//     VERSION_ID and VERSION_CODENAME, quoted or not.
//     TestParseOSRelease_ReadsWhatLSBReleaseShowsFromOSRelease.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D225: the System page shows each machine's operating system.
//
// # Where to change
//
//   - host.go.
package host
