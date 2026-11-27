// SPDX-License-Identifier: MPL-2.0

import { playChunked, type ChunkedPlayback } from "@orpheum/player";
import {
  browserPlayableFormats,
  castReportEveryMs,
  createLiveConnection,
  createTogetherConnection,
  dataOrThrow,
  everyRight,
  followGroup,
  putOnCommands,
  requestPlayback,
  expectedGroupPositionMs,
  failureText,
  leavePlayback,
  NextWarmUp,
  playOnWarmUp,
  tellsNoFilmHere,
  workedByTheRemote,
  type Api,
  type CarriesPictureChoices,
  type CarriesSoundChoices,
  type components,
  type CastToReceive,
  type GroupCommand,
  type GroupView,
  type PlayableFormats,
  type TellsCastState,
  type TellsPlaybackAlone,
  type TogetherConnection,
  type WarmUp,
  type WarmUpAskedAgain,
} from "@orpheum/sdk";

import { systemVolumeOf } from "./caf.ts";
import { ReceiverPicturePanel } from "./picture-panel.ts";
import type { Reached } from "./reach.ts";
import { ReceiverSoundPanel, type SoundAudio } from "./sound-panel.ts";
import { UpNextCard, type NextShown } from "./up-next.ts";

/** What the core decides for a film the receiver plays. */
type PlaybackDecision = components["schemas"]["StartPlaybackOutputBody"];

/** A film's sound and subtitles, as its remote works them. */
interface FilmPanels {
  readonly sound: ReceiverSoundPanel;
  readonly picture: ReceiverPicturePanel;
}

/** The session a cast is played in, as the receiver works it. */
type CastSession = TogetherConnection &
  TellsCastState &
  TellsPlaybackAlone &
  CarriesSoundChoices &
  CarriesPictureChoices;

/**
 * The receiver as a cast target (inventory B-10; the owner, 2026-11-07: "initial chromecast
 * support"): signed in as a device of its sender's person by the session a cast's grant traded for
 * (guest.ts, D465 §2), it keeps a live connection, so it is listed in the Cast control as any device
 * connected now (D337); a cast handed to it is taken at once, the Chromecast having no remote to say
 * Use with, and played in the cast's session as a browser receiver plays it (D434), so the sender's
 * remote works it: it plays the film each hand of the cast names, putting it on the session it
 * leads, and follows the session's play, pause and position by the SDK's follower, as every player
 * does (followGroup, D555 §3). Its remote works its sound and subtitles too, as a browser
 * receiver's (sound-panel.ts, picture-panel.ts; the owner, 2026-11-26: "Check what the chromecast is
 * actually capable of. Anything it can do, I want."), its volume the Cast device's own, which its
 * remote sets through the cast's session as every other control (D557 §5).
 */

/**
 * What the receiver does to have the session's film on its player: the group's film loaded at where
 * the group stands now, or none stopped. Playing, pausing and seeking along are the SDK's follower's
 * (followGroup), as on the Play page and a browser receiver (D555 §3).
 */
export type FilmStep =
  | {
      readonly step: "load";
      readonly fileId: number;
      /** The title it plays as, the group's (B0 question 20). */
      readonly itemId?: number | undefined;
      readonly positionMs: number;
    }
  | { readonly step: "stop" };

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
 * The step that has the receiver's player, playing playingFileId (0 for none), on the group's film,
 * expectedMs being where the group stands now: its film loaded at that position, or none stopped;
 * nothing where it plays the group's film already, or before it is told the group.
 */
export function filmToFollow(
  group: GroupView | undefined,
  expectedMs: number | undefined,
  playingFileId: number,
): FilmStep | undefined {
  if (group === undefined || group.fileId === playingFileId) {
    return undefined;
  }
  if (group.fileId === 0) {
    return { step: "stop" };
  }
  const positionMs = Math.max(0, expectedMs ?? group.positionMs);
  return { step: "load", fileId: group.fileId, itemId: group.itemId, positionMs };
}

/**
 * What a cast handed to the receiver asks of it, taken being the cast it plays in, the credential of
 * the session it plays it in and the film that cast last handed it: a new cast taken; the same cast
 * moved to another session, joined (its person started, joined or left a Watch Together session, and
 * the cast went with them, D555 §7: a receiver staying in the session it left is silent in the new
 * one, and its cast ends as gone); the same cast's film changed, put on (the person played another
 * on the sender, D364 §3, or nothing, ready for the next, §5); or nothing, the same cast, session and
 * film handed again as the connection came back.
 */
