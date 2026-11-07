// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { deadEndReasonOf } from "./dead-end-reason.ts";
import { RtcUnconnected, type Target } from "@orpheum/sdk";

const lan = "https://lan.k7xq2.zone.example";
const v6 = "https://v6.k7xq2.zone.example";
const v4 = "https://v4.k7xq2.zone.example";

/** A target with the names given, seen by the discovery over seenOver. */
function targetOf(
  ipv6Urls: readonly string[],
  ipv4Urls: readonly string[],
  seenOver: "ipv6" | "ipv4" | undefined,
): Target {
  return {
    coreId: "k7xq2",
    fingerprint: "a-fingerprint",
    held: { bootstrapToken: "a-bootstrap-token" },
    key: new Uint8Array(32),
    iceServers: [],
    directUrls: [lan, ...ipv6Urls, ...ipv4Urls],
    lanUrls: [lan],
    families: { ipv6Urls, ipv4Urls, seenOver },
  };
}

const failedAtOnce = "failed: TypeError: Failed to fetch";
const neverAnswered = "failed: no answer within 3000 ms";

describe("why a page reached its core by no way (D398)", () => {
  it("judges no IPv6 only when seen over IPv4 and every IPv6 name failed at once", () => {
    const raced = [
      { url: lan, outcome: neverAnswered },
      { url: v6, outcome: failedAtOnce },
    ];
    expect(deadEndReasonOf(targetOf([v6], [], "ipv4"), raced)).toEqual({
      ipv6: "no",
      tried: [
        { kind: "lan", ended: "noAnswer" },
        { kind: "ipv6", ended: "unreachable" },
        { kind: "webrtc", ended: "failed" },
      ],
      cause: "noIpv6",
    });
  });

  // The owner's Firefox 158 (2026-11-07): ICE got through, DTLS never began; no network's want.
  it("names a WebRTC connection whose ICE got through and whose secure connection never completed", () => {
    const raced = [{ url: v6, outcome: failedAtOnce }];
    const stalled = new RtcUnconnected(
      { ice: "connected", connection: "connecting" },
      "did not connect in time",
    );
    expect(deadEndReasonOf(targetOf([v6], [], "ipv4"), raced, undefined, stalled)).toEqual({
      ipv6: "no",
      tried: [
        { kind: "ipv6", ended: "unreachable" },
        { kind: "webrtc", ended: "failed" },
      ],
      cause: "secureUnfinished",
    });
    const neverThrough = new RtcUnconnected({ ice: "failed", connection: "failed" }, "failed");
    expect(deadEndReasonOf(targetOf([], [], "ipv4"), [], undefined, neverThrough).cause).toBe(
      "unknown",
    );
  });

  it("cannot tell when an IPv6 name only went unanswered, or none was tried", () => {
    const unanswered = [{ url: v6, outcome: neverAnswered }];
    expect(deadEndReasonOf(targetOf([v6], [], "ipv4"), unanswered)).toMatchObject({
      ipv6: "unknown",
      cause: "unknown",
    });
    expect(deadEndReasonOf(targetOf([], [], "ipv4"), [])).toMatchObject({
      ipv6: "unknown",
      cause: "unknown",
    });
    // The discovery's answer named no prefix: an older discovery.
    const failed = [{ url: v6, outcome: failedAtOnce }];
    expect(deadEndReasonOf(targetOf([v6], [], undefined), failed).ipv6).toBe("unknown");
  });

  it("knows this network has IPv6 when seen over it, or an IPv6 name answered at all", () => {
    const failed = [{ url: v6, outcome: failedAtOnce }];
    expect(deadEndReasonOf(targetOf([v6], [], "ipv6"), failed).ipv6).toBe("yes");
    const refused = [{ url: v6, outcome: `failed: Error: ${v6} answered 502` }];
    expect(deadEndReasonOf(targetOf([v6], [], "ipv4"), refused)).toMatchObject({
      ipv6: "yes",
      cause: "unknown",
    });
  });

  it("names no cause where the server has an IPv4 way in, which failed as well", () => {
    const raced = [
      { url: v6, outcome: failedAtOnce },
      { url: v4, outcome: failedAtOnce },
    ];
    expect(deadEndReasonOf(targetOf([v6], [v4], "ipv4"), raced)).toEqual({
      ipv6: "no",
      tried: [
        { kind: "ipv6", ended: "unreachable" },
        { kind: "ipv4", ended: "unreachable" },
        { kind: "webrtc", ended: "failed" },
      ],
      cause: "unknown",
    });
  });

  it("takes the discovery's IPv6 probe's judgement, else judges as without one (D398 §7)", () => {
    const unanswered = [{ url: v6, outcome: neverAnswered }];
    // The probe failed at once: no IPv6 here, though the core's IPv6 name only went unanswered and
    // the discovery's answer named no prefix.
    expect(deadEndReasonOf(targetOf([v6], [], undefined), unanswered, "no")).toMatchObject({
      ipv6: "no",
      cause: "noIpv6",
    });
    // The probe answered: IPv6 here, which got no answer from the core, though its name failed at once.
    const failed = [{ url: v6, outcome: failedAtOnce }];
    expect(deadEndReasonOf(targetOf([v6], [], "ipv4"), failed, "yes")).toMatchObject({
      ipv6: "yes",
      cause: "unknown",
    });
    // Seen over IPv6 outweighs a probe that failed.
    expect(deadEndReasonOf(targetOf([v6], [], "ipv6"), unanswered, "no").ipv6).toBe("yes");
    // A probe that could not tell, or none, leaves the judgement as it was.
    expect(deadEndReasonOf(targetOf([v6], [], "ipv4"), failed, "unknown").ipv6).toBe("no");
    expect(deadEndReasonOf(targetOf([v6], [], "ipv4"), unanswered, "unknown").ipv6).toBe("unknown");
    expect(deadEndReasonOf(targetOf([v6], [], "ipv4"), unanswered, undefined).ipv6).toBe("unknown");
  });

  it("counts no WebRTC try when a name answered, or the race was never decided", () => {
    const answered = [{ url: v6, outcome: "answered" }];
    expect(deadEndReasonOf(targetOf([v6], [], "ipv6"), answered).tried).toEqual([
      { kind: "ipv6", ended: "answered" },
    ]);
    expect(deadEndReasonOf(targetOf([v6], [], "ipv6"), undefined).tried).toEqual([]);
  });
});
