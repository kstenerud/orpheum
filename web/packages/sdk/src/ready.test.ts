// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { ApiProblem } from "./problem.ts";
import { textWhenReady, whenReady } from "./ready.ts";

function fakeResponse(status: number, headers: Record<string, string> = {}, body = "{}"): Response {
  return new Response(body, {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

describe("waiting for a prepared rendition", () => {
  it("asks again after each Retry-After until the playlist exists", async () => {
    const answers = [
      fakeResponse(425, { "Retry-After": "2" }),
      fakeResponse(425),
      fakeResponse(200),
    ];
    const waits: number[] = [];
    const credentials: (RequestCredentials | undefined)[] = [];
    await whenReady("/api/v1/bytes/media/hls/t/master.m3u8", {
      fetch: (_url, init) => {
        credentials.push(init.credentials);
        return Promise.resolve(answers.shift() ?? fakeResponse(500));
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
    const failed = fakeResponse(
      502,
      {},
      JSON.stringify({ error: { code: "prepare_failed", message: "could not be prepared" } }),
    );
    await expect(
      whenReady("/x", { fetch: () => Promise.resolve(failed), sleep: () => Promise.resolve() }),
    ).rejects.toThrow("could not be prepared");
  });

  it("throws a refusal as the ApiProblem it answered, so the client says it by its code", async () => {
    const failed = fakeResponse(
      502,
      {},
      JSON.stringify({ error: { code: "prepare_failed", message: "could not be prepared" } }),
    );
    const refusal: unknown = await whenReady("/x", {
      fetch: () => Promise.resolve(failed),
      sleep: () => Promise.resolve(),
    }).catch((cause: unknown) => cause);
    expect(refusal).toBeInstanceOf(ApiProblem);
    expect(refusal instanceof ApiProblem ? refusal.code : "").toBe("prepare_failed");
    const shapeless: unknown = await whenReady("/x", {
      fetch: () => Promise.resolve(fakeResponse(500, {}, "not json")),
      sleep: () => Promise.resolve(),
    }).catch((cause: unknown) => cause);
    // The server's own 500 is in its shape: one that is not came from something in front of it.
    expect(shapeless instanceof ApiProblem ? shapeless.code : "").toBe("unreadable_answer");
  });

  it("gives up after its attempts", async () => {
    await expect(
      whenReady("/x", {
        fetch: () => Promise.resolve(fakeResponse(425)),
        sleep: () => Promise.resolve(),
        maxAttempts: 3,
      }),
    ).rejects.toThrow("still being prepared");
  });
});

describe("reading a subtitle being extracted", () => {
  it("waits through the 425s and returns the text", async () => {
    const answers = [
      fakeResponse(425),
      fakeResponse(200, { "Content-Type": "text/x-ssa" }, "[Script Info]"),
    ];
    const text = await textWhenReady("/api/v1/bytes/media/sub/t/2.ass", {
      fetch: () => Promise.resolve(answers.shift() ?? fakeResponse(500)),
      sleep: () => Promise.resolve(),
    });
    expect(text).toBe("[Script Info]");
  });
});
