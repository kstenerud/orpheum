// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import Hls from "hls.js";

import { diagnose, type DiagnosedHls } from "./diagnosis.ts";
import { bufferedTextMost, PlayReporter, ReportRefused, type PlayReport } from "./report.ts";
import { settled } from "./test-timing.ts";

/** A counter of report numbers, as one playback's reporters share. */
function counter(): () => number {
  let last = 0;
  return () => ++last;
}

/** A reporter whose clock the test sets and whose reports it keeps; sending fails when told to. */
function fakeReporter(nextReportSeq: () => number = counter()): {
  reporter: PlayReporter;
  sent: PlayReport[];
  setNow: (ms: number) => void;
  failNext: () => void;
  refuseNext: () => void;
  attempts: () => number;
} {
  let now = 0;
  let shouldFail = false;
  let shouldRefuse = false;
  let attempts = 0;
  const sent: PlayReport[] = [];
  const reporter = new PlayReporter(
    (report) => {
      attempts += 1;
      if (shouldRefuse) {
        shouldRefuse = false;
        return Promise.reject(new ReportRefused(new Error("no such playback")));
      }
      if (shouldFail) {
        shouldFail = false;
        return Promise.reject(new Error("offline"));
      }
      sent.push(report);
      return Promise.resolve();
    },
    nextReportSeq,
    () => now,
  );
  return {
    reporter,
    sent,
    setNow: (ms) => {
      now = ms;
    },
    failNext: () => {
      shouldFail = true;
    },
    refuseNext: () => {
      shouldRefuse = true;
    },
    attempts: () => attempts,
  };
}

