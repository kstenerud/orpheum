// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { createApi } from "./client.ts";
import type { PlayableFormats } from "./playable-formats.ts";
import { requestPlayback, requestTogetherPlayback } from "./playback-request.ts";

/** A core answering every ask with decision, each ask kept as method, path, sign-in and body. */
function coreDeciding(decision: unknown): {
  readonly asked: string[];
  readonly api: ReturnType<typeof createApi>;
} {
  const asked: string[] = [];
  const api = createApi({
    origin: "https://core.example",
    getToken: () => "the-session",
    fetch: async (request) => {
      const signedIn = request.headers.get("Authorization") ?? "signed out";
      const path = new URL(request.url).pathname;
      asked.push(`${request.method} ${path} ${signedIn} ${await request.text()}`);
      return new Response(JSON.stringify(decision), {
        headers: { "Content-Type": "application/json" },
      });
    },
  });
  return { asked, api };
}

const formats: PlayableFormats = {
  containers: [],
  videoCodecs: ["h264-sdr"],
  audioCodecs: ["aac-stereo"],
  subtitleFormats: ["webvtt"],
};

describe("a player asking how a film plays (D555 §3)", () => {
  it("asks POST /playback with what the player says of itself, by its session, and gives the decision", async () => {
    const core = coreDeciding({ url: "/media/1" });
    const asked = { fileId: 7, playableFormats: formats, cause: "playPage" } as const;
    await expect(requestPlayback(core.api, asked)).resolves.toEqual({ url: "/media/1" });
    expect(core.asked).toEqual([
      `POST /api/v1/playback Bearer the-session ${JSON.stringify(asked)}`,
    ]);
  });

  it("asks a session's film by the viewer's membership in place of the session (D246 §3)", async () => {
    const core = coreDeciding({ url: "/media/2" });
    await expect(requestTogetherPlayback(core.api, "the-membership", formats)).resolves.toEqual({
      url: "/media/2",
    });
    expect(core.asked).toEqual([
      `POST /api/v1/together/playback Bearer the-membership ${JSON.stringify({ playableFormats: formats })}`,
    ]);
  });
});
