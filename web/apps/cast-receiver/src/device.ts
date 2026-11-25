// SPDX-License-Identifier: MPL-2.0

import { playChunked, type ChunkedPlayback } from "@orpheum/player";
import {
  browserPlayableFormats,
  createLiveConnection,
  createTogetherConnection,
  dataOrThrow,
  putOnCommands,
  expectedGroupPositionMs,
  failureText,
  type Api,
  type components,
  type CastToReceive,
  type GroupCommand,
  type GroupView,
  type PlayableFormats,
  type PlayerState,
  type TogetherConnection,
} from "@orpheum/sdk";

import type { Reached } from "./reach.ts";
import { UpNextCard } from "./up-next.ts";

/** What the core decides for a film the receiver plays. */
type PlaybackDecision = components["schemas"]["StartPlaybackOutputBody"];

/**
 * The receiver as a cast target (inventory B-10; the owner, 2026-11-07: "initial chromecast
 * support"): signed in as a device of its sender's person by the session a cast's grant traded for
 * (guest.ts, D465 §2), it keeps a live connection, so it is listed in the Cast control as any device
 * connected now (D337); a cast handed to it is taken at once, the Chromecast having no remote to say
 * Use with, and played in the cast's session as a browser receiver plays it (D434), so the sender's
 * remote works it: it plays the film each hand of the cast names, putting it on the session it
 * leads, and follows the session's play, pause and position; sound, picture and the cast's other
 * panels wait on what the device shows.
 */

/**
 * How far the receiver may be from the session's position before it seeks there, in milliseconds:
 * past a chunk's start-up, so a film starting or a seek just made is not sought again.
 */
export const seekPastMs = 3000;

/** Where the receiver's player stands. */
export interface PlayerNow {
  /** The film it plays, 0 for none. */
  readonly fileId: number;
  readonly positionMs: number;
  readonly isPlaying: boolean;
}

/** One thing the receiver's player does to follow its session. */
export type FollowStep =
  | {
      readonly step: "load";
      readonly fileId: number;
      /** The title it plays as, the group's (B0 question 20). */
      readonly itemId?: number | undefined;
      readonly positionMs: number;
    }
  | { readonly step: "stop" }
  | { readonly step: "seek"; readonly positionMs: number }
  | { readonly step: "play" }
  | { readonly step: "pause" };

/**
 * The group the receiver follows: none while it is not yet on handedFileId, the film the cast
 * handed, which the receiver itself puts on the session it leads (putOnCommands); until then the
 * group plays nothing, which is no stop.
 */
export function groupToFollow(
  group: GroupView | undefined,
  handedFileId: number,
): GroupView | undefined {
  return handedFileId !== 0 && group?.fileId !== handedFileId ? undefined : group;
}

/**
 * What the player does to follow the session's group, expectedMs being where the group stands now:
 * the group's film loaded at its position, or none; then played where the group plays or waits for
 * its leader to (the receiver leads), paused where it is paused, sought
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
  // A waiting group is to play, standing until its leader plays (D371 §1), and in a cast the
  // receiver is the leader (D337), so it plays, as the web app's leader does (together-leader.ts).
  const isPlaying = group.state === "playing" || group.state === "waiting";
  if (group.fileId !== player.fileId) {
    return [
      { step: "load", fileId: group.fileId, itemId: group.itemId, positionMs },
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

/**
 * What a cast handed to the receiver asks of it, takenCastId being the cast it plays in and
 * handedFileId the film that cast last handed it: a new cast taken; the same cast's film changed,
 * put on (the person played another on the sender, D364 §3, or nothing, ready for the next, §5); or
 * nothing, the same cast and film handed again as the connection came back.
 */