describe("reporting play state", () => {
  it("reports each change with the span it closes, numbered in order", async () => {
    const { reporter, sent, setNow } = fakeReporter();
    setNow(1000);
    reporter.recordStarted(0);
    await settled();
    setNow(31_000);
    reporter.recordSeeked(30_000, 90_000);
    await settled();
    setNow(41_000);
    reporter.recordPaused(100_000);
    await settled();
    expect(sent.map((report) => report.reportSeq)).toEqual([1, 2, 3]);
    expect(sent[0]?.events).toEqual([{ kind: "started", positionMs: 0, happenedMonoMs: 1000 }]);
    expect(sent[1]?.spans).toEqual([
      { fromPositionMs: 0, toPositionMs: 30_000, startedMonoMs: 1000, endedMonoMs: 31_000 },
    ]);
    expect(sent[2]?.spans).toEqual([
      { fromPositionMs: 90_000, toPositionMs: 100_000, startedMonoMs: 31_000, endedMonoMs: 41_000 },
    ]);
    expect(sent[2]).toMatchObject({
      positionMs: 100_000,
      sentMonoMs: 41_000,
      positionReadMonoMs: 41_000,
    });
  });

  it("does not open a span for a seek while paused", async () => {
    const { reporter, sent, setNow } = fakeReporter();
    reporter.recordSeeked(0, 60_000);
    await settled();
    setNow(5000);
    reporter.recordPaused(60_000);
    await settled();
    expect(sent.flatMap((report) => report.spans)).toEqual([]);
  });

  it("sends what a failed report carried in the next one", async () => {
    const { reporter, sent, setNow, failNext } = fakeReporter();
    setNow(1000);
    reporter.recordStarted(0);
    await settled();
    failNext();
    setNow(11_000);
    reporter.recordPaused(10_000);
    await settled();
    reporter.flush(10_000);
    await settled();
    expect(sent.map((report) => report.reportSeq)).toEqual([1, 3]);
    expect(sent[1]?.events.map((event) => event.kind)).toEqual(["paused"]);
    expect(sent[1]?.spans).toHaveLength(1);
  });

  it("sends a pause's and a seek's context with them, and each path move with the next report, again after one that failed", async () => {
    const { reporter, sent, setNow, failNext } = fakeReporter();
    setNow(1000);
    reporter.recordStarted(0);
    await settled();
    reporter.recordPathMove("WebRTC");
    failNext();
    setNow(5000);
    reporter.recordPaused(4000, { isPageHidden: true, mediaSessionAction: "pause", readyState: 4 });
    await settled();
    setNow(6000);
    reporter.recordSeeked(4000, 4251, { skippedBy: "hole", readyState: 3 });
    await settled();
    const paused = sent.flatMap((report) => report.events).find((event) => event.kind === "paused");
    const seeked = sent.flatMap((report) => report.events).find((event) => event.kind === "seeked");
    expect(paused?.context).toEqual({
      isPageHidden: true,
      mediaSessionAction: "pause",
      readyState: 4,
    });
    expect(seeked?.context).toEqual({ skippedBy: "hole", readyState: 3 });
    expect(sent.flatMap((report) => report.pathMoves ?? [])).toEqual([
      { to: "WebRTC", happenedMonoMs: 1000 },
    ]);
    const started = sent
      .flatMap((report) => report.events)
      .find((event) => event.kind === "started");
    expect(started).not.toHaveProperty("context");
  });

  it("tells a stall at once with the link's speed, and again after a report that failed", async () => {
    const { reporter, sent, setNow, failNext } = fakeReporter();
    const link: { kbps: number | undefined } = { kbps: undefined };
    reporter.measureLinkWith(() => link.kbps);
    setNow(1000);
    reporter.recordStarted(0);
    await settled();
    expect(sent[0]).not.toHaveProperty("linkKbps"); // not measured yet
    link.kbps = 1834.6;
    failNext();
    reporter.recordStall(61_500, { readyState: 1, paused: true, error: null });
    await settled();
    reporter.flush(62_000);
    await settled();
    expect(sent[1]?.stalls).toEqual([{ positionMs: 61_500, readyState: 1, isPaused: true }]);
    expect(sent[1]?.linkKbps).toBe(1835);
    reporter.flush(70_000);
    await settled();
    expect(sent[2]).not.toHaveProperty("stalls");
  });

  it("tells a stall only what its body takes, its buffered ranges cut to the server's length", async () => {
    const { reporter, sent, setNow } = fakeReporter();
    setNow(1000);
    reporter.recordStarted(0);
    await settled();
    const listeners = new Map<string, (event: string, data: unknown) => void>();
    const hls = {
      on: (event: string, listener: (event: string, data: unknown) => void) => {
        listeners.set(event, listener);
      },
    } as unknown as DiagnosedHls;
    const video = Object.assign(new EventTarget(), { readyState: 1, paused: false, error: null });
    diagnose(hls, video);
    const many = Array.from({ length: 60 }, (_, index): [number, number] => [
      index * 10,
      index * 10 + 5,
    ]);
    listeners.get(Hls.Events.BUFFER_APPENDED)!("", {
      timeRanges: {
        video: {
          length: many.length,
          start: (index: number) => many[index]![0],
          end: (index: number) => many[index]![1],
        },
      },
    });
    listeners.get(Hls.Events.FRAG_BUFFERED)!("", { id: "main", frag: { sn: 1, start: 0 } });
    reporter.recordStall(61_500, video);
    await settled();
    const [stall] = sent[1]?.stalls ?? [];
    expect(Object.keys(stall ?? {}).sort()).toEqual(
      [
        "audioBuffered",
        "isPaused",
        "lastError",
        "positionMs",
        "readyState",
        "videoBuffered",
      ].sort(),
    );
    expect(stall?.videoBuffered?.length).toBeLessThanOrEqual(bufferedTextMost);
    expect(stall?.videoBuffered?.endsWith("590.00-595.00")).toBe(true);
  });

  it("tells each quality its device cannot play at once, once, and again after a report that failed (D266)", async () => {
    const { reporter, sent, failNext } = fakeReporter();
    failNext();
    reporter.recordUnplayable({ qualityKbps: 36_208, positionMs: 420_500 }, 420_500);
    // A second, while the first's report is still on its way: kept, not in the first's place.
    reporter.recordUnplayable({ qualityKbps: 20_104, positionMs: 421_000 }, 421_000);
    await settled();
    reporter.flush(421_000);
    await settled();
    expect(sent[0]?.unplayables).toEqual([
      { qualityKbps: 36_208, positionMs: 420_500 },
      { qualityKbps: 20_104, positionMs: 421_000 },
    ]);
    reporter.flush(421_500);
    await settled();
    expect(sent[1]).not.toHaveProperty("unplayables");
  });

  it("tells where it stood still at once, once, and again after a report that failed", async () => {
    const { reporter, sent, failNext } = fakeReporter();
    const still = {
      positionMs: 0,
      forMs: 5000,
      readyState: 4,
      networkState: 1,
      buffered: "0.00-31.20",
      lastError: "",
      videoWidth: 0,
      videoHeight: 0,
      isMuted: false,
      volume: 1,
      isPageHidden: false,
    };
    failNext();
    reporter.recordStandstill(still);
    await settled();
    reporter.flush(0);
    await settled();
    expect(sent[0]?.standstills).toEqual([still]);
    reporter.flush(0);
    await settled();
    expect(sent[1]).not.toHaveProperty("standstills");
  });

  it("sends nothing more once a report is refused for good", async () => {
    const { reporter, sent, refuseNext, attempts } = fakeReporter();
    refuseNext();
    reporter.recordStarted(0);
    await settled();
    reporter.recordPaused(5000);
    reporter.flush(5000);
    await settled();
    expect(attempts()).toBe(1);
    expect(sent).toEqual([]);
  });

  it("says nothing when left before it played", async () => {
    const { reporter, sent } = fakeReporter();
    reporter.recordAbandoned(0);
    await settled();
    expect(sent).toEqual([]);
  });

  it("numbers on across a second reporter of the same playback", async () => {
    const shared = counter();
    const first = fakeReporter(shared);
    first.reporter.recordStarted(0);
    await settled();
    const second = fakeReporter(shared);
    second.reporter.recordStarted(0);
    await settled();
    expect([...first.sent, ...second.sent].map((report) => report.reportSeq)).toEqual([1, 2]);
  });
});
