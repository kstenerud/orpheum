// SPDX-License-Identifier: MPL-2.0

import { playChunked, type ChunkedPlayback } from "@orpheum/player";
import {
  browserPlayableFormats,
  createLiveConnection,
  createTogetherConnection,
  dataOrThrow,
  expectedGroupPositionMs,
  failureText,
  type Api,
  type CastToReceive,
  type GroupView,
  type PlayableFormats,
  type PlayerState,
  type TogetherConnection,
} from "@orpheum/sdk";

import type { Reached } from "./reach.ts";

/**
 * The receiver as a cast target (inventory B-10; the owner, 2026-11-07: "initial chromecast
 * support"): signed in as a device of its sender's person by D288's pairing, the code carried over
 * the lab's Cast channel rather than typed and the check codes shown on both screens for the person
 * to compare, it keeps a live connection, so it is listed in the Cast control as any device connected
 * now (D337); a cast handed to it is taken at once, the Chromecast having no remote to say Use with,
 * and played in the cast's session as a browser receiver plays it (D434), so the sender's remote
 * works it. A first cut, behind the lab (plans/navigation-and-widgets-answers-2026-10-27.md, the
 * Chromecast test steps): it follows the session's film, play, pause and position; sound, picture and
 * the cast's other panels wait on what the device shows.
 */

/** Where the receiver keeps the session its pairing gave it, so a later launch needs no pairing. */
const keptSessionKey = "orpheum.cast.session";

/** How often a pairing's state is asked while the person compares the codes, in milliseconds. */
export const pairingPollEveryMs = 2000;

/**
 * How many times a pairing's state is asked before the receiver gives it up: five minutes, longer
 * than the server keeps a code (D288 §1), so the server's end is what normally stops it.
 */
export const pairingPolls = 150;

/**
 * How far the receiver may be from the session's position before it seeks there, in milliseconds:
 * past a chunk's start-up, so a film starting or a seek just made is not sought again.
 */
export const seekPastMs = 3000;

/** The session a pairing gave this receiver: its token, and the locator its discovery finds it by. */
export interface KeptSession {
  readonly token: string;
  readonly locator: string;
}

/** Storage as the receiver uses it: the browser's localStorage, or a test's. */
export interface StringStorage {
  readonly getItem: (key: string) => string | null;
  readonly setItem: (key: string, value: string) => void;
  readonly removeItem: (key: string) => void;
}

/** The session this receiver keeps, or undefined for none or anything unreadable. */
export function keptSessionOf(storage: StringStorage): KeptSession | undefined {
  try {
    const kept: unknown = JSON.parse(storage.getItem(keptSessionKey) ?? "null");
    if (typeof kept !== "object" || kept === null) {
      return undefined;
    }
    const token: unknown = Reflect.get(kept, "token");
    const locator: unknown = Reflect.get(kept, "locator");
    return typeof token === "string" && typeof locator === "string" && token !== ""
      ? { token, locator }
      : undefined;
  } catch {
    return undefined;
  }
}

/** Keeps session, or forgets the one kept for undefined (the server refused it). */
export function keepSession(storage: StringStorage, session: KeptSession | undefined): void {
  if (session === undefined) {
    storage.removeItem(keptSessionKey);
  } else {
    storage.setItem(keptSessionKey, JSON.stringify(session));
  }
}

/** What the pairing tells as it goes: the code for the sender to enter, and the check code to show. */
export interface PairingSteps {
  readonly tellCode: (code: string) => void;
  readonly showCheckCode: (checkCode: string) => void;
  readonly waitMs: (ms: number) => Promise<void>;
}

/**
 * Pairs this receiver as a device (D288): asks the core for a code, tells it (to the sender, which
 * enters it under its person's session), shows the check code once the code is entered, and takes
 * the session once the person said the codes match. Fails where the pairing ends unconfirmed.
 */
export async function pairThisDevice(api: Api, steps: PairingSteps): Promise<KeptSession> {
  const pairing = dataOrThrow(await api.POST("/pairings", { body: { platform: "chromecast" } }));
  steps.tellCode(pairing.code);
  let shown = "";
  for (let poll = 0; poll < pairingPolls; poll += 1) {
    const state = dataOrThrow(
      await api.POST("/pairings/state", { body: { secret: pairing.secret } }),
    );
    if (state.checkCode !== undefined && state.checkCode !== shown) {
      shown = state.checkCode;
      steps.showCheckCode(shown);
    }
    if (state.state === "activated") {
      const session = dataOrThrow(
        await api.POST("/pairings/session", { body: { secret: pairing.secret } }),
      );
      return { token: session.token, locator: session.locator };
    }
    if (state.state === "ended") {
      throw new Error("the pairing ended unconfirmed");
    }
    await steps.waitMs(pairingPollEveryMs);
  }
  throw new Error("the pairing was not confirmed in time");
}

