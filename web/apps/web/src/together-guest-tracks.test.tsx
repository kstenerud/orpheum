// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { readFileSync } from "node:fs";
import path from "node:path";

import { apiPrefix } from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";

import { buttonWithText, chooseIn, click, mount, type Mounted } from "./test-dom.tsx";
import type { SubtitleTrack } from "./subtitles.tsx";
import { directDecision, unmeasuredSound } from "./test-fixtures.ts";
import { testServices, withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import { TogetherViewer } from "./together-viewer.tsx";

// A Watch Together guest's sound and subtitles are their own (D471): chosen on their page, they play
// and show there alone, against a server that refuses the guest every route its membership does not
// open, and sends the group nothing.

/** The API document, as text: who may call each operation (x-orpheum-capability). */
const apiDocument = readFileSync(
  path.join(import.meta.dirname, "../../../../api/openapi.json"),
  "utf8",
);

/**
 * The capabilities a Watch Together guest's membership holds: a member's route, one that checks a
 * secret in the request alone, and a byte path, which the viewer's cookie opens.
 */
const guestCapabilities = new Set(["member", "none", "media"]);

/** Each route a guest may call, its capability, as its method and a pattern its path matches. */
function guestsRoutes(): readonly { readonly capability: string; readonly pattern: RegExp }[] {
  const parsed: unknown = JSON.parse(apiDocument);
  const paths = Object.entries(Reflect.get(parsed as object, "paths") as object);
  return paths.flatMap(([template, operations]) =>
    Object.entries(operations as object).flatMap(([method, operation]) => {
      const capability: unknown = Reflect.get(operation as object, "x-orpheum-capability");
      if (typeof capability !== "string" || !guestCapabilities.has(capability)) {
        return [];
      }
      const route = `${method.toUpperCase()} ${template}`;
      const pattern = new RegExp(`^${route.replaceAll(/\{[^/}]+\}/gu, "[^/]+")}$`, "u");
      return [{ capability, pattern }];
    }),
  );
}

/** A subtitle track of the playback t. */
function subtitle(streamId: number, language: string): SubtitleTrack {
  return {
    streamId,
    language,
    isDefault: false,
    isForced: false,
    isHearingImpaired: false,
    assUrl: `/api/v1/bytes/media/sub/t/${String(streamId)}.ass`,
    vttUrl: `/api/v1/bytes/media/sub/t/${String(streamId)}.vtt`,
  };
}

/** The host's film, played as it is, with two audio tracks and two subtitle tracks. */
function twoOfEach(): ReturnType<typeof directDecision> {
  const plain = directDecision();
  const own = { codec: "aac", channelCount: 2, kbps: 0, isOriginal: true, isBoosted: false };
  return directDecision({
    audioTracks: [
      { streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" },
      { streamId: 81, audioTrackIndex: 1, language: "fr", kind: "plain" },
    ],
    startingAudioStreamId: 80,
    subtitles: [subtitle(5, "en"), subtitle(7, "fr")],
    tracks: {
      ...plain.tracks,
      sounds: [
        { audioTrackIndex: 0, ...own, ...unmeasuredSound, stream: "sound0-own" },
        { audioTrackIndex: 1, ...own, ...unmeasuredSound, stream: "sound1-own" },
      ],
    },
  });
}

/** What the server was asked: each request, those it refused, and the bodies of the wants said. */
interface Asked {
  readonly asked: string[];
  readonly refused: string[];
  readonly wants: unknown[];
}

/**
 * A server that answers a guest's page as the core does: every route its membership opens, a
 * member's only with the membership's credential, and every other refused, each kept in refused.
 */
function guestsServer(kept: Asked): (request: Request) => Promise<Response> {
  const routes = guestsRoutes();
  return async (request) => {
    const text = await request.text();
    const methodAndPath = `${request.method} ${new URL(request.url).pathname.slice(apiPrefix.length)}`;
    kept.asked.push(methodAndPath);
    const route = routes.find((each) => each.pattern.test(methodAndPath));
    const isMembers = request.headers.get("Authorization") === "Bearer viewer-credential";
    if (route === undefined || (route.capability === "member" && !isMembers)) {
      kept.refused.push(methodAndPath);
      return answer(403, { error: { code: "forbidden", message: "not a guest's" } });
    }
    if (methodAndPath === "PUT /playback/t/wants") {
      kept.wants.push(JSON.parse(text));
    }
    if (methodAndPath === "POST /together/playback") {
      return answer(200, twoOfEach());
    }
    // A track's text is not answered: drawing it is JASSUB's, which needs a worker this page lacks.
    if (methodAndPath.startsWith("GET /bytes/media/sub/")) {
      return answer(404, { error: { code: "not_found", message: "not here" } });
    }
    return answer(204, undefined);
  };
}

/** A JSON answer, or an empty one for 204. */
function answer(status: number, body: unknown): Response {
  return status === 204
    ? new Response(null, { status })
    : new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      });
}

const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 1,
  memberId: 2,
  deviceId: 8,
  rights: { canPlayPause: true, canSeek: true, canNext: false },
  group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video" }] },
    { id: 2, name: "Gran", devices: [{ id: 8, role: "video" }] },
  ],
};

describe("a Watch Together guest's sound and subtitles (D471)", () => {
  let mounted: Mounted | undefined;
  const tracks = { length: 2, 0: { enabled: true }, 1: { enabled: false } };
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    Reflect.deleteProperty(HTMLMediaElement.prototype, "audioTracks");
    window.localStorage.clear();
  });

  it("are chosen on the guest's page, played and shown there alone, asking no route the core refuses a guest and sending the group nothing", async () => {
    // A browser whose video lists a file's own audio tracks, as Safari's does.
    Object.defineProperty(HTMLMediaElement.prototype, "audioTracks", {
      get: () => tracks,
      configurable: true,
    });
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const kept: Asked = { asked: [], refused: [], wants: [] };
    const server = fakeTogetherServer();
    const services = {
      ...testServices(undefined, "/together", guestsServer(kept)),
      openTogetherSocket: server.openSocket,
    };
    mounted = await mount(withServices(<TogetherViewer />, services));
    const { container } = mounted;
    await click(buttonWithText(container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "playing", positionMs: 60_000, rate: 1, fileId: 5 },
    });
    const heardBefore = server.heard.length;
    await chooseIn(container, "Subtitles", "French");
    await chooseIn(container, "Sound", "French");
    expect([tracks[0].enabled, tracks[1].enabled]).toEqual([false, true]);
    expect(kept.wants).toEqual([{ soundStream: "sound1-own", positionMs: 0, isPaused: true }]);
    expect(kept.asked).toContain("GET /bytes/media/sub/t/7.ass");
    expect(kept.asked.filter((each) => each.includes("/preferences"))).toEqual([]);
    expect(kept.refused).toEqual([]);
    expect(server.heard.slice(heardBefore)).toEqual([]);
  });

  it("offer no sound where another track cannot switch in place, the film played as it is", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const kept: Asked = { asked: [], refused: [], wants: [] };
    const server = fakeTogetherServer();
    const services = {
      ...testServices(undefined, "/together", guestsServer(kept)),
      openTogetherSocket: server.openSocket,
    };
    mounted = await mount(withServices(<TogetherViewer />, services));
    const { container } = mounted;
    await click(buttonWithText(container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "playing", positionMs: 60_000, rate: 1, fileId: 5 },
    });
    const fields = [...container.querySelectorAll(".route-chooser__field")].map(
      (each) => each.textContent,
    );
    expect(fields).toEqual(["Subtitles"]);
    expect(kept.refused).toEqual([]);
  });
});
