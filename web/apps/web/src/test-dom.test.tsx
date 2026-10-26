// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  failOnUnexpectedRequests,
  servedBy,
  settleAnswers,
  settleUntil,
  unanswered,
  unreachable,
} from "./test-dom.tsx";

/**
 * Advances vitest's fake clock by totalMs, in chunks rather than one call: a single large
 * advanceTimersByTimeAsync left a loop that awaits React's act() around each of its own timers
 * never resolving (observed directly: a 4-turn settleUntil hung the same as a 6000-turn one, and
 * stopped hanging the moment act() was removed), where many small advances did not.
 */
async function advanceByChunks(totalMs: number, chunkMs = 5): Promise<void> {
  let advanced = 0;
  while (advanced < totalMs) {
    const step = Math.min(chunkMs, totalMs - advanced);
    await vi.advanceTimersByTimeAsync(step);
    advanced += step;
  }
}

/** Real timers back, whether a fake-timer test passed or threw. */
afterEach(() => {
  vi.useRealTimers();
});

describe("settleUntil, the wait for what outlasts a page's answers", () => {
  it("returns once its condition holds", async () => {
    let turns = 0;
    await settleUntil(() => {
      turns += 1;
      return turns > 3;
    });
    expect(turns).toBeGreaterThan(3);
  });

  it(
    "outlasts the old 2 s bound: a condition true only after 5 s still resolves " +
      "(finding 3, test-architecture-2026-10-26.md)",
    async () => {
      vi.useFakeTimers();
      let turns = 0;
      const settled = settleUntil(() => {
        turns += 1;
        return turns > 500; // 500 * 10 ms = 5 s: past the old 2 s bound, within the new one.
      });
      await advanceByChunks(5_500);
      await expect(settled).resolves.toBeUndefined();
    },
  );

  it("fails the test, naming the condition, when it never holds (the audit of 2026-10-25, W11)", async () => {
    vi.useFakeTimers();
    const isShown = false;
    const settled = settleUntil(() => isShown);
    // Attached before advancing: a rejection with no handler yet is reported unhandled even once
    // one is attached later, which failed the suite here though this very assertion caught it.
    const rejected = expect(settled).rejects.toThrow(
      /settleUntil: this never held within about 60 s: .*isShown/,
    );
    await advanceByChunks(60_000);
    await rejected;
  });
});

describe("servedBy, the stand-in server (finding 2, test-architecture-2026-10-26.md)", () => {
  it("fails the test, naming it, when nothing in Answers answers a request", async () => {
    const { api } = servedBy({});
    await api.GET("/me").catch(() => undefined);
    expect(() => {
      failOnUnexpectedRequests();
    }).toThrow(/the test has no answer for GET \/me|GET \/me/);
  });

  it("fails the test, naming it, when a request reaches outside the API's prefix", async () => {
    const { fetchMedia } = servedBy({});
    await fetchMedia("/playlist.m3u8", {}).catch(() => undefined);
    expect(() => {
      failOnUnexpectedRequests();
    }).toThrow(/playlist\.m3u8/);
  });

  it("does not fail the test for a request declared unreachable: that is a test's own choice", async () => {
    const { api } = servedBy({ "GET /me": unreachable });
    await api.GET("/me").catch(() => undefined);
    expect(() => {
      failOnUnexpectedRequests();
    }).not.toThrow();
  });
});

describe("settleAnswers, the wait after a request answers (finding 3, test-architecture-2026-10-26.md)", () => {
  it("does not wait on a request answered unanswered: it is left pending on purpose", async () => {
    const { api } = servedBy({ "GET /me": [unanswered] });
    void api.GET("/me");
    await expect(settleAnswers()).resolves.toBeUndefined();
    failOnUnexpectedRequests();
  });

  it("fails, naming the DOM, when it never stops changing", async () => {
    vi.useFakeTimers();
    const spinner = document.createElement("div");
    document.body.append(spinner);
    let turns = 0;
    const spinning = setInterval(() => {
      turns += 1;
      spinner.textContent = String(turns);
    }, 5);
    try {
      const settled = settleAnswers();
      const rejected = expect(settled).rejects.toThrow(
        /settleAnswers: still waiting after about 60 s: the DOM kept changing/,
      );
      await advanceByChunks(60_000);
      await rejected;
    } finally {
      clearInterval(spinning);
      spinner.remove();
    }
  });
});