/** Where the receiver's player stands. */
export interface PlayerNow {
  /** The film it plays, 0 for none. */
  readonly fileId: number;
  readonly positionMs: number;
  readonly isPlaying: boolean;
}

/** One thing the receiver's player does to follow its session. */
export type FollowStep =
  | { readonly step: "load"; readonly fileId: number; readonly positionMs: number }
  | { readonly step: "stop" }
  | { readonly step: "seek"; readonly positionMs: number }
  | { readonly step: "play" }
  | { readonly step: "pause" };

/**
 * What the player does to follow the session's group, expectedMs being where the group stands now:
 * the group's film loaded at its position, or none; then played or paused as the group is, sought
 * where it is further than seekPastMs away.
 */
export function stepsToFollow(
  group: GroupView | undefined,
  expectedMs: number | undefined,
  player: PlayerNow,
): readonly FollowStep[] {
  if (group === undefined) {
    return [];
  }
  if (group.fileId === 0) {
    return player.fileId === 0 ? [] : [{ step: "stop" }];
  }
  const positionMs = Math.max(0, expectedMs ?? group.positionMs);
  const isPlaying = group.state === "Playing";
  if (group.fileId !== player.fileId) {
    return [
      { step: "load", fileId: group.fileId, positionMs },
      ...(isPlaying ? [{ step: "play" } as const] : []),
    ];
  }
  return [
    ...(Math.abs(player.positionMs - positionMs) > seekPastMs
      ? [{ step: "seek", positionMs } as const]
      : []),
    ...playOrPause(isPlaying, player.isPlaying),
  ];
}

/** Play or pause, where the player is not as the group is. */
function playOrPause(isGroupPlaying: boolean, isPlaying: boolean): readonly FollowStep[] {
  if (isGroupPlaying === isPlaying) {
    return [];
  }
  return [{ step: isGroupPlaying ? "play" : "pause" }];
}

/**
 * What the receiver asks to play: the device's own codecs, by no container of a file's own, so the
 * core puts the film on its ladder, which our player plays (the lab's sender asks the same).
 */
function receiverFormats(): PlayableFormats {
  return { ...browserPlayableFormats(), containers: [] };
}

/** The video's events after which the session is told where the player is. */
const reportedVideoEvents = ["playing", "pause", "seeked", "waiting", "canplay", "ended"] as const;

/** What a cast target needs: its API, session and path to the core, its video, and its log. */
export interface CastDeviceOptions {
  readonly api: Api;
  readonly token: string;
  readonly reached: Reached;
  readonly video: HTMLVideoElement;
  readonly note: (line: string) => void;
  /** Called when the server refuses the receiver's session: its device was removed. */
  readonly onRefused: () => void;
}

/** The receiver as a device of its person's: its live connection, and the cast it plays. */
export class CastDevice {
  private castId: number | undefined;
  private together: TogetherConnection | undefined;
  private playback: ChunkedPlayback | undefined;
  private playingFileId = 0;

  constructor(private readonly options: CastDeviceOptions) {
    for (const type of reportedVideoEvents) {
      options.video.addEventListener(type, () => {
        this.together?.reportState();
      });
    }
  }

  /** Connects as a device, listed in the Cast control while connected; returns the stopping. */
  start(): () => void {
    const { token, reached, note } = this.options;
    const live = createLiveConnection({
      sessionToken: token,
      client: "cast-receiver",
      openSocket: reached.openSocket,
      monotonicMs: () => performance.now(),
    });
    let wasConnected = false;
    const stop = live.subscribe(() => {
      const snapshot = live.snapshot();
      if (snapshot.isConnected !== wasConnected) {
        wasConnected = snapshot.isConnected;
        note(wasConnected ? "connected as a device: ready to be cast to" : "connection lost");
      }
      if (snapshot.isRefused) {
        note("the server refused this receiver's sign-in");
        this.options.onRefused();
      }
      const handed = snapshot.receive;
      if (handed !== undefined) {
        live.takeReceive();
        if (handed.castId !== this.castId) {
          this.receive(handed);
        }
      }
    });
    live.open();
    return () => {
      stop();
      live.close();
      this.endCast();
    };
  }

