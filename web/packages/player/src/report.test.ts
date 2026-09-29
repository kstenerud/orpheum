// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { PlayReporter, ReportRefused, type PlayReport } from "./report.ts";

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

async function settled(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
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