export function handedWhat(
  handed: Pick<CastToReceive, "castId" | "fileId" | "membershipCredential">,
  taken: {
    readonly castId: number | undefined;
    readonly credential: string;
    readonly fileId: number;
  },
): "take" | "join" | "putOn" | "same" {
  if (handed.castId !== taken.castId) {
    return "take";
  }
  if (handed.membershipCredential !== taken.credential) {
    return "join";
  }
  return handed.fileId === taken.fileId ? "same" : "putOn";
}

/**
 * What the receiver asks to play: the device's own codecs, by no container of a file's own, so the
 * core puts the film on its ladder, which our player plays.
 */
export function receiverFormats(): PlayableFormats {
  return { ...browserPlayableFormats(), containers: [] };
}

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
  /** Starts a film on the video: our chunked player, a test's stand-in where one is given. */
  readonly startsFilm?: typeof playChunked | undefined;
  /** Web Audio's parts of the sound's panel, a test's stand-ins where given. */
  readonly audio?: SoundAudio | undefined;
  /** The clock a warmed next episode's age is read by: performance.now, a test's where given. */
  readonly nowMs?: (() => number) | undefined;
}

/** The receiver as a device of its person's: its live connection, and the cast it plays. */
/**
 * Where a film on the core's ladder, onLadder, starts: within the speed its offer gives, and whether
 * its node's speed held it there, so its player jumps once to its link's quality (StartJump;
 * start-quality-by-node-speed.md §4); at the lowest, not held, off the ladder.
 */
function ladderStartOf(
  onLadder:
    | { readonly startAtBps: number; readonly isStartHeldByNodeSpeed?: boolean | undefined }
    | undefined,
): { readonly startAtBps: number; readonly isStartHeldByNodeSpeed: boolean } {
  return {
    startAtBps: onLadder?.startAtBps ?? 0,
    isStartHeldByNodeSpeed: onLadder?.isStartHeldByNodeSpeed === true,
  };
}

export class CastDevice {
  private castId: number | undefined;
  private together: CastSession | undefined;
  private playback: ChunkedPlayback | undefined;
  /** The SDK's follower of the session's group for the film playing, or the telling of none. */
  private stopFollowing: (() => void) | undefined;
  /** The film's sound and subtitles as its remote works them, and the stopping of its working. */
  private panels: { readonly picture: ReceiverPicturePanel; readonly stop: () => void } | undefined;
  private playingFileId = 0;
  /** The film the cast last handed, 0 for none: the one the receiver puts on its session. */
  private handedFileId = 0;
  /** The credential of the session the cast is played in, empty for none. */
  private credential = "";
  /** The commands putting the handed film on, until the session is connected to take them. */
  private toPutOn: readonly GroupCommand[] = [];
  /** What plays after the film, shown near its end; chosen on the caster (D471). */
  private readonly upNext: UpNextCard;
  /** The next episode's decision, asked as its Up next shows, until it plays (warmUpNext). */
  private readonly nextWarm: NextWarmUp<PlaybackDecision>;

