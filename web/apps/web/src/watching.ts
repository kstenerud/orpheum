// SPDX-License-Identifier: MPL-2.0

import { isWebAudioLate, type ChunkedPlayback } from "@orpheum/player";
import type { Api, components, PlayableFormats } from "@orpheum/sdk";

import { browserStorage } from "./browser-storage.ts";
import { noteDecisionTimes } from "./decision-times.ts";
import type { LadderOffer } from "./moving-down.ts";
import { leavePlayback } from "./interest.ts";
import { dataOrThrow } from "./problem.ts";
import { soundLatencyMemory } from "./sound-latency.ts";
import { monotonicMs } from "./use-together.ts";

/** The server's decision on how a file plays here (cut-a §6). */
export type PlaybackDecision = components["schemas"]["StartPlaybackOutputBody"];

/** Whether decision plays the film's own file, as it is, rather than chunks of its ladder. */
export function playsOwnFile(decision: PlaybackDecision): boolean {
  return decision.delivery === "direct" && decision.onLadder === undefined;
}

/**
 * Why a page asks the server for a playback, for the server's log alone: a phone's playback started
 * over by itself and nothing said what started it (staging, 2026-10-14).
 */
export type StartCause = NonNullable<components["schemas"]["StartPlaybackInputBody"]["cause"]>;

/**
 * Why a page asks for a playback, and, when it asks again from where its video stood, where it
 * starts it in place of the resume position answered: both for the server's log alone. A film whose
 * audio track was changed three minutes in started again from its beginning, and the log said only
 * resumePositionMs=0 (staging, 2026-10-26).
 */
export interface StartAsked {
  readonly cause: StartCause;
  readonly startsAtMs?: number | undefined;
}

/** How many playbacks this page has asked for since it loaded, which the server logs beside each. */
const pageStarts = { count: 0 };

/** Asks the server how a file plays in this browser, and why it asks; nothing of where it is (D247 §11). */
export async function requestPlayback(
  api: Api,
  fileId: number,
  asked: StartAsked & { readonly playableFormats: PlayableFormats },
): Promise<PlaybackDecision> {
  const askedMs = performance.now();
  pageStarts.count += 1;
  const decision = dataOrThrow(
    await api.POST("/playback", {
      body: {
        fileId,
        playableFormats: asked.playableFormats,
        cause: asked.cause,
        pageStarts: pageStarts.count,
        // Its player gave the film's own picture up, which the core keeps before it decides (D323 §2).
        ...(asked.cause === "ownPictureGivenUp" ? { hasGivenUpOwnPicture: true } : {}),
        // This device's sound through Web Audio plays too late to raise a sound by: a raise levelling
        // wants is its ladder's sound's (D436).
        ...(isWebAudioLate(soundLatencyMemory()) ? { isWebAudioLate: true } : {}),
        // The person chose the film's own file over its ladder.
        ...(asked.cause === "originalChosen" ? { preferredDelivery: "direct" as const } : {}),
        ...(asked.startsAtMs === undefined ? {} : { startsAtMs: Math.round(asked.startsAtMs) }),
      },
    }),
  );
  noteDecisionTimes(decision.mediaToken, { askedMs, decidedMs: performance.now() });
  return decision;
}

/**
 * How long a film page's decision may wait for its Play page before a new one is asked for: half
 * the least mediaTokenIdleExpiryHours can be, an hour, so its token is never idled out by the
 * time it plays.
 */
export const warmUpKeptMs = 30 * 60_000;

/** A hidden player filling a film's first buffer on its page (D260 §3). */
export interface Preload {
  /** The BANDWIDTH of the quality its stepping settled on so far; undefined before it loads one. */
  readonly settledAtBps: () => number | undefined;
  /** Its player's last steps (diagnosisOf), for the playback's startup report; undefined for none. */
  readonly steps: () => string | undefined;
  readonly stop: () => void;
}

