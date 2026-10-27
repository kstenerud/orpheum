// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { candidatePairsOf, pairsReportedMost } from "./rtc-pairs.ts";
import type { ChannelLike } from "./rtc-request.ts";
import { RtcTransport } from "./rtc-transport.ts";

/** The stats of the owner's Firefox at home on staging: the LAN host pair failed, the hairpin won. */
const hairpinned = [
  {
    id: "L1",
    type: "local-candidate",
    candidateType: "host",
    protocol: "udp",
    address: "3f2a.local",
    port: 51233,
  },
  {
    id: "L2",
    type: "local-candidate",
    candidateType: "srflx",
    protocol: "udp",
    address: "11.22.33.144",
    port: 51234,
  },
  {
    id: "R1",
    type: "remote-candidate",
    candidateType: "host",
    protocol: "udp",
    address: "172.17.0.2",
    port: 40000,
  },
  {
    id: "R2",
    type: "remote-candidate",
    candidateType: "host",
    protocol: "udp",
    address: "192.168.1.199",
    port: 8443,
  },
  {
    id: "R3",
    type: "remote-candidate",
    candidateType: "srflx",
    protocol: "udp",
    address: "11.22.33.144",
    port: 40001,
  },
  {
    id: "P1",
    type: "candidate-pair",
    localCandidateId: "L2",
    remoteCandidateId: "R3",
    state: "succeeded",
    nominated: true,
    selected: true,
  },
  {
    id: "P2",
    type: "candidate-pair",
    localCandidateId: "L2",
    remoteCandidateId: "R1",
    state: "failed",
  },
  {
    id: "P3",
    type: "candidate-pair",
    localCandidateId: "L1",
    remoteCandidateId: "R2",
    state: "failed",
  },
  { id: "C1", type: "codec" },
];

describe("a WebRTC connection's candidate pairs as the browser sees them (local-path-2026-10-27.md §4 item 2)", () => {
  it("puts the selected pair first, then those to the core's host candidates, each end by type and address", () => {
    const shuffled = [...hairpinned.slice(5).reverse(), ...hairpinned.slice(0, 5)];
    expect(candidatePairsOf(shuffled)).toEqual([
      {
        state: "succeeded",
        browser: "srflx udp 11.22.33.144:51234",
        core: "srflx udp 11.22.33.144:40001",
        isSelected: true,
      },
      {
        state: "failed",
        browser: "host udp 3f2a.local:51233",
        core: "host udp 192.168.1.199:8443",
      },
      {
        state: "failed",
        browser: "srflx udp 11.22.33.144:51234",
        core: "host udp 172.17.0.2:40000",
      },
    ]);
  });

  it("finds the selected pair by the transport's word, as Chrome gives it, or a nominated success", () => {
    const chrome = hairpinned.map((entry) =>
      entry.type === "candidate-pair" ? { ...entry, selected: undefined } : entry,
    );
    const byTransport = [...chrome, { id: "T1", type: "transport", selectedCandidatePairId: "P3" }];
    expect(candidatePairsOf(byTransport)[0]).toMatchObject({
      core: "host udp 192.168.1.199:8443",
      isSelected: true,
    });
    expect(candidatePairsOf(chrome)[0]).toMatchObject({
      core: "srflx udp 11.22.33.144:40001",
      isSelected: true,
    });
  });

  it("reports at most the pairs the core takes", () => {
    const many = Array.from({ length: pairsReportedMost + 5 }, (_, index) => ({
      id: `P${String(index)}`,
      type: "candidate-pair",
      state: "waiting",
    }));
    expect(candidatePairsOf(many)).toHaveLength(pairsReportedMost);
  });

  it("are the pairs of the transport's first connection, or none from one that cannot say", async () => {
    const channel: ChannelLike = {
      readyState: "open",
      send: () => undefined,
      close: () => undefined,
      listen: () => undefined,
    };
    const pairs = [
      { state: "succeeded", browser: "host udp a.local:1", core: "host udp 192.0.2.5:8443" },
    ];
    const saying = new RtcTransport([
      {
        createDataChannel: () => channel,
        close: () => undefined,
        candidatePairs: () => Promise.resolve(pairs),
      },
    ]);
    expect(await saying.candidatePairs()).toEqual(pairs);
    const silent = new RtcTransport([{ createDataChannel: () => channel, close: () => undefined }]);
    expect(await silent.candidatePairs()).toEqual([]);
  });
});
