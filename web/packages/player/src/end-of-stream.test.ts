// SPDX-License-Identifier: MPL-2.0

import Hls from "hls.js";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { endAtTheLastFrame, type EndingHls, type EndingMedia } from "./end-of-stream.ts";

/** Ranges as a video's buffered gives them. */
function ranges(...spans: readonly (readonly [number, number])[]): TimeRanges {
  return {
    length: spans.length,
    start: (index: number) => spans[index]?.[0] ?? Number.NaN,
    end: (index: number) => spans[index]?.[1] ?? Number.NaN,
  };
}

/** A playlist whose last chunk ends at edge, and every BUFFER_EOS it was sent. */
function playlistHls(edge: number, live = false): EndingHls & { readonly sent: unknown[] } {
  const sent: unknown[] = [];
  return {
    levels: [{ details: { live, edge } }],
    currentLevel: 0,
    trigger: (event) => sent.push(event),
    sent,
  };
}

/** A video playing at currentTime, holding buffered. */
function playing(currentTime: number, buffered: TimeRanges): EndingMedia {
  return { currentTime, paused: false, seeking: false, ended: false, buffered };
}

describe("endAtTheLastFrame", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  // The Fire TV, 2026-11-02: The Pumbletons' S33E8 stood at 1299.967 of 1300.056, all of it loaded
  // past a two-frame hole at 1280.8, its media source never ended, and no next episode came.
  it("ends the stream of a video standing still at the end of a whole film loaded past a hole", () => {
    const hls = playlistHls(1300.054);
    const stop = endAtTheLastFrame(
      hls,
      playing(1299.967, ranges([1270.44, 1280.784], [1280.868, 1300.054])),
    );
    vi.advanceTimersByTime(1000);
    stop();
    expect(hls.sent).toEqual([Hls.Events.BUFFER_EOS]);
  });

  it("ends it once, however long the video stands there", () => {
    const hls = playlistHls(1300.054);
    const stop = endAtTheLastFrame(hls, playing(1299.967, ranges([1270.44, 1300.054])));
    vi.advanceTimersByTime(10_000);
    stop();
    expect(hls.sent).toHaveLength(1);
  });

  it("leaves alone a video waiting mid-film, one loaded short of the end, a live one and a paused one", () => {
    const midFilm = playlistHls(1300.054);
    const shortOfTheEnd = playlistHls(1300.054);
    const live = playlistHls(1300.054, true);
    const paused = playlistHls(1300.054);
    const stops = [
      endAtTheLastFrame(midFilm, playing(640.2, ranges([600, 640.3]))),
      endAtTheLastFrame(shortOfTheEnd, playing(1297.5, ranges([1270.44, 1297.593]))),
      endAtTheLastFrame(live, playing(1299.967, ranges([1270.44, 1300.054]))),
      endAtTheLastFrame(paused, {
        ...playing(1299.967, ranges([1270.44, 1300.054])),
        paused: true,
      }),
    ];
    vi.advanceTimersByTime(10_000);
    for (const stop of stops) {
      stop();
    }
    expect([midFilm.sent, shortOfTheEnd.sent, live.sent, paused.sent]).toEqual([[], [], [], []]);
  });

  it("ends it again after a seek back, which reopens the source", () => {
    const hls = playlistHls(1300.054);
    const media = { ...playing(1299.967, ranges([1270.44, 1300.054])) };
    const stop = endAtTheLastFrame(hls, media);
    vi.advanceTimersByTime(1000);
    Object.assign(media, { seeking: true, currentTime: 1280 });
    vi.advanceTimersByTime(500);
    Object.assign(media, { seeking: false, currentTime: 1299.967 });
    vi.advanceTimersByTime(1000);
    stop();
    expect(hls.sent).toHaveLength(2);
  });

  it("leaves alone a video still moving toward its end", () => {
    const hls = playlistHls(1300.054);
    const media = { ...playing(1299.6, ranges([1270.44, 1300.054])) };
    const stop = endAtTheLastFrame(hls, media);
    for (let look = 0; look < 6; look += 1) {
      media.currentTime += 0.07;
      vi.advanceTimersByTime(500);
    }
    stop();
    expect(hls.sent).toEqual([]);
  });
});
