// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { PlayReporter, type PlayReport } from "./report.ts";

/** A reporter whose clock the test sets and whose reports it keeps; sending fails when told to. */
function reporter(): {
  play: PlayReporter;
  sent: PlayReport[];
  setNow: (ms: number) => void;
  failNext: () => void;
} {
  let now = 0;
  let shouldFail = false;
  const sent: PlayReport[] = [];
  const play = new PlayReporter(
    (report) => {
      if (shouldFail) {
        shouldFail = false;
        return Promise.reject(new Error("offline"));
      }
      sent.push(report);
      return Promise.resolve();
    },
    () => now,
  );
  return {
    play,
    sent,
    setNow: (ms) => {
      now = ms;
    },
    failNext: () => {
      shouldFail = true;
    },
  };
}

async function settled(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
}

describe("reporting play state", () => {
  it("reports each change with the span it closes, numbered in order", async () => {
    const { play, sent, setNow } = reporter();
    setNow(1000);
    play.started(0);
    await settled();
    setNow(31_000);
    play.seeked(30_000, 90_000);
    await settled();
    setNow(41_000);
    play.paused(100_000);
    await settled();
    expect(sent.map((report) => report.seq)).toEqual([1, 2, 3]);
    expect(sent[0]?.events).toEqual([{ kind: "started", positionMs: 0, monoMs: 1000 }]);
    expect(sent[1]?.spans).toEqual([
      { fromPositionMs: 0, toPositionMs: 30_000, startedMonoMs: 1000, endedMonoMs: 31_000 },
    ]);
    expect(sent[2]?.spans).toEqual([
      { fromPositionMs: 90_000, toPositionMs: 100_000, startedMonoMs: 31_000, endedMonoMs: 41_000 },
    ]);
    expect(sent[2]).toMatchObject({
      positionMs: 100_000,
      sentMonoMs: 41_000,
      positionMonoMs: 41_000,
    });
  });

  it("does not open a span for a seek while paused", async () => {
    const { play, sent, setNow } = reporter();
    play.seeked(0, 60_000);
    await settled();
    setNow(5000);
    play.paused(60_000);
    await settled();
    expect(sent.flatMap((report) => report.spans)).toEqual([]);
  });

  it("sends what a failed report carried in the next one", async () => {
    const { play, sent, setNow, failNext } = reporter();
    setNow(1000);
    play.started(0);
    await settled();
    failNext();
    setNow(11_000);
    play.paused(10_000);
    await settled();
    play.flush(10_000);
    await settled();
    expect(sent.map((report) => report.seq)).toEqual([1, 3]);
    expect(sent[1]?.events.map((event) => event.kind)).toEqual(["paused"]);
    expect(sent[1]?.spans).toHaveLength(1);
  });
});
