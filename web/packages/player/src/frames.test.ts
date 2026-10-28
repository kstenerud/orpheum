// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { framesShownOf, type FramesShown } from "./frames.ts";
import { PlayReporter, type PlayReport } from "./report.ts";
import { settled } from "./test-timing.ts";

/** A reporter whose reports are kept, sending failing once when told, its video's frames as set. */
function framesReporter(): {
  readonly reporter: PlayReporter;
  readonly sent: PlayReport[];
  readonly show: (frames: FramesShown | undefined) => void;
  readonly failNext: () => void;
} {
  let frames: FramesShown | undefined = { total: 0, dropped: 0 };
  let shouldFail = false;
  let seq = 0;
  const sent: PlayReport[] = [];
  const reporter = new PlayReporter(
    (report) => {
      if (shouldFail) {
        shouldFail = false;
        return Promise.reject(new Error("offline"));
      }
      sent.push(report);
      return Promise.resolve();
    },
    () => ++seq,
    () => 0,
  );
  reporter.reportFramesWith(() => frames);
  return {
    reporter,
    sent,
    show: (now) => {
      frames = now;
    },
    failNext: () => {
      shouldFail = true;
    },
  };
}

describe("the frames a video showed and dropped, in its reports (the owner, 2026-10-28)", () => {
  it("reads the browser's count, and nothing where it keeps none", () => {
    expect(
      framesShownOf({
        getVideoPlaybackQuality: () => ({ totalVideoFrames: 240, droppedVideoFrames: 3 }),
      }),
    ).toEqual({ total: 240, dropped: 3 });
    expect(framesShownOf({})).toBeUndefined();
  });

  it("reports the frames since the last report, so a burst shows near when it happened", async () => {
    const { reporter, sent, show } = framesReporter();
    show({ total: 240, dropped: 1 });
    reporter.flush(10_000);
    await settled();
    show({ total: 480, dropped: 31 });
    reporter.flush(20_000);
    await settled();
    expect(sent.map(({ totalFrames, droppedFrames }) => ({ totalFrames, droppedFrames }))).toEqual([
      { totalFrames: 240, droppedFrames: 1 },
      { totalFrames: 240, droppedFrames: 30 },
    ]);
  });

  it("counts a report that failed to send in the next, and a count started again from zero", async () => {
    const { reporter, sent, show, failNext } = framesReporter();
    show({ total: 240, dropped: 2 });
    failNext();
    reporter.flush(10_000);
    await settled();
    show({ total: 480, dropped: 4 });
    reporter.flush(20_000);
    await settled();
    show({ total: 100, dropped: 0 }); // a new source: the browser counts from zero again
    reporter.flush(30_000);
    await settled();
    expect(sent.map(({ totalFrames, droppedFrames }) => ({ totalFrames, droppedFrames }))).toEqual([
      { totalFrames: 480, droppedFrames: 4 },
      { totalFrames: 100, droppedFrames: 0 },
    ]);
  });

  it("says nothing of frames where the browser counts none", async () => {
    const { reporter, sent, show } = framesReporter();
    show(undefined);
    reporter.flush(10_000);
    await settled();
    expect(sent[0]).not.toHaveProperty("totalFrames");
    expect(sent[0]).not.toHaveProperty("droppedFrames");
  });
});
