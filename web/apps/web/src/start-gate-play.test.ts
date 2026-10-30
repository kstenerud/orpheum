// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { tellNextChunkLoadMs } from "@orpheum/sdk";
import { act, createElement } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { PlayView } from "./play.tsx";
import { noteDecisionTimes } from "./play-state-video-events.ts";
import { directDecision } from "./test-fixtures.ts";
import { mount, servedBy, type Heard } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Awaited<ReturnType<typeof mount>> | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** Loaded ranges, as a video's buffered says them. */
function ranges(loaded: readonly (readonly [number, number])[]): TimeRanges {
  return {
    length: loaded.length,
    start: (index: number) => loaded[index]?.[0] ?? 0,
    end: (index: number) => loaded[index]?.[1] ?? 0,
  };
}

/** Fires event at video, and lets what it starts settle. */
async function fire(video: HTMLVideoElement, event: string): Promise<void> {
  await act(async () => {
    video.dispatchEvent(new Event(event));
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

/**
 * The Play page's video as a Fire TV's was (the owner's Marrowgate Family, staging, 2026-10-28): resumed
 * at 1798.9 s, its first chunk loaded from 1794.96 s to 1800.47 s, able to play; its next chunk, 10.4
 * s of film, expected to take 2.3 s. Its plays are counted, and what it loaded is the test's.
 */
function asTheFireTvWas(video: HTMLVideoElement): {
  readonly plays: () => number;
  readonly load: (loaded: readonly (readonly [number, number])[]) => void;
} {
  let plays = 0;
  let isPaused = true;
  let buffered = ranges([[1794.96, 1800.47]]);
  Object.defineProperty(video, "readyState", { value: 4 });
  Object.defineProperty(video, "networkState", { value: 2 }); // loading
  Object.defineProperty(video, "currentTime", { value: 1798.9, writable: true });
  Object.defineProperty(video, "paused", { get: () => isPaused });
  Object.defineProperty(video, "buffered", { get: () => buffered });
  Object.defineProperty(video, "play", {
    value: async (): Promise<void> => {
      plays += 1;
      isPaused = false;
      await Promise.resolve();
    },
  });
  Object.defineProperty(video, "pause", {
    value: () => {
      isPaused = true;
    },
  });
  tellNextChunkLoadMs(video, () => ({ loadMs: 2300, from: "confident" }));
  return {
    plays: () => plays,
    load: (loaded) => {
      buffered = ranges(loaded);
    },
  };
}

/** The Play page playing a direct film whose playback is token, its reports heard. */
async function mountPlay(token: string, heard: Heard[]): Promise<HTMLVideoElement> {
  mounted = await mount(
    withServices(
      createElement(PlayView, {
        fileId: 2,
        decision: directDecision({ mediaToken: token }),
        onDecideAgain: () => undefined,
      }),
      servedBy(
        {
          [`POST /playback/${token}/report`]: [204, undefined],
          "GET /files/2/waits": [200, {}],
          "GET /files/2/up-next": [200, {}],
          [`PUT /playback/${token}/interest`]: [204, null],
          [`DELETE /playback/${token}`]: [204, null],
        },
        heard,
      ),
    ),
  );
  const video = mounted.container.querySelector("video");
  if (video === null) {
    throw new Error("no video");
  }
  return video;
}

/** The bodies of the reports heard. */
function reportsOf(heard: readonly Heard[]): unknown[] {
  return heard
    .filter((request) => request.methodAndPath.endsWith("/report"))
    .map((request) => request.body);
}

describe("the Play page's start (the owner, 2026-10-28)", () => {
  it("plays its film only once what it loaded outlasts its next chunk's load, and its start's line says so", async () => {
    const heard: Heard[] = [];
    const askedMs = performance.now() - 3000;
    noteDecisionTimes("gated", { askedMs, decidedMs: askedMs + 200 });
    const video = await mountPlay("gated", heard);
    const fireTv = asTheFireTvWas(video);
    await fire(video, "loadstart");
    await fire(video, "loadeddata");
    await fire(video, "canplay");
    expect(fireTv.plays()).toBe(0); // 1.57 s loaded, its next chunk 2.3 s away
    fireTv.load([[1794.96, 1810.87]]);
    await fire(video, "progress");
    expect(fireTv.plays()).toBe(1);
    await fire(video, "playing");
    expect(reportsOf(heard)[0]).toMatchObject({
      startup: {
        loadedAheadMs: 11_970,
        waitedForAheadMs: 3450,
        nextChunkLoadMs: 2300,
        nextChunkLoadFrom: "confident",
      },
    });
  });

  it("holds a seek to where little is loaded until enough is, telling no pause, and its seek's line says so", async () => {
    const heard: Heard[] = [];
    const video = await mountPlay("held", heard);
    const fireTv = asTheFireTvWas(video);
    fireTv.load([[1794.96, 1810.87]]);
    await fire(video, "loadstart");
    await fire(video, "playing");
    expect(fireTv.plays()).toBe(1);
    video.currentTime = 3000; // the person's jump
    await fire(video, "seeking");
    expect(video.paused).toBe(true); // held before it can play there
    await fire(video, "pause");
    await fire(video, "canplay");
    expect(fireTv.plays()).toBe(1);
    fireTv.load([
      [1794.96, 1810.87],
      [2998, 3010],
    ]);
    await fire(video, "progress");
    expect(fireTv.plays()).toBe(2);
    await fire(video, "playing");
    const reports = reportsOf(heard);
    expect(reports).toContainEqual(
      expect.objectContaining({
        seeks: [
          expect.objectContaining({
            toPositionMs: 3_000_000,
            loadedAheadMs: 10_000,
            waitedForAheadMs: 3450,
            nextChunkLoadMs: 2300,
          }),
        ],
      }),
    );
    const kinds = reports.flatMap((report) =>
      (report as { readonly events: readonly { readonly kind: string }[] }).events.map(
        (event) => event.kind,
      ),
    );
    expect(kinds).not.toContain("paused");
  });
});
