// SPDX-License-Identifier: MPL-2.0

import { SilentRequestError } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import { ApiProblem } from "./problem.ts";
import {
  askUntilAnswered,
  askWhileUnanswered,
  isPlaybackGone,
  isUnanswered,
  mayPass,
  reopenWaitsMs,
  type AskAttempt,
} from "./reopen.ts";

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

  it("tells onAttempt how long each try of askUntilAnswered took and what happened", async () => {
    const attempts: AskAttempt[] = [];
    let nowMs = 0;
    const failures: Error[] = [new TypeError("Failed to fetch")];
    const answered = await askUntilAnswered(
      () => {
        const failure = failures.shift();
        return failure === undefined ? Promise.resolve("opened") : Promise.reject(failure);
      },
      (ms) => {
        nowMs += ms;
        return Promise.resolve();
      },
      {
        onAttempt: (attempt) => {
          attempts.push(attempt);
        },
        nowMs: () => nowMs,
      },
    );
    expect(answered).toBe("opened");
    expect(attempts).toEqual([
      { ms: 0, outcome: "TypeError: Failed to fetch" },
      { ms: 0, outcome: "ok" },
    ]);
  });

  it("tells onAttempt how long each try of askWhileUnanswered took and what happened", async () => {
    const attempts: AskAttempt[] = [];
    const silent = new SilentRequestError("/api/v1/playback");
    let isAnswered = false;
    const reported: unknown[] = [];
    let nowMs = 0;
    const answered = await askWhileUnanswered(
      () => {
        nowMs += 1200;
        if (isAnswered) {
          return Promise.resolve("opened");
        }
        isAnswered = true;
        return Promise.reject(silent);
      },
      (cause) => {
        reported.push(cause);
      },
      {
        onAttempt: (attempt) => {
          attempts.push(attempt);
        },
        nowMs: () => nowMs,
      },
    );
    expect(answered).toBe("opened");
    expect(reported).toEqual([silent]);
    expect(attempts).toEqual([
      { ms: 1200, outcome: `SilentRequestError: ${silent.message}` },
      { ms: 1200, outcome: "ok" },
    ]);
  });

  it("knows a request the core answered nothing for from the loader's own copy of the SDK, by its name, named by a path or not", () => {
    // The loader's class, of another build than this client's: not this copy's SilentRequestError.
    class LoadersSilentRequestError extends Error {
      constructor() {
        super("nothing came for /api/v1/playback in 40000 ms");
        this.name = "SilentRequestError";
      }
    }
    const loaders = new LoadersSilentRequestError();
    expect(isUnanswered(loaders)).toBe(true);
    expect(isUnanswered(new Error(`${loaders.message} (over WebRTC)`, { cause: loaders }))).toBe(
      true,
    );
    expect(isUnanswered(new TypeError("Failed to fetch"))).toBe(false);
  });

  it("knows a lost playback, and the failures that pass, by the server's code", () => {
    expect(isPlaybackGone(refused("playbackGone"))).toBe(true);
    expect(isPlaybackGone(refused("notFound"))).toBe(false);
    expect(mayPass(refused("migrating"))).toBe(true);
    const away = new ApiProblem({
      code: "mediaUnreachable",
      message: "a note for developers",
      detail: { reason: "nodeAway", nodeName: "nas" },
    });
    expect(mayPass(away)).toBe(true); // heard again within seconds of a restart
    const missing = new ApiProblem({
      code: "mediaUnreachable",
      message: "a note for developers",
      detail: { reason: "rootMissing", nodeName: "nas", path: "/films" },
    });
    expect(mayPass(missing)).toBe(false);
    expect(mayPass(refused("playbackGone"))).toBe(false);
    expect(mayPass(new Error("something else"))).toBe(false);
  });
});
