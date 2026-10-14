// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { offerThroughDiscovery, relayedCandidatesMost } from "./relayed-offer.ts";

/** A candidate line: its address, protocol and type. */
const candidate = (address: string, protocol: string, type: string): string =>
  `a=candidate:1 1 ${protocol} 2122260223 ${address} 50000 typ ${type}`;

/** An offer's SDP holding lines, around a session and media part. */
const sdpOf = (lines: readonly string[]): string =>
  [
    "v=0",
    "a=fingerprint:sha-256 AB:CD",
    "m=application 9 UDP/DTLS/SCTP webrtc-datachannel",
    ...lines,
    "",
  ].join("\r\n");

/** The candidate lines an SDP holds. */
const candidatesIn = (sdp: string): string[] =>
  sdp.split("\r\n").filter((line) => line.startsWith("a=candidate:"));

describe("an offer through a discovery", () => {
  it("drops the candidates no core outside the client's network could use", () => {
    const reachable = [
      candidate("2a0f:1000:1::5", "udp", "host"),
      candidate("203.0.113.9", "udp", "srflx"),
      candidate("198.51.100.4", "udp", "relay"),
    ];
    const local = [
      candidate("192.168.1.20", "udp", "host"),
      candidate("10.0.0.3", "udp", "host"),
      candidate("172.20.0.2", "tcp", "host"),
      candidate("100.70.1.1", "udp", "host"),
      candidate("169.254.1.1", "udp", "host"),
      candidate("127.0.0.1", "udp", "host"),
      candidate("fe80::1", "udp", "host"),
      candidate("fd77::3", "udp", "host"),
      candidate("::1", "udp", "host"),
      candidate("::ffff:192.168.1.20", "udp", "host"),
      candidate("4d1f6a2e-0b1c.local", "udp", "host"),
    ];
    const trimmed = offerThroughDiscovery(sdpOf([...local, ...reachable]));
    expect(candidatesIn(trimmed)).toEqual(reachable);
    expect(trimmed).toContain("a=fingerprint:sha-256 AB:CD");
    expect(trimmed).toContain("m=application 9");
  });

  it("keeps at most the discovery's limit, relayed and UDP ones first, as a phone on Wi-Fi offers more", () => {
    const hosts = Array.from({ length: 8 }, (_, index) =>
      candidate(`2a0f:1000:1::${String(index + 1)}`, index % 2 === 0 ? "udp" : "tcp", "host"),
    );
    const relays = [
      candidate("198.51.100.4", "udp", "relay"),
      candidate("198.51.100.4", "tcp", "relay"),
    ];
    const reflexive = candidate("203.0.113.9", "udp", "srflx");
    const kept = candidatesIn(offerThroughDiscovery(sdpOf([...hosts, reflexive, ...relays])));
    expect(kept).toHaveLength(relayedCandidatesMost);
    expect(kept).toEqual(expect.arrayContaining([...relays, reflexive]));
    const udpHosts = hosts.filter((line) => line.includes(" udp "));
    expect(kept).toEqual(expect.arrayContaining(udpHosts));
  });

  it("leaves an offer within the limit as it was", () => {
    const sdp = sdpOf([candidate("203.0.113.9", "udp", "srflx")]);
    expect(offerThroughDiscovery(sdp)).toBe(sdp);
  });
});
