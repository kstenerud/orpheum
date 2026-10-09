// SPDX-License-Identifier: GPL-3.0-or-later

// Package rtcapitest is a client's end of the WebRTC transport, for tests: a pion peer that sends
// each request on a channel of its own and reads its response whole, as the web client does.
//
// # Invariants
//
//   - Speaks the transport as internal/core/rtcapi serves it: a text head, body messages of at most
//     rtcapi.MaxMessageBytes, an empty message ending each side. Its use in rtcapi's tests is what
//     holds it to the server.
//
// # Read before changing
//
//   - docs/contributors/design/plans/webrtc-transport.md.
//
// # Where to change
//
//   - client.go: connecting, sending a request and reading its response.
package rtcapitest