  /** Takes the cast handed, and plays in its session. */
  private receive(handed: CastToReceive): void {
    const { api, token, reached, note } = this.options;
    this.endCast();
    this.castId = handed.castId;
    note(`cast ${String(handed.castId)} from ${handed.fromDeviceName}: ${handed.itemName}`);
    void api
      .POST("/me/casts/{id}/accept", {
        params: { path: { id: handed.castId } },
        headers: { "Idempotency-Key": crypto.getRandomValues(new Uint32Array(4)).join("-") },
      })
      .then(dataOrThrow)
      .catch((cause: unknown) => {
        note(`could not take the cast: ${failureText(cause)}`);
      });
    const together = createTogetherConnection({
      credential: handed.membershipCredential,
      sessionToken: token,
      client: "cast-receiver",
      openSocket: reached.openSocket,
      monotonicMs: () => performance.now(),
    });
    together.setPlayer(() => this.playerState());
    together.subscribe(() => {
      this.follow(together);
    });
    this.together = together;
    together.open();
  }

  /** Does what following the session asks, or ends the cast where the session ended. */
  private follow(together: TogetherConnection): void {
    if (together !== this.together) {
      return;
    }
    const snapshot = together.snapshot();
    if (snapshot.endReason !== undefined) {
      this.options.note(`the cast ended: ${snapshot.endReason}`);
      this.endCast();
      return;
    }
    const expectedMs = expectedGroupPositionMs(snapshot, performance.now());
    for (const each of stepsToFollow(snapshot.group, expectedMs, this.playerNow())) {
      this.apply(each);
    }
  }

  private apply(step: FollowStep): void {
    const { video, note } = this.options;
    switch (step.step) {
      case "load": {
        void this.load(step.fileId, step.positionMs);
        return;
      }
      case "stop": {
        this.stopFilm();
        return;
      }
      case "seek": {
        video.currentTime = step.positionMs / 1000;
        return;
      }
      case "play": {
        void video.play().catch((cause: unknown) => {
          note(`play refused: ${failureText(cause)}`);
        });
        return;
      }
      case "pause": {
        video.pause();
        return;
      }
    }
  }

  /** Starts the session's film on the core's ladder, at positionMs. */
  private async load(fileId: number, positionMs: number): Promise<void> {
    const { api, token, reached, video, note } = this.options;
    this.stopFilm();
    this.playingFileId = fileId;
    try {
      const decision = dataOrThrow(
        await api.POST("/playback", {
          body: { fileId, playableFormats: receiverFormats(), cause: "playPage" },
        }),
      );
      if (this.playingFileId !== fileId) {
        return;
      }
      const played = decision.onLadder?.url ?? decision.url;
      note(`playing file ${String(fileId)} from ${(positionMs / 1000).toFixed(1)} s: ${played}`);
      this.playback = playChunked(video, new URL(played, globalThis.location.origin).href, {
        onFailed: (failure) => {
          note(`failed: ${failure}`);
        },
        startAtBps: decision.onLadder?.startAtBps ?? 0,
        startPositionS: positionMs / 1000,
        monotonicMs: () => performance.now(),
        authorization: `Bearer ${token}`,
        fetch: reached.fetch,
      });
      this.together?.reportState();
    } catch (cause) {
      note(`could not start file ${String(fileId)}: ${failureText(cause)}`);
    }
  }

  private playerNow(): PlayerNow {
    const { video } = this.options;
    return {
      fileId: this.playingFileId,
      positionMs: Math.round(video.currentTime * 1000),
      isPlaying: !video.paused,
    };
  }

  private playerState(): PlayerState {
    const { video } = this.options;
    return {
      ...this.playerNow(),
      // Not ready to play along: loading or seeking (HAVE_FUTURE_DATA is 3).
      isBuffering: this.playingFileId !== 0 && (video.seeking || video.readyState < 3),
    };
  }

  private stopFilm(): void {
    this.playback?.destroy();
    this.playback = undefined;
    this.playingFileId = 0;
  }

  private endCast(): void {
    this.together?.close();
    this.together = undefined;
    this.castId = undefined;
    this.stopFilm();
  }
}