/**
 * How far ahead a film page's hidden player loads: its first buffer, the first chunk or few of its
 * film (one to five, by where its keyframes fall: one where its chunks are 10 s, D414 §5), not the
 * half minute hls.js loads by itself. Three or more chunks of a film's
 * own picture, 12 MB of Bram's, held a phone's link after its person had moved on (2026-10-07).
 */
export const preloadAheadS = 10;

/**
 * The browser's key that, set to "1", starts each film on its own picture with no film page preload
 * (see startsOnFilmsPicture). Set in the console:
 * localStorage.setItem("orpheum.debug.startOnFilmsPicture", "1").
 */
export const startOnFilmsPictureKey = "orpheum.debug.startOnFilmsPicture";

/**
 * Whether this browser is set to start each film on its own picture, the ladder's top, from the Play
 * page itself: a debugging aid, so the Play page's player meets what the browser cannot play there
 * and not the film page's preload, which met it first and stepped down out of sight (the owner's
 * test of Firefox on macOS, 2026-10-18).
 */
function startsOnFilmsPicture(): boolean {
  return browserStorage().getItem(startOnFilmsPictureKey) === "1";
}

/** The start rate that puts a player on a ladder's top quality. */
const topOfTheLadderBps = Number.MAX_SAFE_INTEGER;

/**
 * The ladder offers whose start the page chose itself, a warm-up's settled quality or the ladder's
 * top (startsOnFilmsPicture), which the player starts from as given; every other playback's own
 * offer the player starts by the link it remembers (D381).
 */
const startsThePageChose = new WeakSet<object>();

/** offer with the start the page chose, startAtBps, kept as the page's (isStartThePages). */
function withThePagesStart(offer: LadderOffer, startAtBps: number): LadderOffer {
  const chosen = { ...offer, startAtBps };
  startsThePageChose.add(chosen);
  return chosen;
}

/** Whether offer's start is one the page chose itself, which the player starts from as given. */
export function isStartThePages(offer: object): boolean {
  return startsThePageChose.has(offer);
}

/** Starts a Preload of the film's ladder from where its playback resumes. */
export type StartPreload = (offer: LadderOffer, positionMs: number) => Preload;

/**
 * A Preload in a muted video the page never shows: hls.js loads the first buffer from the offered
 * quality and steps down by itself if the link cannot fill it, and the chunks it loads are kept by
 * the browser for the Play page's player. One that fails is the Play page's to try again. The player
 * and its start (moving-down.ts, which reads the player) are imported as the preload starts, not
 * with the film's page: imported with it, the title and series
 * pages drew only once hls.js had loaded and compiled (the web client's architecture audit of
 * 2026-11-10, Playback 2). The Play page's code, hls.js with it, is warmed apart (warm-play.ts).
 */
export const preloadInHiddenVideo: StartPreload = (offer, positionMs) => {
  const video = document.createElement("video");
  video.muted = true;
  video.hidden = true;
  video.playsInline = true;
  document.body.append(video);
  let playback: ChunkedPlayback | undefined = undefined;
  let stepsOf: (() => string | undefined) | undefined = undefined;
  let isStopped = false;
  Promise.all([import("@orpheum/player"), import("./moving-down.ts")]).then(
    ([{ diagnosisOf, playChunked }, { startOn }]) => {
      if (isStopped) {
        return;
      }
      playback = playChunked(video, offer.url, {
        onFailed: () => undefined,
        // A playback's own start, as the Play page's player starts it (D381).
        ...startOn({ ...offer, shouldStartByRememberedLink: !isStartThePages(offer) }, positionMs),
        monotonicMs,
        loadsAheadAtMostS: preloadAheadS,
      });
      stepsOf = () => diagnosisOf(video)?.steps;
    },
    // A player that did not load preloads nothing: the Play page loads it again as it shows.
    () => undefined,
  );
  return {
    settledAtBps: () => playback?.settledAtBps(),
    steps: () => stepsOf?.(),
    stop: () => {
      isStopped = true;
      playback?.destroy();
      video.remove();
    },
  };
};

