// SPDX-License-Identifier: MPL-2.0

// What the tests of the cast's remote share (cast-remote.test.tsx, cast-remote-ends.test.tsx): the
// cast, what the remote reads of it, and the remote drawn on film 4's Play page.

import type { ReactNode } from "react";
import type { CastRemote } from "./cast-remote.tsx";
import { CastSignOverTheFilmContext } from "./cast-sign-over-the-film.tsx";
import { LiveProvider } from "./live.tsx";
import { overlayUpClass, PlayUpwardContext } from "./play-up.tsx";
import { click, mount, servedBy, type Answers, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";
import type { Upward } from "./upward.ts";

/** Opens the remote's menu whose opener's name starts with name, as over the film. */
export async function openedMenu(page: Element, name: string): Promise<void> {
  const opener = [...page.querySelectorAll("[aria-haspopup]")].find(
    (button) => button.getAttribute("aria-label")?.startsWith(name) === true,
  );
  await click(opener);
}

/** The cast's session saying its receiver, leading it, plays as playback says. */
export async function receiverSays(
  server: ReturnType<typeof liveAndSession>["session"],
  playback: Readonly<Record<string, unknown>>,
): Promise<void> {
  const receiver = { id: 7, role: "video", isLeader: true, status: "watching", playback };
  await server.say({
    type: "roster",
    roster: [{ id: 2, name: "owner", isHost: true, devices: [{ id: 5, role: "panel" }, receiver] }],
  });
}

/** The cast this device sends, as GET /me/casts lists it. */
export const cast = {
  castId: 9,
  isSender: true,
  fileId: 4,
  itemId: 2,
  itemName: "Metropolis",
  durationMs: 600_000,
  senderDeviceId: 1,
  senderName: "Pixel",
  receiverDeviceId: 2,
  receiverName: "Fire TV",
  membershipCredential: "remote",
};

/** Item 2, the cast's film, as GET /items/2 has it: the remote's way up leads to its page. */
export const metropolis = {
  id: 2,
  kind: "film",
  name: "Metropolis",
  identificationState: "identified",
  art: {},
  versions: [] as const,
  extras: [] as const,
};

/** Item 3, Cat, the episode file 5 plays, as GET /items/3 has it. */
export const catTitle = {
  id: 3,
  kind: "episode",
  name: "Cat",
  identificationState: "identified",
  art: {},
  versions: [] as const,
  extras: [] as const,
  episode: { seriesId: 1, seriesName: "Glass Tide", seasonNumber: 1, episodeNumber: 2 },
};

/** The cast moved on to Cat, file 5, as GET /me/casts lists it. */
export const castOnCat: typeof cast = {
  ...cast,
  fileId: 5,
  itemId: 3,
  itemName: "Cat",
  durationMs: 1_200_000,
};

/** What the remote reads for film 4 besides the cast: its title, for its way up, and its segments, none. */
export const filmReads: Answers = {
  "GET /items/2": [200, metropolis],
  "GET /files/4/segments": [200, { segments: [] }],
};

/** What the remote reads once the cast has moved on to Cat: the cast again, Cat's title and segments. */
export const catReads: Answers = {
  "GET /me/casts": [
    [200, { casts: [cast] }],
    [200, { casts: [castOnCat] }],
  ],
  "GET /items/3": [200, catTitle],
  "GET /files/5/segments": [200, { segments: [] }],
};

/** The name of the remote's way up, the player's own (OverlayUp): the page of what it plays. */
export function wayUp(page: Element): string | undefined {
  return (
    page.querySelector(`.cast-remote__bar .${overlayUpClass} .player-overlay__title`)
      ?.textContent ?? undefined
  );
}

/** The remote's sign under its way up, where its film plays. */
export function signOf(page: Element): string | undefined {
  return page.querySelector(".cast-remote__bar h2")?.textContent ?? undefined;
}

/** The remote's welcome, the receiver's film paused at a minute. */
export const welcome = {
  type: "welcome",
  seq: 3,
  sessionId: 9,
  memberId: 2,
  deviceId: 5,
  isHost: true,
  rights: { canPlayPause: true, canSeek: true },
  group: { state: "paused", positionMs: 61_000, rate: 1, fileId: 4 },
  roster: [] as const,
};

/** The cast's roster, its receiver's video leading it. */
export const receiverLeads = [
  { id: 2, name: "owner", isHost: true, devices: [{ id: 6, role: "video", isLeader: true }] },
] as const;

/** The cast this device sends, as the live connection says it, its receiver having said Use. */
export const sending = {
  castId: 9,
  receiverDeviceId: 2,
  receiverName: "Fire TV",
  fileId: 4,
  itemName: "Metropolis",
  durationMs: 600_000,
  isAccepted: true,
};

/** File 4's Up next: the second episode, Cat, file 5. */
export const catEpisode = { itemId: 3, name: "Cat", groupNumber: 1, numberInGroup: 2, fileId: 5 };
export const catNext: {
  readonly upNext: {
    readonly episode: typeof catEpisode & { readonly isPartway: boolean };
    readonly ordering: string;
  };
} = {
  upNext: { episode: { ...catEpisode, isPartway: false }, ordering: "aired" },
};

/** The receiver's file 4 paused at positionMs, as a seek leaves it. */
export const pausedAt = (positionMs: number): Readonly<Record<string, unknown>> => ({
  state: "paused",
  positionMs,
  rate: 1,
  fileId: 4,
  event: "seeked",
});

/** The Up next card's own button whose offer says words: the card holds its still and "Play now" too. */
export function upNextCard(container: HTMLElement, words: string): HTMLElement | undefined {
  return [...container.querySelectorAll<HTMLElement>("button.offer-card__take")].find(
    (each) => each.querySelector(".offer-card__offer")?.textContent === words,
  );
}

/** How a test draws the remote beyond its answers: what was sent, the Cast sign and the way up. */
export interface RemoteShown {
  readonly sent?: Readonly<Record<string, unknown>>;
  /** What the session's welcome says in place of `welcome`, as a session that holds no position yet. */
  readonly welcome?: Readonly<Record<string, unknown>>;
  readonly castSign?: () => ReactNode;
  readonly upward?: Upward;
}

/**
 * shown.remote (CastRemote, which each test imports itself: a lazily loaded screen,
 * home-imports.test.ts) on film 4's Play page, its live connection saying the cast this device sends, its
 * session joined; what it was told of playing the film here again.
 */
export async function castingRemote(
  answers: Parameters<typeof servedBy>[0],
  heard: Heard[] = [],
  shown: RemoteShown & {
    readonly remote: typeof CastRemote;
    readonly onPlayHere: Parameters<typeof CastRemote>[0]["onPlayHere"];
  },
): Promise<{
  readonly mounted: Mounted;
  readonly services: ReturnType<typeof servedBy>;
  readonly sockets: ReturnType<typeof liveAndSession>;
}> {
  const Remote = shown.remote;
  const services = servedBy(
    {
      "GET /me/casts": [200, { casts: [cast] }],
      "GET /files/4/up-next": [200, {}],
      ...filmReads,
      ...answers,
    },
    heard,
    "token",
  );
  const sockets = liveAndSession();
  const mounted = await mount(
    withServices(
      <CastSignOverTheFilmContext value={shown.castSign ?? (() => null)}>
        <PlayUpwardContext value={shown.upward}>
          <LiveProvider>
            <Remote castId={9} fileId={4} onPlayHere={shown.onPlayHere} />
          </LiveProvider>
        </PlayUpwardContext>
      </CastSignOverTheFilmContext>,
      { ...services, openTogetherSocket: sockets.openSocket },
    ),
  );
  await sockets.live.accept();
  await sockets.live.say({ type: "sending", sending: shown.sent ?? sending });
  await sockets.session.accept();
  await sockets.session.say(shown.welcome ?? welcome);
  return { mounted, services, sockets };
}
