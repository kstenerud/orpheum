// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { probeIpv6 } from "./ipv6-probe.ts";

const probe = "https://v6probe.zone.example/";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

/** A fetch that answers after afterMs, or fails then, or never ends until aborted. */
function sending(
  ends: "answers" | "fails" | "never",
  afterMs = 0,
  asked: RequestInit[] = [],
): typeof fetch {
  return (_input, init) => {
    asked.push(init ?? {});
    return new Promise((resolve, reject) => {
      init?.signal?.addEventListener("abort", () => {
        reject(new DOMException("aborted", "AbortError"));
      });
      if (ends === "never") {
        return;
      }
      setTimeout(() => {
        if (ends === "answers") {
          resolve(new Response(null, { status: 502 }));
        } else {
          reject(new TypeError("Failed to fetch"));
        }
      }, afterMs);
    });
  };
}

/** What the probe judged, the fake clock run until it ended. */
async function judged(send: typeof fetch): Promise<string> {
  const judging = probeIpv6(probe, send);
  await vi.runAllTimersAsync();
  return await judging;
}

describe("the discovery's IPv6 probe name (D398 §7)", () => {
  it("judges IPv6 here when anything answers, of any status, asked with no cookie, cache or CORS", async () => {
    const asked: RequestInit[] = [];
    expect(await judged(sending("answers", 0, asked))).toBe("yes");
    expect(await judged(sending("answers", 2500))).toBe("yes");
    expect(asked[0]).toMatchObject({ mode: "no-cors", cache: "no-store", credentials: "omit" });
  });

  it("judges no IPv6 here when it fails at once, as a family with no route does", async () => {
    expect(await judged(sending("fails", 0))).toBe("no");
    expect(await judged(sending("fails", 900))).toBe("no");
    // A fetch that throws rather than rejects fails at once too.
    expect(
      await judged(() => {
        throw new TypeError("refused");
      }),
    ).toBe("no");
  });

  it("cannot tell when it fails late, or nothing answers in the wait, which it gives up", async () => {
    expect(await judged(sending("fails", 1100))).toBe("unknown");
    const asked: RequestInit[] = [];
    const judging = probeIpv6(probe, sending("never", 0, asked));
    await vi.advanceTimersByTimeAsync(3000);
    expect(await judging).toBe("unknown");
    expect(asked[0]?.signal?.aborted).toBe(true);
  });
});