/**
 * The film a person is watching in this tab, and the playbacks asked for it, which the server keeps
 * working on until the person shows they want another film (D258 §2): opening another film's page,
 * or playing another. Going from the player back to the film's page, or anywhere else, keeps it
 * (the owner, 2026-10-07: "Until the user signals intent to watch a different video, we should keep
 * data").
 *
 * A film's page warms it up (D258 §1): it asks how the film plays at once, so the server starts its
 * chunks, and the Play page takes that decision instead of asking again. A film
 * on its ladder is preloaded too, where its media do not go over WebRTC, and the Play page's player
 * starts at the quality the preload settled on, its chunks already in the browser (D260 §3).
 */
export class Watching {
  private fileId: number | undefined;
  // The playbacks asked for the film, by their media tokens, which a move to another lets go of.
  private readonly tokens = new Set<string>();
  // The film page's decision, until the Play page takes it, and when it was asked for.
  private warm: Warm | undefined;

  constructor(
    private readonly api: Api,
    private readonly nowMs: () => number = () => performance.now(),
    private readonly startPreload: StartPreload = preloadInHiddenVideo,
  ) {}

  /**
   * A film's page was opened: the film is warmed up, and any other one let go of. Its preload is
   * started only where the media do not go over WebRTC, as isOverWebrtc says once the decision is
   * in: there the service worker builds each chunk's answer from the page's transport and the
   * browser's HTTP cache keeps none, so the Play page loaded every chunk of the preload again, ~10 s
   * of film twice over the same link while the person waited (D260 §3's revision of 2026-10-28; the
   * efficiency audit's finding 1). The decision is taken all the same, so the server's chunks start.
   */
  warmUp(
    fileId: number,
    playableFormats: PlayableFormats,
    isOverWebrtc: () => boolean = () => false,
  ): void {
    if (this.fileId === fileId && this.warm !== undefined) {
      return;
    }
    this.watch(fileId);
    const decision = this.ask(fileId, { playableFormats, cause: "filmPage" });
    const warm: Warm = { decision, askedMs: this.nowMs(), preload: undefined };
    this.warm = warm;
    decision.then(
      (decided) => {
        // Not once the Play page took it, or another film was chosen.
        if (
          this.warm === warm &&
          decided.onLadder !== undefined &&
          !startsOnFilmsPicture() &&
          !isOverWebrtc()
        ) {
          warm.preload = this.startPreload(decided.onLadder, decided.resumePositionMs);
        }
      },
      // A warm-up that fails is the Play page's to ask again; nothing waits on it here.
      () => undefined,
    );
  }

  /**
   * The Play page asks how the film plays: the film page's decision when it is fresh and on the
   * film's ladder, else a new one, since the warm-up read the keyframes of a film not cut yet and it
   * may be on its ladder now (D260 §1). Another film being watched is let go of first. Where asked
   * says where the page starts the playback, each request says so (StartAsked), and a warm-up, made
   * and preloaded for where the film resumes, is let go of for a new one asked from there: a Play
   * here mid-film took the warm-up's, which the server never heard of, and played from 0:00
   * (staging, 2026-10-27).
   */
  decide(
    fileId: number,
    playableFormats: PlayableFormats,
    asked: StartAsked = { cause: "playPage" },
  ): Promise<PlaybackDecision> {
    this.watch(fileId);
    const warm = this.warm;
    this.warm = undefined;
    const { startsAtMs } = asked;
    if (warm === undefined) {
      return this.ask(fileId, { playableFormats, ...asked });
    }
    if (startsAtMs !== undefined) {
      this.letGoOf(warm);
      return this.ask(fileId, { playableFormats, ...asked });
    }
    if (this.nowMs() - warm.askedMs >= warmUpKeptMs) {
      warm.preload?.stop();
      return this.ask(fileId, { playableFormats, cause: "warmUpStale", startsAtMs });
    }
    // Its start is timed from Play, not from opening the page: what the person waits for.
    const pressedMs = performance.now();
    return warm.decision.then(
      (decision) => {
        const settledAtBps = warm.preload?.settledAtBps();
        const preloadSteps = warm.preload?.steps();
        warm.preload?.stop();
        if (decision.onLadder === undefined) {
          // The warm-up's playback is let go of: the server worked on both (the M4 closing audit).
          this.tokens.delete(decision.mediaToken);
          this.leave(decision.mediaToken);
          return this.ask(fileId, { playableFormats, cause: "warmUpOffLadder", startsAtMs });
        }
        noteDecisionTimes(decision.mediaToken, {
          askedMs: pressedMs,
          decidedMs: performance.now(),
          preloadSteps,
        });
        return settledAtBps === undefined
          ? decision
          : { ...decision, onLadder: withThePagesStart(decision.onLadder, settledAtBps) };
      },
      // A warm-up that failed is asked again at Play, as its page left it to be: it failed the
      // first press of Play instead (the M4 closing audit, 2026-10-08).
      () => this.ask(fileId, { playableFormats, cause: "warmUpFailed", startsAtMs }),
    );
  }

