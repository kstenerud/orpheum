// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { deliveries, rewritten, rewrittenMessage } from "./local-paths.ts";

describe("rewritten", () => {
  it("finds a packet that arrived from another address than it left", () => {
    const sent = [
      { from: "172.17.0.2", to: "172.18.0.1", arrivedFrom: "172.17.0.2" },
      { from: "172.18.0.1", to: "172.17.0.2", arrivedFrom: "172.17.0.2" },
    ];
    expect(rewritten(sent)).toEqual([sent[1]]);
  });

  it("names the pair and the fix", () => {
    const message = rewrittenMessage([
      { from: "172.18.0.1", to: "172.17.0.2", arrivedFrom: "172.17.0.2" },
    ]);
    expect(message).toContain("sent from 172.18.0.1 to 172.17.0.2, arrived from 172.17.0.2");
    expect(message).toContain("iptables -t nat -I POSTROUTING 1 -o lo -j RETURN");
  });
});

describe("deliveries", () => {
  it("reports where a packet between two of the machine's addresses arrived from", async () => {
    // Loopback holds all of 127.0.0.0/8, and no NAT rule touches it.
    const found = await deliveries(["127.0.0.1", "127.0.0.2"]);
    expect(found).toEqual([
      { from: "127.0.0.1", to: "127.0.0.2", arrivedFrom: "127.0.0.1" },
      { from: "127.0.0.2", to: "127.0.0.1", arrivedFrom: "127.0.0.2" },
    ]);
  });
});
