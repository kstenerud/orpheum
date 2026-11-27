// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { createApi } from "./client.ts";
import { ApiProblem } from "./problem.ts";
import { rungOf } from "./rtc-transport.ts";
import { readScrubPictures } from "./trickplay.ts";

/** An API whose every answer is status with body, the paths it was asked kept in asked. */
function answering(status: number, body: unknown, asked: string[]): ReturnType<typeof createApi> {
  return createApi({
    origin: "http://orpheum.test",
    getToken: () => "token",
    fetch: (request: Request) => {
      asked.push(new URL(request.url).pathname);
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status,
          headers: { "Content-Type": "application/json" },
        }),
      );
    },
  });
}

describe("readScrubPictures", () => {
  it("reads a file's scrub-bar pictures, its sheets in order", async () => {
    const asked: string[] = [];
    const body = {
      intervalMs: 10_000,
      pictureWidth: 320,
      pictureHeight: 180,
      columns: 10,
      rows: 10,
      pictureCount: 120,
      sheets: [{ index: 1, url: "/api/v1/bytes/trickplay/7/b.webp" }],
    };
    await expect(readScrubPictures(answering(200, body, asked), 7)).resolves.toEqual(body);
    expect(asked).toEqual(["/api/v1/files/7/trickplay"]);
  });

  it("is undefined while none is made, the core's 404, and throws any other refusal", async () => {
    const problem = { status: 404, code: "notFound", message: "none yet" };
    await expect(readScrubPictures(answering(404, problem, []), 7)).resolves.toBeUndefined();
    const refused = { status: 401, code: "authRequired", message: "sign in" };
    await expect(readScrubPictures(answering(401, refused, []), 7)).rejects.toBeInstanceOf(
      ApiProblem,
    );
  });
});

describe("a scrub-bar sheet over WebRTC", () => {
  it("waits on the cosmetic rung, below the film's own bytes (D479)", () => {
    expect(rungOf("https://app.example/api/v1/bytes/trickplay/2/ab.webp")).toBe("cosmetic");
    expect(rungOf("https://app.example/api/v1/bytes/media/chunks/t/master.m3u8")).toBe("waited-on");
  });
});
