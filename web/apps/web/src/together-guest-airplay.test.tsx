// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { readFileSync } from "node:fs";
import path from "node:path";

import { apiPrefix } from "@orpheum/sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { airPlayHere } from "./airplay.ts";
import { buttonWithText, chooseIn, click, mount, settleUntil, type Mounted } from "./test-dom.tsx";
import { directDecision, unmeasuredSound } from "./test-fixtures.ts";
import { testServices, withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";
import { TogetherViewer } from "./together-viewer.tsx";

// A Watch Together guest by link, signed in to nothing, AirPlays the group's film from their page's
// Cast (D519 §1), in Safari, against a server that refuses the guest every route its membership
// does not open.

/** Each route a guest's membership may call (x-orpheum-capability member, none or media). */
function membersRoutes(): readonly RegExp[] {
  const document: unknown = JSON.parse(
    readFileSync(path.join(import.meta.dirname, "../../../../api/openapi.json"), "utf8"),
  );
  const routes: RegExp[] = [];
  for (const [template, operations] of Object.entries(
    Reflect.get(document as object, "paths") as object,
  )) {
    for (const [method, operation] of Object.entries(operations as object)) {
      const capability: unknown = Reflect.get(operation as object, "x-orpheum-capability");
      if (capability === "member" || capability === "none" || capability === "media") {
        const pattern = `${method.toUpperCase()} ${template.replaceAll(/\{[^/}]+\}/gu, "[^/]+")}`;
        routes.push(new RegExp(`^${pattern}$`, "u"));
      }
    }
  }
  return routes;
}

/** A request the page made: its route, its authorization, whether it had a key, and its body. */
interface Asked {
  readonly route: string;
  readonly authorization: string | null;
  readonly hasKey: boolean;
  readonly body: unknown;
}

const masterPath = "/api/v1/bytes/media/g/guests-grant/master.m3u8";

/** The group's film on its ladder, its playback's media token naming the film. */
function onItsLadder(fileId: number): ReturnType<typeof directDecision> {
  return directDecision({
    mediaToken: `film-${String(fileId)}`,
    onLadder: {
      url: `/api/v1/bytes/media/chunks/film-${String(fileId)}/master.m3u8`,
      startAtBps: 1_896_000,
      qualityKbps: 1200,
      height: 480,
      isBelowTheLowest: false,
      soundCarriedGainDb: 0,
      isReady: true,
      filmPictureKbps: 4000,
      linkSharePercent: 70,
      steadiness: 0,
    },
  });
}

/** The group's film on its ladder with two audio tracks, English and French, the first its start. */
function withTwoSoundsOnItsLadder(fileId: number): ReturnType<typeof directDecision> {
  const ladder = onItsLadder(fileId);
  const made = { isBoosted: false, codec: "aac", channelCount: 2, kbps: 96, isOriginal: false };
  return {
    ...ladder,
    audioTracks: [
      { streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" },
      { streamId: 81, audioTrackIndex: 1, language: "fr", kind: "plain" },
    ],
    startingAudioStreamId: 80,
    tracks: {
      ...ladder.tracks,
      sounds: [
        { audioTrackIndex: 0, ...made, ...unmeasuredSound, stream: "sound0-96k" },
        { audioTrackIndex: 1, ...made, ...unmeasuredSound, stream: "sound1-96k" },
      ],
      fileSounds: [
        { audioTrackIndex: 0, codec: "aac", channelCount: 2 },
        { audioTrackIndex: 1, codec: "aac", channelCount: 2 },
      ],
    },
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
  // A link that gives no controls: the guest's player shows none, and AirPlay is offered still.
  rights: { canPlayPause: false, canSeek: false, canNext: false },
  group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video" }] },
    { id: 2, name: "Gran", devices: [{ id: 8, role: "video" }] },
  ],
};

describe("a Watch Together guest by link's AirPlay (D519 §1)", () => {
  let mounted: Mounted | undefined;
  let server: FakeTogetherServer;
  let asked: Asked[];
  let refused: string[];
  let groupsFilm: number;
  /** Whether the group's film has two audio tracks (withTwoSoundsOnItsLadder). */
  let hasTwoSounds: boolean;
  /** Holds DELETE /together/airplay-grants unanswered while set; its release answers it. */
  let isRevokeHeld: boolean;
  let releaseRevoke: (() => void) | undefined;
  const pickers: HTMLMediaElement[] = [];

  beforeEach(() => {
    window.localStorage.clear();
    window.sessionStorage.clear();
    asked = [];
    refused = [];
    groupsFilm = 5;
    hasTwoSounds = false;
    isRevokeHeld = false;
    releaseRevoke = undefined;
    // Safari's AirPlay, as the page finds it.
    vi.stubGlobal("WebKitPlaybackTargetAvailabilityEvent", Event);
    Object.defineProperty(HTMLMediaElement.prototype, "webkitShowPlaybackTargetPicker", {
      value(this: HTMLMediaElement) {
        pickers.push(this);
      },
      configurable: true,
    });
  });

  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    pickers.length = 0;
    vi.unstubAllGlobals();
    Reflect.deleteProperty(HTMLMediaElement.prototype, "webkitShowPlaybackTargetPicker");
  });

  /** The core, answering the guest's page as it does, refusing what a guest may not call. */
  function guestsServer(): (request: Request) => Promise<Response> {
    const routes = membersRoutes();
    return async (request) => {
      const text = await request.text();
      const route = `${request.method} ${new URL(request.url).pathname.slice(apiPrefix.length)}`;
      asked.push({
        route,
        authorization: request.headers.get("Authorization"),
        hasKey: request.headers.has("Idempotency-Key"),
        body: text === "" ? undefined : JSON.parse(text),
      });
      if (!routes.some((each) => each.test(route))) {
        refused.push(route);
        return answer(403, { error: { code: "forbidden", message: "not a guest's" } });
      }
      if (route === "POST /together/playback") {
        return answer(200, (hasTwoSounds ? withTwoSoundsOnItsLadder : onItsLadder)(groupsFilm));
      }
      if (route === "POST /together/airplay-grants") {
        return answer(201, { masterPath, expiresAt: "2026-11-18T18:00:00Z" });
      }
      if (route === "DELETE /together/airplay-grants") {
        if (isRevokeHeld) {
          await new Promise<void>((resolve) => {
            releaseRevoke = resolve;
          });
        }
        return answer(200, { revoked: 1 });
      }
      return answer(204, undefined);
    };
  }

  /** The guest's page, in their session, the group playing film 5. */
  async function joinedAsTheFilmPlays(): Promise<HTMLElement> {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    server = fakeTogetherServer();
    const services = {
      ...testServices(undefined, "/together", guestsServer()),
      openTogetherSocket: server.openSocket,
    };
    mounted = await mount(withServices(<TogetherViewer />, services));
    const { container } = mounted;
    await click(buttonWithText(container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await playsTheGroupsFilm(5);
    return container;
  }

  /** The host puts on film fileId, and the guest's player offers its AirPlay. */
  async function playsTheGroupsFilm(fileId: number): Promise<void> {
    groupsFilm = fileId;
    await server.say({
      type: "group",
      seq: fileId,
      group: { state: "playing", positionMs: 60_000, rate: 1, fileId },
    });
    await settleUntil(() => airPlayHere()?.mediaToken === `film-${String(fileId)}`);
  }

  /** The page's button with text, wherever it shows, the dialog's included. */
  function button(text: string): HTMLElement | undefined {
    return buttonWithText(document.body, text);
  }

  /** Cast, then AirPlay once its grant is had. */
  async function airPlays(): Promise<void> {
    await click(button("Cast"));
    await settleUntil(() => button("AirPlay")?.hasAttribute("disabled") === false);
    await click(button("AirPlay"));
  }

  /** What the page asked of AirPlay's grants. */
  function grantsAsked(): Asked[] {
    return asked.filter((each) => each.route.endsWith(" /together/airplay-grants"));
  }

  it("asks its grant by the guest's membership with the playback's media token under a key, hands AirPlay the grant's master path, and asks for no route a guest is refused", async () => {
    await joinedAsTheFilmPlays();
    await airPlays();
    expect(grantsAsked()).toEqual([
      {
        route: "POST /together/airplay-grants",
        authorization: "Bearer viewer-credential",
        hasKey: true,
        body: { mediaToken: "film-5", audioTrackIndex: 0 },
      },
    ]);
    const [video] = pickers;
    expect(pickers).toHaveLength(1);
    expect(video?.getAttribute("src")).toBe(new URL(masterPath, globalThis.location.origin).href);
    expect(airPlayHere()?.isPlaying()).toBe(true);
    expect(button("AirPlay")).toBeUndefined(); // the dialog closed as the film went to AirPlay
    expect(refused).toEqual([]);
  });

  it("asks its grant with the audio track the guest chose on their page, which AirPlay plays (D521 §2)", async () => {
    hasTwoSounds = true;
    const container = await joinedAsTheFilmPlays();
    await chooseIn(container, "Sound", "French");
    await airPlays();
    expect(grantsAsked().map((each) => each.body)).toEqual([
      { mediaToken: "film-5", audioTrackIndex: 1 },
    ]);
    expect(refused).toEqual([]);
  });

  it("revokes the grant by the membership at the page's Stop AirPlay, the film handed back", async () => {
    await joinedAsTheFilmPlays();
    await airPlays();
    await click(button("Cast"));
    await click(button("Stop AirPlay"));
    await settleUntil(() => grantsAsked().length >= 2);
    expect(grantsAsked()[1]).toEqual({
      route: "DELETE /together/airplay-grants",
      authorization: "Bearer viewer-credential",
      hasKey: false,
      body: undefined,
    });
    expect(airPlayHere()?.isPlaying()).toBe(false);
    expect(refused).toEqual([]);
  });

  it("revokes the grant as the guest leaves", async () => {
    await joinedAsTheFilmPlays();
    await airPlays();
    await click(button("Leave"));
    await settleUntil(() => grantsAsked().length >= 2);
    expect(grantsAsked().map((each) => `${each.route} ${String(each.authorization)}`)).toEqual([
      "POST /together/airplay-grants Bearer viewer-credential",
      "DELETE /together/airplay-grants Bearer viewer-credential",
    ]);
  });

  it("ends AirPlay of the last film as the host puts on another, and asks the new film's grant once the last one's revocation is answered", async () => {
    await joinedAsTheFilmPlays();
    await airPlays();
    // The dialog open on Stop AirPlay as the film changes: it offers the new film's AirPlay.
    await click(button("Cast"));
    expect(button("Stop AirPlay")).toBeDefined();
    isRevokeHeld = true;
    await playsTheGroupsFilm(6);
    await settleUntil(() => releaseRevoke !== undefined);
    expect(grantsAsked().map((each) => each.route)).toEqual([
      "POST /together/airplay-grants",
      "DELETE /together/airplay-grants",
    ]);
    expect(button("AirPlay")?.hasAttribute("disabled")).toBe(true);
    releaseRevoke?.();
    await settleUntil(() => button("AirPlay")?.hasAttribute("disabled") === false);
    expect(grantsAsked()[2]).toEqual({
      route: "POST /together/airplay-grants",
      authorization: "Bearer viewer-credential",
      hasKey: true,
      body: { mediaToken: "film-6", audioTrackIndex: 0 },
    });
    await click(button("AirPlay"));
    expect(airPlayHere()?.isPlaying()).toBe(true);
    expect(refused).toEqual([]);
  });
});