  /**
   * Where fileId resumes for its person, as its film page's warm-up was told (StartPlaybackOutput's
   * resumePositionMs), 0 for its start; undefined where it is not the film warmed up or its warm-up
   * failed. A title's page names its Play by it, Resume with the point, asking nothing more.
   */
  async warmResumeMs(fileId: number): Promise<number | undefined> {
    const warm = this.warm;
    if (this.fileId !== fileId || warm === undefined) {
      return undefined;
    }
    try {
      return (await warm.decision).resumePositionMs;
    } catch {
      return undefined; // the Play page asks again (decide's warmUpFailed)
    }
  }

  /**
   * The film fileId plays on a cast's receiver, not here (D364 §3): its film page's warm-up, which
   * this device will not take, is let go of, its preload and its playback. A preload left behind
   * kept its hidden video and its chunks on the sender for half an hour (the cast test, 2026-10-27).
   */
  sentAway(fileId: number): void {
    const warm = this.warm;
    if (this.fileId !== fileId || warm === undefined) {
      return;
    }
    this.warm = undefined;
    this.letGoOf(warm);
  }

  private async ask(
    fileId: number,
    asked: StartAsked & { readonly playableFormats: PlayableFormats },
  ): Promise<PlaybackDecision> {
    const answered = await requestPlayback(this.api, fileId, asked);
    const decision =
      answered.onLadder !== undefined && startsOnFilmsPicture()
        ? { ...answered, onLadder: withThePagesStart(answered.onLadder, topOfTheLadderBps) }
        : answered;
    if (this.fileId === fileId) {
      this.tokens.add(decision.mediaToken);
    } else {
      // Another film was chosen while this one was asked for.
      this.leave(decision.mediaToken);
    }
    return decision;
  }

  /** Lets go of a warm-up the Play page does not take: its preload, and its playback once decided. */
  private letGoOf(warm: Warm): void {
    warm.preload?.stop();
    warm.decision.then(
      (decision) => {
        this.tokens.delete(decision.mediaToken);
        this.leave(decision.mediaToken);
      },
      () => undefined,
    );
  }

  /** Makes fileId the film watched, letting go of the playbacks of the one before. */
  private watch(fileId: number): void {
    if (this.fileId === fileId) {
      return;
    }
    for (const token of this.tokens) {
      this.leave(token);
    }
    this.tokens.clear();
    this.warm?.preload?.stop();
    this.warm = undefined;
    this.fileId = fileId;
  }

  private leave(token: string): void {
    // Every leave here is for another film, or another playback of this one: its runs stop at once.
    leavePlayback(this.api, token, "replaced");
  }
}

/** A film page's decision, until the Play page takes it, when it was asked for, and its preload. */
interface Warm {
  readonly decision: Promise<PlaybackDecision>;
  readonly askedMs: number;
  preload: Preload | undefined;
}

const watchingByApi = new WeakMap<Api, Watching>();

/** The tab's Watching, one per API client, so each test's is its own. */
export function watchingOf(api: Api): Watching {
  let watching = watchingByApi.get(api);
  if (watching === undefined) {
    watching = new Watching(api);
    watchingByApi.set(api, watching);
  }
  return watching;
}