  constructor(private readonly options: CastDeviceOptions) {
    this.nextWarm = new NextWarmUp({
      leave: (token) => {
        leavePlayback(options.api, token, "replaced");
      },
      nowMs: () => this.nowMs(),
    });
    this.upNext = new UpNextCard(options.video, options.api, options.note);
    this.upNext.tellsShown((next) => {
      this.warmUpNext(next);
    });
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
        const what = handedWhat(handed, {
          castId: this.castId,
          credential: this.credential,
          fileId: this.handedFileId,
        });
        if (what === "take") {
          this.receive(handed);
        } else if (what === "join") {
          this.options.note(`cast ${String(handed.castId)} moved to another session`);
          this.leaveSession();
          this.join(handed);
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
    const { api, note } = this.options;
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
    this.join(handed);
  }

  /** Joins the session the cast is handed in, and plays its film there. */
  private join(handed: CastToReceive): void {
    const { token, reached } = this.options;
    this.credential = handed.membershipCredential;
    const together = createTogetherConnection({
      credential: handed.membershipCredential,
      sessionToken: token,
      client: "cast-receiver",
      openSocket: reached.openSocket,
      monotonicMs: () => performance.now(),
      stateEveryMs: castReportEveryMs, // its silence ends the cast (D555 §3)
    });
    together.subscribe(() => {
      this.follow(together);
    });
    this.together = together;
    // Nothing on yet, it still says where it is: a castee silent for 20 s ends its cast (D555 §3).
    this.stopFollowing = tellsNoFilmHere(together);
    together.setCastState("starting"); // running once its film plays, or at once for none (D555 §4)
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
      this.together?.setCastState("running");
      return;
    }
    const { fileId, isPaused, itemId } = handed;
    void this.load(fileId, handed.positionMs, itemId).then((startedAtMs) => {
      if (startedAtMs !== undefined && this.handedFileId === fileId) {
        this.toPutOn = putOnCommands({ fileId, itemId }, startedAtMs, isPaused === true);
        this.putOn();
        this.together?.setCastState("running");
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
    if (snapshot.endReason === "castMoved") {
      // No end: the cast moved into its person's part elsewhere, and is handed again there (join).
      this.options.note("the cast's own session was let go as it moved");
      return;
    }
    if (snapshot.endReason !== undefined) {
      this.options.note(`the cast ended: ${snapshot.endReason}`);
      this.endCast();
      return;
    }
    this.putOn();
    const expectedMs = expectedGroupPositionMs(snapshot, performance.now());
    const group = groupToFollow(snapshot.group, this.handedFileId);
    const step = filmToFollow(group, expectedMs, this.playingFileId);
    if (step?.step === "load") {
      void this.load(step.fileId, step.positionMs, step.itemId);
    } else if (step?.step === "stop") {
      this.stopFilm();
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
    const startsFilm = this.options.startsFilm ?? playChunked;
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
      const panels = this.panelsFor(decision);
      const { sound } = panels;
      this.playback = startsFilm(video, new URL(played, globalThis.location.origin).href, {
        onFailed: (failure) => {
          note(`failed: ${failure}`);
        },
        ...ladderStartOf(decision.onLadder),
        startPositionS: positionMs / 1000,
        monotonicMs: () => performance.now(),
        authorization: `Bearer ${token}`,
        fetch: reached.fetch,
        ...sound.startingSound(),
      });
      sound.plays(this.playback);
      this.followTheGroup(fileId, decision, panels);
      this.upNext.plays(fileId, decision.segments ?? []);
      return positionMs;
    } catch (cause) {
      note(`could not start file ${String(fileId)}: ${failureText(cause)}`);
      return undefined;
    }
  }

  /** The film's sound and subtitles as its remote works them, from the core's decision. */
  private panelsFor(decision: PlaybackDecision): FilmPanels {
    const { token, reached, video, note } = this.options;
    // An older core's decision may lack the parts the panels read: they then offer nothing.
    const told: Partial<PlaybackDecision> = decision;
    const ways = { video, note };
    return {
      sound: new ReceiverSoundPanel(
        told,
        { ...ways, volume: systemVolumeOf() },
        this.options.audio,
      ),
      picture: new ReceiverPicturePanel(told, ways, {
        fetch: reached.fetch,
        authorization: `Bearer ${token}`,
        language: navigator.language,
      }),
    };
  }

  /**
   * Up next shows next, the episode that plays after the film: its decision is asked now, so the
   * server starts making its chunks, and the receiver plays on it once the cast moves on to it, as a
   * browser's Up next warms its own (the web app's Watching.warmUpNext; the owner, 2026-11-27: a
   * Chromecast's next episode started cold, 10 to 20 s). The film playing keeps its playbacks; nothing
   * is preloaded beside it; a different next replaces it, and at most one is kept (NextWarmUp).
   */
  private warmUpNext(next: NextShown): void {
    if (next.fileId === this.playingFileId) {
      return;
    }
    const { fileId, itemId } = next;
    this.nextWarm.warm(fileId, itemId, () => this.ask(fileId, itemId, "upNext"));
  }

  /**
   * The core's decision for playing fileId here, as the title itemId where one is given: the next
   * episode's, warmed as its Up next showed, where it is fileId's as that title, fresh and on the
   * film's ladder (playOnWarmUp); else one asked now. Any other warm-up kept is let go of.
   */
  private async decisionFor(fileId: number, itemId: number | undefined): Promise<PlaybackDecision> {
    const warm = this.nextWarm.take(fileId);
    if (warm === undefined) {
      return await this.ask(fileId, itemId, "playPage");
    }
    if (warm.itemId !== itemId) {
      this.letGoOf(warm);
      return await this.ask(fileId, itemId, "playPage");
    }
    return await playOnWarmUp(warm, {
      nowMs: this.nowMs(),
      askAgain: (cause) => this.ask(fileId, itemId, cause),
      stale: () => {
        this.letGoOf(warm);
      },
      offLadder: (decision) => {
        leavePlayback(this.options.api, decision.mediaToken, "replaced");
      },
      played: (decision) => decision,
    });
  }

  /** Asks the core how fileId plays here, as the title itemId where one is given, and why. */
  private ask(
    fileId: number,
    itemId: number | undefined,
    cause: "playPage" | "upNext" | WarmUpAskedAgain,
  ): Promise<PlaybackDecision> {
    return requestPlayback(this.options.api, {
      fileId,
      ...(itemId === undefined ? {} : { itemId }),
      playableFormats: receiverFormats(),
      cause,
    });
  }

  /** Lets go of a warm-up not played on: its playback, once decided. */
  private letGoOf(warm: WarmUp<PlaybackDecision>): void {
    warm.decision.then(
      (decision) => {
        leavePlayback(this.options.api, decision.mediaToken, "replaced");
      },
      () => undefined,
    );
  }

  private nowMs(): number {
    return this.options.nowMs?.() ?? performance.now();
  }

  /**
   * Has the SDK's follower play the film fileId along with the session, as the Play page and a
   * browser receiver do (D555 §3): leading, as in a cast of its own, it carries out each move its
   * remote has the group make and holds the group while it loads; following, in its person's Watch
   * Together session, it keeps to the group. It tells the session its buffering, its loaded chunks,
   * its quality and how it is given the film.
   */
  private followTheGroup(
    fileId: number,
    decision: PlaybackDecision,
    { sound, picture }: FilmPanels,
  ): void {
    const { together } = this;
    if (together === undefined) {
      picture.destroy();
      return;
    }
    this.stopFollowing?.();
    const stopSound = workedByTheRemote(together.subscribeToSoundChoices, sound, {
      cast: together,
      led: together,
    });
    const stopPicture = workedByTheRemote(together.subscribeToPictureChoices, picture, {
      cast: together,
      led: together,
    });
    this.panels = {
      picture,
      stop: () => {
        stopSound();
        stopPicture();
      },
    };
    this.stopFollowing = followGroup(this.options.video, together, {
      fileId,
      leads: everyRight, // the person's own playing device, its remote's every move theirs
      monotonicMs: () => performance.now(),
      // Its file as it is only where the core gave it so, as the Play page says it (playsOwnFile).
      delivery:
        decision.onLadder === undefined && decision.delivery === "direct" ? "direct" : "converted",
      readSound: sound.read,
      readPicture: picture.read,
    });
  }

  private stopFilm(): void {
    this.stopFollowing?.();
    this.stopFollowing = undefined;
    this.panels?.stop();
    this.panels?.picture.destroy();
    this.panels = undefined;
    if (this.together !== undefined) {
      this.stopFollowing = tellsNoFilmHere(this.together);
    }
    this.playback?.destroy();
    this.playback = undefined;
    this.playingFileId = 0;
    this.upNext.plays(0);
  }

  private endCast(): void {
    this.leaveSession();
    this.castId = undefined;
  }

  /** Leaves the session the cast played in, its film off, the cast still taken. */
  private leaveSession(): void {
    // Let go of the session before closing it: its close tells its listener, follow, which ended the
    // cast again while it was still this cast's, over and over until the stack ran out, and the
    // receiver took no cast after (the owner's Chromecast, 2026-11-12).
    const together = this.together;
    this.together = undefined;
    together?.close();
    this.credential = "";
    this.handedFileId = 0;
    this.toPutOn = [];
    this.nextWarm.letGo();
    this.stopFilm();
  }
}
