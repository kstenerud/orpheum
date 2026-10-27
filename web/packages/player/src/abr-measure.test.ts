// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import Hls, { Events, Fragment, PlaylistLevelType } from "hls.js";

import { orpheumAbr } from "./abr.ts";
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
    expect(abr.choosing()?.linkKbps()).toBeCloseTo(8400, 0);
    hls.destroy();
  });
});
