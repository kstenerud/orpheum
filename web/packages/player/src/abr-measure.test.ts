// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import Hls, { Events, Fragment, PlaylistLevelType } from "hls.js";

import { orpheumAbr } from "./abr-controller.ts";
import { arrivalMs } from "./abr-choice.ts";

/** A picture chunk of level 0, sn and loaded bytes as given, timed as loading says. */
function pictureChunk(
  sn: number | "initSegment",
  loaded: number,
  loading: { readonly start: number; readonly first: number; readonly end: number },
): Fragment {
  const frag = new Fragment(PlaylistLevelType.MAIN, "");
  frag.sn = sn;
  frag.level = 0;
  frag.duration = 2.9;
  frag.stats.loaded = loaded;
  frag.stats.loading = { ...loading };
  frag.stats.parsing = { start: loading.end, end: loading.end + 50 };
  return frag;
}

describe("how the controller measures the link inside hls.js (D247 §9, D265 §7)", () => {
  it("times a chunk from its first byte to its last, at least a millisecond", () => {
    expect(arrivalMs({ first: 1600, end: 1800 })).toBe(200);
    expect(arrivalMs({ first: 1800, end: 1800 })).toBe(1);
  });

  it("measures the link by how fast a chunk's bytes arrive, not by the wait for its first (the owner, on staging, 2026-10-26)", () => {
    const abr = orpheumAbr({
      startAtBps: 0,
      monotonicMs: () => 0,
      isVoiceLive: () => false,
      fromCache: () => undefined,
    });
    const hls = new Hls({ abrController: abr.controller });
    // The picture's head, answered at once, which sets hls.js's own estimate of a first byte's wait.
    hls.trigger(Events.FRAG_LOADED, {
      frag: pictureChunk("initSegment", 1500, { start: 0, first: 60, end: 70 }),
      part: null,
      payload: new ArrayBuffer(0),
      networkDetails: null,
    });
    // Its first chunk, 210 KB, waiting 1.5 s for its first byte on a busy WebRTC transport, then
    // arriving in 200 ms: the link carries 8.4 Mb/s. hls.js's own sample read it at about 1.3 Mb/s.
    const first = pictureChunk(0, 210_000, { start: 100, first: 1600, end: 1800 });
    hls.trigger(Events.FRAG_LOADED, {
      frag: first,
      part: null,
      payload: new ArrayBuffer(0),
      networkDetails: null,
    });
    hls.trigger(Events.FRAG_BUFFERED, {
      stats: first.stats,
      frag: first,
      part: null,
      id: PlaylistLevelType.MAIN,
    });
    expect(hls.bandwidthEstimate).toBeCloseTo(8_400_000, -4);
    // The controller's own estimate times it the same, reported from its second sample (D352 §1).
    const second = pictureChunk(1, 210_000, { start: 2000, first: 3500, end: 3700 });
    hls.trigger(Events.FRAG_BUFFERED, {
      stats: second.stats,
      frag: second,
      part: null,
      id: PlaylistLevelType.MAIN,
    });
    expect(abr.choosing()?.linkKbps()).toBeCloseTo(8400, 0);
    hls.destroy();
  });

  it("measures the link by small chunks loaded back to back, which alone measure nothing (D414 §3)", () => {
    /** The link the controller reports after eight 40 KB chunks, each asked restMs after the last ended. */
    const linkAfter = (restMs: number): number | undefined => {
      const abr = orpheumAbr({
        startAtBps: 0,
        monotonicMs: () => 0,
        isVoiceLive: () => false,
        fromCache: () => false,
      });
      const hls = new Hls({ abrController: abr.controller });
      let askedMs = 0;
      for (let sn = 0; sn < 8; sn++) {
        const chunk = pictureChunk(sn, 40_000, {
          start: askedMs,
          first: askedMs + 50,
          end: askedMs + 250,
        });
        hls.trigger(Events.FRAG_BUFFERED, {
          stats: chunk.stats,
          frag: chunk,
          part: null,
          id: PlaylistLevelType.MAIN,
        });
        askedMs += 250 + restMs;
      }
      const kbps = abr.choosing()?.linkKbps();
      hls.destroy();
      return kbps;
    };
    // Two samples of 160 KB, each over 980 ms: 1306 kb/s.
    expect(linkAfter(10)).toBeCloseTo(1306, 0);
    expect(linkAfter(500)).toBeUndefined();
  });
});
