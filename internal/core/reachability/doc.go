// SPDX-License-Identifier: GPL-3.0-or-later

// Package reachability checks the server's path from outside in, for the System page (D243 §7):
// its public name and where it points, HTTPS through the name back to this server, and the voice
// relay's port over UDP and TCP. A failure names its hop (remote-access.md, "Am I reachable?"):
// the name, nothing answering at its address (the VPS, its forwards or the tunnel), the
// certificate, or another server answering for it. The check is the core asking its own name, as a
// browser outside would; the answer coming back to this server, recognised by a nonce, is the
// whole path working.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Opens no file and walks no directory. Enforced by the forbidigo rule tagged [core-fs].
//   - Reached means the request came back to this server, never only that something answered;
//     and each failure is named by its hop. Enforced by
//     TestCheck_ReachesThisServerByItsNameOrNamesTheHopThatFailed and
//     TestCheck_TheRelaysPortAnswersOverUDPAndTCP.
//   - The echo recognises only a nonce this server made, within a minute. Enforced by
//     TestRecognises_OnlyANonceItMadeWithinAMinute.
//
// # Read before changing
//
//   - D243 §7: reachability is a VPS and a WireGuard tunnel.
//   - D152: "Am I reachable?" is three questions; a failure names its hop.
//   - docs/contributors/design/plans/remote-access.md; deploy/vps/README.md.
//
// # Where to change
//
//   - reachability.go: the checks, their outcomes, and the echo's nonces. The endpoints are
//     internal/core/api's (reachability.go), the page web/apps/web/src/system-reachability.tsx.
package reachability
