// SPDX-License-Identifier: GPL-3.0-or-later

// Package outbound builds the HTTP client every request the server makes to another host goes
// through: TMDB, OpenSubtitles, the update check. Go's default client and transport take a proxy
// from HTTP_PROXY and HTTPS_PROXY, which is configuration nobody gave the server; this one takes
// none (D204). If a proxy is ever needed, it becomes a config-file key passed in here.
//
// # Invariants
//
//   - A client from NewClient uses no proxy, whatever the environment says, and says it is
//     Orpheum in every request's User-Agent (D213 §6). TestNewClient_TakesNoProxyFromTheEnvironment.
//   - No other package uses http.DefaultClient, http.DefaultTransport, http.Get and its
//     siblings, or http.ProxyFromEnvironment. Enforced by the forbidigo rule tagged [outbound].
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D204: configuration is explicit and contained.
//   - docs/contributors/standards/go.md §12.
//
// # Where to change
//
//   - client.go: transport settings every outbound request shares.
package outbound
