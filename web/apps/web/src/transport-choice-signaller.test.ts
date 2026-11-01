// SPDX-License-Identifier: MPL-2.0

import {
  createApi,
  isUnreached,
  RtcTransport,
  unreachedRetryMs,
  type Description,
} from "@orpheum/sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { directSignaller } from "./transport-choice.ts";

const offer: Description = { type: "offer", sdp: "v=0" };

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

function apiWhosePostsFail(problem: () => Error): ReturnType<typeof createApi> {
  return createApi({
    origin: "http://core.test",
    getToken: () => undefined,
    fetch: () => Promise.reject(problem()),
  });
}

describe("the direct mode's signaller", () => {
  it("fails an offer that reached no server as Unreached, and an abort as an abort", async () => {
    const noNetwork = directSignaller(
      apiWhosePostsFail(() => new TypeError("NetworkError when attempting to fetch resource.")),
    );
    expect(isUnreached(await noNetwork(offer).catch((caught: unknown) => caught))).toBe(true);
    const aborted = directSignaller(
      apiWhosePostsFail(() => new DOMException("The operation was aborted.", "AbortError")),
    );
    const abortProblem = await aborted(offer).catch((caught: unknown) => caught);
    expect(isUnreached(abortProblem)).toBe(false);
  });

  it("has the transport try again after unreachedRetryMs, not its growing wait, as its offers reach nothing", async () => {
    const signal = directSignaller(
      apiWhosePostsFail(() => new TypeError("NetworkError when attempting to fetch resource.")),
    );
    const triedAtMs: number[] = [];
    let goneListener: ((why: string) => void) | undefined;
    const first = {
      isClosed: false,
      createDataChannel: () => {
        throw new Error("the test opens no channel");
      },
      close: () => undefined,
      onGone: (listener: (why: string) => void) => {
        goneListener = listener;
      },
    };
    const transport = new RtcTransport(
      [first],
      {
        open: (aborting) => {
          triedAtMs.push(Date.now());
          return signal(offer, aborting).then(() => first);
        },
        size: 4,
        onOnline: () => () => undefined,
      },
      () => Date.now(),
    );
    goneListener?.("it stayed disconnected");
    await vi.advanceTimersByTimeAsync(60_000);
    const waits = triedAtMs.slice(1).map((atMs, index) => atMs - (triedAtMs[index] ?? 0));
    expect(waits.length).toBeGreaterThan(5);
    expect(new Set(waits)).toEqual(new Set([unreachedRetryMs]));
    transport.close();
  });
});
