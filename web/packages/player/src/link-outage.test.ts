// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { deeperAfterOutageAtLeastMs, deeperForMs, LinkOutages, outageText } from "./link-outage.ts";

/** Outages with one of lastedMs, from 1000 ms, its first sign why, already back. */
function afterOutageOf(lastedMs: number, why = "its loading stopped"): LinkOutages {
  const outages = new LinkOutages();
  outages.lost(why, 1000);
  outages.lost("a chunk failed (fragLoadError)", 1500);
  outages.back(1000 + lastedMs);
  return outages;
}

// The owner's phone, out 6.4 minutes mid-film, held 30 s and stalled (staging, 2026-11-14).
describe("the page's outages (D487)", () => {
  it("has nothing load deeper before any outage, nor after one shorter than deeperAfterOutageAtLeastMs", () => {
    expect(new LinkOutages().deeperAt(0)).toBeUndefined();
    const short = afterOutageOf(deeperAfterOutageAtLeastMs - 1);
    expect(short.deeperAt(deeperAfterOutageAtLeastMs + 1000)).toBeUndefined();
  });

  it("has a playback load deeper once one of deeperAfterOutageAtLeastMs ends, by its first sign, and not while it lasts", () => {
    const outages = new LinkOutages();
    outages.lost("its loading stopped", 1000);
    outages.lost("a chunk failed (fragLoadError)", 1500);
    expect(outages.deeperAt(1000 + deeperAfterOutageAtLeastMs)).toBeUndefined();
    const started = outages.back(1000 + deeperAfterOutageAtLeastMs);
    const outage = {
      lastedMs: deeperAfterOutageAtLeastMs,
      endedAtMs: 1000 + deeperAfterOutageAtLeastMs,
      why: "its loading stopped",
    };
    expect(started).toEqual(outage);
    expect(outages.deeperAt(1000 + deeperAfterOutageAtLeastMs)).toEqual(outage);
    expect(outageText(outage)).toBe("out 10.0 s (its loading stopped)");
  });

  it("loads deeper until the link has stayed good deeperForMs since the outage ended", () => {
    const outages = afterOutageOf(381_300);
    const endedAtMs = 1000 + 381_300;
    expect(outages.deeperAt(endedAtMs + deeperForMs - 1)?.lastedMs).toBe(381_300);
    expect(outages.deeperAt(endedAtMs + deeperForMs)).toBeUndefined();
  });

  it("stops counting the link's time good through an outage under way, and starts afresh from any outage that ends while it loads deeper", () => {
    const outages = afterOutageOf(381_300);
    const endedAtMs = 1000 + 381_300;
    outages.lost("the browser offline", endedAtMs + 60_000);
    // Out longer than deeperForMs: it loads deeper still, its good time counted to the outage's start.
    expect(outages.deeperAt(endedAtMs + 60_000 + deeperForMs)).toBeDefined();
    // A short one ends, which starts no deeper loading of its own, but counts the good time afresh.
    const backAtMs = endedAtMs + 60_000 + 2000;
    expect(outages.back(backAtMs)).toBeUndefined();
    expect(outages.deeperAt(backAtMs + deeperForMs - 1)).toEqual({
      lastedMs: 2000,
      endedAtMs: backAtMs,
      why: "the browser offline",
    });
    expect(outages.deeperAt(backAtMs + deeperForMs)).toBeUndefined();
  });

  it("has a short outage after the deeper loading ended start none, and a long one start it again", () => {
    const outages = afterOutageOf(381_300);
    const overAtMs = 1000 + 381_300 + deeperForMs;
    outages.lost("its loading stopped", overAtMs + 1000);
    expect(outages.deeperAt(overAtMs + 2000)).toBeUndefined();
    expect(outages.back(overAtMs + 2000)).toBeUndefined();
    expect(outages.deeperAt(overAtMs + 2000)).toBeUndefined();
    outages.lost("its loading stopped", overAtMs + 3000);
    expect(outages.back(overAtMs + 3000 + deeperAfterOutageAtLeastMs)?.why).toBe(
      "its loading stopped",
    );
  });

  it("ends no outage when the link is back without one", () => {
    const outages = new LinkOutages();
    expect(outages.back(5000)).toBeUndefined();
    expect(outages.deeperAt(5000)).toBeUndefined();
  });
});
