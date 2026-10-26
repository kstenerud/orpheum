// SPDX-License-Identifier: MPL-2.0

import { diagnosisOf, playChunked } from "@orpheum/player";
import type { Api, components, PlayableFormats } from "@orpheum/sdk";

import { browserStorage } from "./browser-storage.ts";
import { startOn, type LadderOffer } from "./moving-down.ts";
import { noteDecisionTimes } from "./play-state.ts";
import { leavePlayback } from "./interest.ts";
import { dataOrThrow } from "./problem.ts";
import { monotonicMs } from "./use-together.ts";

/** The server's decision on how a file plays here (cut-a §6). */
export type PlaybackDecision = components["schemas"]["StartPlaybackOutputBody"];

/**
 * Why a page asks the server for a playback, for the server's log alone: a phone's playback started
 * over by itself and nothing said what started it (staging, 2026-10-14).
 */
export type StartCause = NonNullable<components["schemas"]["StartPlaybackInputBody"]["cause"]>;

/** How many playbacks this page has asked for since it loaded, which the server logs beside each. */
const pageStarts = { count: 0 };

/** Asks the server how a file plays in this browser, and why it asks; nothing of where it is (D247 §11). */
export async function requestPlayback(
  api: Api,
  fileId: number,
  asked: { readonly playableFormats: PlayableFormats; readonly cause: StartCause },
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
        // The person chose the film's own file over its ladder.
        ...(asked.cause === "originalChosen" ? { preferredDelivery: "direct" as const } : {}),
      },
    }),
  );
  noteDecisionTimes(decision.mediaToken, { askedMs, decidedMs: performance.now() });
  return decision;
}

/**
 * How long a film page's decision may wait for its Play page before a new one is asked for: half
 * the least media_token_idle_expiry_hours can be, an hour, so its token is never idled out by the
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
 * How far ahead a film page's hidden player loads: its first buffer, one chunk of the ladder's
 * ten-second grid, not the half minute hls.js loads by itself. Three or more chunks of a film's
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

/** Starts a Preload of the film's ladder from where its playback resumes. */
export type StartPreload = (offer: LadderOffer, positionMs: number) => Preload;

/**
 * A Preload in a muted video the page never shows: hls.js loads the first buffer from the offered
 * quality and steps down by itself if the link cannot fill it, and the chunks it loads are kept by
 * the browser for the Play page's player. One that fails is the Play page's to try again.
 */
export const preloadInHiddenVideo: StartPreload = (offer, positionMs) => {
  const video = document.createElement("video");
  video.muted = true;
  video.hidden = true;
  video.playsInline = true;
  document.body.append(video);
  const playback = playChunked(video, offer.url, {
    onFailed: () => undefined,
    ...startOn(offer, positionMs),
    monotonicMs,
    loadsAheadAtMostS: preloadAheadS,
  });
  return {
    settledAtBps: () => playback.settledAtBps(),
    steps: () => diagnosisOf(video)?.steps,
    stop: () => {
      playback.destroy();
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
 * on its ladder is preloaded too, and the Play page's player starts at the quality the preload
 * settled on, its chunks already in the browser (D260 §3).
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

  /** A film's page was opened: the film is warmed up, and any other one let go of. */
  warmUp(fileId: number, playableFormats: PlayableFormats): void {
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
        if (this.warm === warm && decided.onLadder !== undefined && !startsOnFilmsPicture()) {
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
   * may be on its ladder now (D260 §1). Another film being watched is let go of first.
   */
  decide(
    fileId: number,
    playableFormats: PlayableFormats,
    cause: StartCause = "playPage",
  ): Promise<PlaybackDecision> {
    this.watch(fileId);
    const warm = this.warm;
    this.warm = undefined;
    if (warm === undefined) {
      return this.ask(fileId, { playableFormats, cause });
    }
    if (this.nowMs() - warm.askedMs >= warmUpKeptMs) {
      warm.preload?.stop();
      return this.ask(fileId, { playableFormats, cause: "warmUpStale" });
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
          return this.ask(fileId, { playableFormats, cause: "warmUpOffLadder" });
        }
        noteDecisionTimes(decision.mediaToken, {
          askedMs: pressedMs,
          decidedMs: performance.now(),
          preloadSteps,
        });
        return settledAtBps === undefined
          ? decision
          : { ...decision, onLadder: { ...decision.onLadder, startAtBps: settledAtBps } };
      },
      // A warm-up that failed is asked again at Play, as its page left it to be: it failed the
      // first press of Play instead (the M4 closing audit, 2026-10-08).
      () => this.ask(fileId, { playableFormats, cause: "warmUpFailed" }),
    );
  }

  private async ask(
    fileId: number,
    asked: { readonly playableFormats: PlayableFormats; readonly cause: StartCause },
  ): Promise<PlaybackDecision> {
    const answered = await requestPlayback(this.api, fileId, asked);
    const decision =
      answered.onLadder !== undefined && startsOnFilmsPicture()
        ? { ...answered, onLadder: { ...answered.onLadder, startAtBps: topOfTheLadderBps } }
        : answered;
    if (this.fileId === fileId) {
      this.tokens.add(decision.mediaToken);
    } else {
      // Another film was chosen while this one was asked for.
      this.leave(decision.mediaToken);
    }
    return decision;
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
