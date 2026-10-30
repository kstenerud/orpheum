// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  isWorkerBypassed,
  workerProbeAnswer,
  workerProbePath,
  workerProbeResponse,
} from "./worker-control.ts";

afterEach(() => {
  vi.useRealTimers();
});

// DevTools' "Bypass for network" sends a page's loads past its worker, and at a client origin its
// posters and films 404 saying nothing (agent-failures A51): the page asks for an address only the
// worker answers.
describe("isWorkerBypassed", () => {
  it("is not bypassed when the worker answers the probe", async () => {
    const asked: string[] = [];
    const isBypassed = await isWorkerBypassed((path) => {
      asked.push(path);
      return Promise.resolve(workerProbeResponse());
    });
    expect(isBypassed).toBe(false);
    expect(asked).toEqual([workerProbePath]);
  });

  it("is bypassed when the network answers the probe, with a 404 or a page of its own", async () => {
    expect(
      await isWorkerBypassed(() =>
        Promise.resolve(new Response("404 page not found", { status: 404 })),
      ),
    ).toBe(true);
    expect(
      await isWorkerBypassed(() =>
        Promise.resolve(new Response("<!doctype html>", { status: 200 })),
      ),
    ).toBe(true);
  });

  it("is bypassed when the probe fails, or nothing answers it in time", async () => {
    expect(await isWorkerBypassed(() => Promise.reject(new TypeError("Failed to fetch")))).toBe(
      true,
    );
    vi.useFakeTimers();
    const silent = isWorkerBypassed(() => new Promise<Response>(() => undefined), 5000);
    await vi.advanceTimersByTimeAsync(5000);
    expect(await silent).toBe(true);
  });

  it("is not fooled by an answer that is not the worker's word", async () => {
    expect(
      await isWorkerBypassed(() => Promise.resolve(new Response(`${workerProbeAnswer}.`))),
    ).toBe(true);
  });
});
