// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { whenReady } from "./ready.ts";

function answer(status: number, headers: Record<string, string> = {}, body = "{}"): Response {
  return new Response(body, {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

describe("waiting for a prepared rendition", () => {
  it("asks again after each Retry-After until the playlist exists", async () => {
    const answers = [answer(425, { "Retry-After": "2" }), answer(425), answer(200)];
    const waits: number[] = [];
    const credentials: (RequestCredentials | undefined)[] = [];
    await whenReady("/api/v1/bytes/media/hls/t/master.m3u8", {
      fetch: (_url, init) => {
        credentials.push(init.credentials);
        return Promise.resolve(answers.shift() ?? answer(500));
      },
      sleep: (milliseconds) => {
        waits.push(milliseconds);
        return Promise.resolve();
      },
    });
    expect(waits).toEqual([2000, 1000]);
    expect(credentials).toEqual(["same-origin", "same-origin", "same-origin"]);
  });

  it("stops with the server's message on anything but too early", async () => {
    const failed = answer(
      502,
      {},
      JSON.stringify({ error: { code: "prepare_failed", message: "could not be prepared" } }),
    );
    await expect(
      whenReady("/x", { fetch: () => Promise.resolve(failed), sleep: () => Promise.resolve() }),
    ).rejects.toThrow("could not be prepared");
  });

  it("gives up after its attempts", async () => {
    await expect(
      whenReady("/x", {
        fetch: () => Promise.resolve(answer(425)),
        sleep: () => Promise.resolve(),
        attempts: 3,
      }),
    ).rejects.toThrow("still being prepared");
  });
});