export function handedWhat(
  handed: Pick<CastToReceive, "castId" | "fileId">,
  takenCastId: number | undefined,
  handedFileId: number,
): "take" | "putOn" | "same" {
  if (handed.castId !== takenCastId) {
    return "take";
  }
  return handed.fileId === handedFileId ? "same" : "putOn";
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
 * core puts the film on its ladder, which our player plays.
 */
export function receiverFormats(): PlayableFormats {
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
  /**
   * Called when the server refuses the receiver's session (its device was removed, or the sign-in was
   * not one it takes), with the server's words where it said any.
   */
  readonly onRefused: (reason?: string) => void;
  /**
   * Called once, the first time the core welcomes the live connection: the device is one of its
   * person's connected now, which a cast to it needs (the core refuses one to a device not yet).
   */
  readonly onConnected?: () => void;
}

/** The receiver as a device of its person's: its live connection, and the cast it plays. */
export class CastDevice {
  private castId: number | undefined;
  private together: TogetherConnection | undefined;
  private playback: ChunkedPlayback | undefined;
  private playingFileId = 0;
  /** The film the cast last handed, 0 for none: the one the receiver puts on its session. */
  private handedFileId = 0;
  /** The commands putting the handed film on, until the session is connected to take them. */
  private toPutOn: readonly GroupCommand[] = [];
  /** What plays after the film, shown near its end; chosen on the caster (D471). */
  private readonly upNext: UpNextCard;

  constructor(private readonly options: CastDeviceOptions) {
    this.upNext = new UpNextCard(options.video, options.api, options.note);
    for (const type of reportedVideoEvents) {
      options.video.addEventListener(type, () => {
        this.together?.reportState();
      });
    }
    options.video.addEventListener("ended", () => {
      this.waitsReadyAtTheEnd();
    });
  }

  /**
   * At its cast's film's end with nothing after it that can be read now, has the cast play nothing,
   * as a browser receiver does (the web app's readyForTheNext, D364 §5): the server hands the cast
   * again so, the film is taken off and the receiver's black screen shows, still connected, for the
   * next video the person casts, until its idle close ends it (idle.ts). Where a next episode is
   * offered, the caster's Up next moves the cast on, or leaves it at its end (D471).
   */
  private waitsReadyAtTheEnd(): void {
    const { castId } = this;
    const { api, note } = this.options;
    if (castId === undefined || this.playingFileId === 0 || this.upNext.hasNext()) {
      return;
    }
    note("the film ended with nothing next: the cast plays nothing, ready for the next");
    void api
      .POST("/me/casts/{id}/play", { params: { path: { id: castId } }, body: {} })
      .then(dataOrThrow)
      .catch((cause: unknown) => {
        note(`could not have the cast wait ready: ${failureText(cause)}`);
      });
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
    let hasConnected = false;
    const heardConnected = (isConnected: boolean): void => {
      if (isConnected === wasConnected) {
        return;
      }
      wasConnected = isConnected;
      note(isConnected ? "connected as a device: ready to be cast to" : "connection lost");
      if (isConnected && !hasConnected) {
        hasConnected = true;
        this.options.onConnected?.();
      }
    };
    const stop = live.subscribe(() => {
      const snapshot = live.snapshot();
      heardConnected(snapshot.isConnected);
      if (snapshot.isRefused) {
        const { refusalReason } = snapshot;
        note(
          refusalReason === undefined
            ? "the server refused this receiver's sign-in"
            : `the server refused this receiver's sign-in: ${refusalReason}`,
        );
        this.options.onRefused(refusalReason);
      }
      const handed = snapshot.receive;
      if (handed !== undefined) {
        live.takeReceive();
        const what = handedWhat(handed, this.castId, this.handedFileId);
        if (what === "take") {
          this.receive(handed);
        } else if (what === "putOn") {
          this.options.note(`the cast plays ${handed.fileId === 0 ? "nothing" : handed.itemName}`);
          this.hand(handed);
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

  /** Takes the cast handed, and plays its film, leading its session. */
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
    this.hand(handed);
  }

  /**
   * Plays the film the cast handed, or nothing for none, and puts it on the session the receiver
   * leads once the film is started: the group has no film until the receiver plays one.
   */
  private hand(handed: CastToReceive): void {
    this.handedFileId = handed.fileId;
    this.toPutOn = [];
    if (handed.fileId === 0) {
      this.stopFilm();
      return;
    }
    const { fileId, isPaused, itemId } = handed;
    void this.load(fileId, handed.positionMs, itemId).then((startedAtMs) => {
      if (startedAtMs !== undefined && this.handedFileId === fileId) {
        this.toPutOn = putOnCommands({ fileId, itemId }, startedAtMs, isPaused === true);
        this.putOn();
      }
    });
  }

  /** Sends the commands putting the handed film on, once the session is connected to take them. */
  private putOn(): void {
    const together = this.together;
    if (together === undefined || this.toPutOn.length === 0) {
      return;
    }
    const [first, ...rest] = this.toPutOn;
    if (first !== undefined && together.command(first)) {
      for (const each of rest) {
        together.command(each);
      }
      this.toPutOn = [];
    }
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
    this.putOn();
    const expectedMs = expectedGroupPositionMs(snapshot, performance.now());
    const group = groupToFollow(snapshot.group, this.handedFileId);
    const steps = stepsToFollow(group, expectedMs, this.playerNow());
    for (const each of steps) {
      this.apply(each);
    }
  }

  private apply(step: FollowStep): void {
    const { video, note } = this.options;
    switch (step.step) {
      case "load": {
        void this.load(step.fileId, step.positionMs, step.itemId);
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

  /**
   * Starts a film on the core's ladder, at positionMs, or where the person left it for undefined, as
   * the title itemId (B0 question 20); gives where it started, or undefined where it did not.
   */
  private async load(
    fileId: number,
    atMs: number | undefined,
    itemId?: number,
  ): Promise<number | undefined> {
    const { token, reached, video, note } = this.options;
    this.stopFilm();
    this.playingFileId = fileId;
    try {
      const decision = await this.decisionFor(fileId, itemId);
      if (this.playingFileId !== fileId) {
        return undefined;
      }
      const positionMs = atMs ?? decision.resumePositionMs;
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
      this.upNext.plays(fileId, decision.segments ?? []);
      return positionMs;
    } catch (cause) {
      note(`could not start file ${String(fileId)}: ${failureText(cause)}`);
      return undefined;
    }
  }

  /** The core's decision for playing fileId here, as the title itemId where one is given. */
  private async decisionFor(fileId: number, itemId: number | undefined): Promise<PlaybackDecision> {
    return dataOrThrow(
      await this.options.api.POST("/playback", {
        body: {
          fileId,
          ...(itemId === undefined ? {} : { itemId }),
          playableFormats: receiverFormats(),
          cause: "playPage",
        },
      }),
    );
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
    this.upNext.plays(0);
  }

  private endCast(): void {
    // Let go of the session before closing it: its close tells its listener, follow, which ended the
    // cast again while it was still this cast's, over and over until the stack ran out, and the
    // receiver took no cast after (the owner's Chromecast, 2026-11-12).
    const together = this.together;
    this.together = undefined;
    together?.close();
    this.castId = undefined;
    this.handedFileId = 0;
    this.toPutOn = [];
    this.stopFilm();
  }
}
