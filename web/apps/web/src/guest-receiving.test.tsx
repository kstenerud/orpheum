// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { readFileSync } from "node:fs";
import path from "node:path";

import { apiPrefix } from "@orpheum/sdk";
import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { App } from "./app.tsx";
import { tabStorage } from "./browser-storage.ts";
import { castReceivingKey } from "./cast-receiving-mark.ts";
import { mount, pressToStartTheCast, settleUntil, type Mounted } from "./test-dom.tsx";
import type { SubtitleTrack } from "./subtitles.tsx";
import { directDecision } from "./test-fixtures.ts";
import { testServices, withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";

// A guest receiver's page, signed in for one cast alone (D469 §4), against a server that refuses it
// every route the core refuses a guest (D465 §2): the page asks for none of them.

/** The API document, as text: who may call each operation (x-orpheum-capability). */
const apiDocument = readFileSync(
  path.join(import.meta.dirname, "../../../../api/openapi.json"),
  "utf8",
);

/**
 * The capabilities a guest receiver's session holds: a route for a guest or anyone signed in, one
 * that checks a secret in the request alone, and a byte path, which its media cookie opens.
 */
const guestCapabilities = new Set(["signedInOrGuest", "none", "media"]);

/**
 * The member's routes that take a guest receiver too, for the playbacks of its cast's film
 * (reporterOf, internal/core/api/report.go): its player's wants, reports and leaving.
 */
const guestsPlaybackRoutes = [
  "DELETE /playback/{token}",
  "POST /playback/{token}/report",
  "PUT /playback/{token}/wants",
];

/** Each route a guest receiver may call, as its method and a pattern its concrete path matches. */
function guestsRoutes(): readonly RegExp[] {
  const parsed: unknown = JSON.parse(apiDocument);
  const paths = new Map<string, unknown>(
    Object.entries(Reflect.get(parsed as object, "paths") as object),
  );
  const routes: string[] = [...guestsPlaybackRoutes];
  for (const [template, operations] of paths) {
    for (const [method, operation] of Object.entries(operations as object)) {
      const capability: unknown = Reflect.get(operation as object, "x-orpheum-capability");
      if (typeof capability === "string" && guestCapabilities.has(capability)) {
        routes.push(`${method.toUpperCase()} ${template}`);
      }
    }
  }
  return routes.map((route) => new RegExp(`^${route.replaceAll(/\{[^/}]+\}/gu, "[^/]+")}$`, "u"));
}

/** A cast handed to the guest's page: cast 5, of film 7, from the Pixel. */
const handed = {
  castId: 5,
  membershipCredential: "cast",
  fileId: 7,
  positionMs: 0,
  fromDeviceId: 2,
  fromDeviceName: "Pixel",
};

/** What the guest's page may ask, and the core answers it. */
const guestsAnswers: Readonly<Record<string, readonly [number, unknown]>> = {
  "GET /me": [
    200,
    { id: 3, name: "Pixel's guest", role: "user", isAdmin: false, isGuestReceiver: true },
  ],
  "POST /me/casts/5/accept": [200, { castId: 5 }],
  "POST /playback": [
    200,
    directDecision({
      mediaToken: "t",
      audioTracks: [
        { streamId: 1, audioTrackIndex: 0, language: "en", kind: "plain" },
        { streamId: 2, audioTrackIndex: 1, language: "fr", kind: "plain" },
      ],
      startingAudioStreamId: 1,
      subtitles: [subtitle(5, "en"), subtitle(7, "fr")],
    }),
  ],
};

/** One of the film's subtitle tracks, served as the guest's media cookie opens it. */
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

/**
 * A server that answers a guest's page as the core does: every route a guest may call (guestsRoutes)
 * from guestsAnswers, and every other refused as the core refuses a guest receiver, each kept in
 * refused.
 */
function guestsServer(asked: string[], refused: string[]): (request: Request) => Promise<Response> {
  const routes = guestsRoutes();
  return async (request) => {
    await request.text();
    const apiPath = new URL(request.url).pathname.slice(apiPrefix.length);
    const methodAndPath = `${request.method} ${apiPath}`;
    asked.push(methodAndPath);
    if (!routes.some((route) => route.test(methodAndPath))) {
      refused.push(methodAndPath);
      return answer(403, { error: { code: "forbidden", message: "signed in for one cast alone" } });
    }
    const [status, body] = guestsAnswers[methodAndPath] ?? [204, undefined];
    return answer(status, body);
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

describe("a guest receiver's page (D469 §4)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    tabStorage().removeItem(castReceivingKey);
  });

  it("asks for no route the core refuses a guest, through its cast's film played and its remote's sound and subtitle choices", async () => {
    const asked: string[] = [];
    const refused: string[] = [];
    const sockets = liveAndSession();
    const services = {
      ...testServices("token", "/", guestsServer(asked, refused)),
      openTogetherSocket: sockets.openSocket,
    };
    mounted = await mount(withServices(<App />, services));
    const { container } = mounted;
    await settleUntil(() => container.textContent.includes("Waiting for the cast from your phone"));
    await sockets.live.accept();
    await sockets.live.say({ type: "receive", receive: handed });
    await pressToStartTheCast();
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 7 });
    await settleUntil(() => container.querySelector("video") !== null);
    await sockets.session.accept();
    await sockets.session.say({
      type: "welcome",
      seq: 1,
      sessionId: 5,
      memberId: 2,
      deviceId: 6,
      isHost: true,
      rights: { canPlayPause: true, canSeek: true },
      group: { state: "playing", positionMs: 0, rate: 1, fileId: 7 },
      roster: [],
    });
    // The film has played, when Up next would be asked for, and the remote chooses the levelling.
    const video = container.querySelector("video");
    if (video !== null) {
      Object.defineProperty(video, "paused", { configurable: true, value: false });
    }
    await act(async () => {
      video?.dispatchEvent(new Event("playing"));
      await new Promise((resolve) => setTimeout(resolve, 20));
    });
    await sockets.session.say({
      type: "sound",
      fromDeviceId: 2,
      sound: { kind: "levelling", isOn: false },
    });
    // Its remote chooses another audio track and subtitles, which the guest keeps for no one.
    await sockets.session.say({
      type: "sound",
      fromDeviceId: 2,
      sound: { kind: "audioTrack", streamId: 2 },
    });
    await sockets.session.say({
      type: "picture",
      fromDeviceId: 2,
      picture: { kind: "subtitles", key: "s7" },
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 50));
    });
    expect(asked).toContain("POST /playback");
    // The audio track chosen plays: the playback is asked for again, with it.
    expect(asked.filter((each) => each === "POST /playback").length).toBeGreaterThan(1);
    expect(refused).toEqual([]);
  });
});
