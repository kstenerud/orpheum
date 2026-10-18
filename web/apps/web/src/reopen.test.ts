// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { ApiProblem } from "./problem.ts";
import { askUntilAnswered, isPlaybackGone, mayPass, reopenWaitsMs } from "./reopen.ts";

/** A refusal with code, as the SDK reads one. */
function refused(code: string): ApiProblem {
  return new ApiProblem({ code, message: "a note for developers" });
}

describe("opening a lost playback again", () => {
  it("asks again while the server does not answer or is restarting, and stops at what cannot pass", async () => {
    const waited: number[] = [];
    const wait = (ms: number): Promise<void> => {
      waited.push(ms);
      return Promise.resolve();
    };
    const failures: Error[] = [new TypeError("Failed to fetch"), refused("not_answering")];
    const answered = await askUntilAnswered(() => {
      const failure = failures.shift();
      return failure === undefined ? Promise.resolve("opened") : Promise.reject(failure);
    }, wait);
    expect(answered).toBe("opened");
    expect(waited).toEqual(reopenWaitsMs.slice(0, 2));

    waited.length = 0;
    const forbidden = refused("forbidden");
    await expect(askUntilAnswered(() => Promise.reject(forbidden), wait)).rejects.toBe(forbidden);
    expect(waited).toEqual([]);
  });

  it("gives up after about a minute of no answer, with the last failure", async () => {
    let asks = 0;
    const down = new TypeError("Failed to fetch");
    await expect(
      askUntilAnswered(
        () => {
          asks += 1;
          return Promise.reject(down);
        },
        () => Promise.resolve(),
      ),
    ).rejects.toBe(down);
    expect(asks).toBe(reopenWaitsMs.length + 1);
    const totalMs = reopenWaitsMs.reduce((sum, ms) => sum + ms, 0);
    expect(totalMs).toBeGreaterThanOrEqual(50_000);
    expect(totalMs).toBeLessThanOrEqual(90_000);
  });

  it("knows a lost playback, and the failures that pass, by the server's code", () => {
    expect(isPlaybackGone(refused("playback_gone"))).toBe(true);
    expect(isPlaybackGone(refused("not_found"))).toBe(false);
    expect(mayPass(refused("migrating"))).toBe(true);
    expect(mayPass(refused("playback_gone"))).toBe(false);
    expect(mayPass(new Error("something else"))).toBe(false);
  });
});
