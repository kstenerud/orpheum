// SPDX-License-Identifier: MPL-2.0

/**
 * The wait before a player that is not a follower plays: alone, a cast's receiver, a session's
 * leader. It starts, and plays on after a seek, only once what it has loaded ahead of where it is
 * outlasts loading its next chunk, with a margin (readyAheadOf, the rule a follower is ready by after
 * a seek; the owner, 2026-10-28), picture and sound both, the element's buffered being what the two
 * hold in common. The browser plays at canplay: a Fire TV's Silk resumed a series at
 * 1798.9 s with its first chunk loaded to 1800.47 s, played, ran dry 1.1 s in while the next chunk,
 * 10.4 s of film, took 2.3 s to come, and stalled (the owner's cast, staging, 2026-10-28;
 * idiosyncrasies.md). The wait comes before the play, never a play paused and played again: on Silk
 * each pause, play and seek costs a refill.
 */

import { readyAheadOf, type ReadyAhead } from "./ready-ahead.ts";
import {
  hasRefilled,
  haveFutureData,
  loadedEndMs,
  networkIdle,
  type FollowedMedia,
} from "./together-media.ts";
import { playerLoadedChunkEvent, playerMovedItselfEvent } from "./together-player-events.ts";

/**
 * How often a waiting play looks again at what is loaded, in ms, beside the element's own events: a
 * chunk hls.js appends fires no progress in every browser. A justified constant: a quarter of a
 * second is too short to notice beside a chunk's load it waits on.
 */
export const startCheckEveryMs = 250;

/** The part of a video or audio element the start gate reads and plays. */
export type GatedMedia = Pick<
  FollowedMedia,
  | "currentTime"
  | "paused"
  | "ended"
  | "seeking"
  | "readyState"
  | "networkState"
  | "buffered"
  | "duration"
  | "play"
  | "pause"
  | "addEventListener"
  | "removeEventListener"
>;

/**
 * What a start's or a seek's play waited for: the film loaded ahead it was ready on, and the next
 * chunk's expected load it was to outlast, where its player said one.
 */
export interface StartWait {
  readonly waitedForAheadMs: number;
  readonly nextChunkLoadMs?: number;
  /** What the next chunk's expected load rested on; "none" where the wait was refillAheadMs. */
  readonly nextChunkLoadFrom: ReadyAhead["nextChunkLoadFrom"];
}

/** How much film is loaded ahead of where media is, in ms, read on across a hole hls.js steps over. */
export function loadedAheadMsOf(media: Pick<GatedMedia, "buffered" | "currentTime">): number {
  const endMs = loadedEndMs(atFirstLoaded(media));
  return endMs === undefined ? 0 : Math.max(0, Math.round(endMs - media.currentTime * 1000));
}

/**
 * How far before the first stretch it loaded a starting player may stand and still start on it, in
 * seconds: a film's first picture is stamped a little after 0 (0.08 s on staging's films), so a film
 * started from its beginning stood before what it loaded, read nothing loaded, and its start's wait
 * never ended, a paused player stepping over no gap (season 2's first episode on the Mac and a film
 * on the Fire TV, staging, 2026-10-28). A justified constant: once playing, hls.js steps the video
 * onto its first loaded picture, as it did with the autoplay before this gate; half a second covers a
 * first picture held back by reordered frames. A follower steps onto it itself (playableAt).
 */
export const startsBeforeFirstLoadedS = 0.5;

/**
 * The parts of media hasRefilled reads, as from its first loaded picture where it stands just before
 * it: the browser counts the position in the gap as having no data there, which a playing video
 * leaves at once.
 */
function startingOnFirstLoaded(
  media: GatedMedia,
): Pick<GatedMedia, "readyState" | "networkState" | "buffered" | "currentTime" | "duration"> {
  const at = atFirstLoaded(media);
  const read = pickRefill(media);
  return at === media
    ? read
    : {
        ...read,
        currentTime: at.currentTime,
        readyState: Math.max(read.readyState, haveFutureData),
      };
}

/** The parts of media hasRefilled reads. */
function pickRefill(
  media: GatedMedia,
): Pick<GatedMedia, "readyState" | "networkState" | "buffered" | "currentTime" | "duration"> {
  return {
    readyState: media.readyState,
    networkState: media.networkState,
    buffered: media.buffered,
    currentTime: media.currentTime,
    duration: media.duration,
  };
}

/** media as read from its first loaded picture where it stands just before it, else as it is. */
function atFirstLoaded(
  media: Pick<GatedMedia, "buffered" | "currentTime">,
): Pick<GatedMedia, "buffered" | "currentTime"> {
  const { buffered, currentTime } = media;
  if (buffered.length === 0) {
    return media;
  }
  const firstS = buffered.start(0);
  return currentTime < firstS && firstS - currentTime <= startsBeforeFirstLoadedS
    ? { buffered, currentTime: firstS }
    : media;
}

/** The element's events at which a waiting play looks again. */
const lookAgainOn = [
  "progress",
  "loadeddata",
  "canplay",
  "canplaythrough",
  "seeked",
  "durationchange",
  "suspend",
  playerLoadedChunkEvent,
] as const;

/** A wait as its log says it, from what readyAheadOf said as it ended. */
function waitOf(ready: ReadyAhead): StartWait {
  const from = ready.nextChunkLoadFrom;
  return ready.nextChunkLoadMs === undefined
    ? { waitedForAheadMs: ready.aheadMs, nextChunkLoadFrom: from }
    : {
        waitedForAheadMs: ready.aheadMs,
        nextChunkLoadMs: ready.nextChunkLoadMs,
        nextChunkLoadFrom: from,
      };
}

/**
 * One element's wait before it plays (startGateOf). A page or a leader asks it to play; it plays at
 * once when enough is loaded ahead, else once it is. A playing element that seeks to where too little
 * is loaded is paused as it seeks, before it can play there, and played again once enough is.
 */
export class StartGate {
  readonly #media: GatedMedia;
  /** The play to make once enough is loaded; undefined while none waits. */
  #pending: (() => void) | undefined = undefined;
  /** The play the gate made last, which plays the element on after a seek it held. */
  #lastPlay: (() => void) | undefined = undefined;
  /** Whether the gate paused the element at a seek, holding it until enough is loaded. */
  #isHolding = false;
  /** Whether the next seeking is the player's own step over a gap, never held. */
  #isNextSeekThePlayers = false;
  /** What the gate's last play after a wait waited for, until a seek or a pause. */
  #wait: StartWait | undefined = undefined;
  #checker: ReturnType<typeof setInterval> | undefined = undefined;
  /** While any of these says so, the element follows a group, whose follower moves it. */
  readonly #followings = new Set<() => boolean>();
  /** While any of these says so, the element is asked to play by its group, though it is paused. */
  readonly #askings = new Set<() => boolean>();

  constructor(media: GatedMedia) {
    this.#media = media;
    for (const event of lookAgainOn) {
      media.addEventListener(event, this.#lookAgain);
    }
    media.addEventListener(playerMovedItselfEvent, this.#movedItself);
    media.addEventListener("seeking", this.#seeking);
    media.addEventListener("play", this.#played);
    media.addEventListener("pause", this.#paused);
  }

  /**
   * Plays the element as how does: at once when enough is loaded ahead, else once it is. Only a
   * paused element is played; one that follows a group is played at once, its follower moving it.
   * A play asked while another waits takes its place.
   */
  play(how: () => void): void {
    const media = this.#media;
    if (!media.paused) {
      return;
    }
    if (media.ended || this.#isFollowing() || this.#isReady(readyAheadOf(media))) {
      this.#wait = undefined;
      this.#playNow(how);
      return;
    }
    this.#waitToPlay(how);
  }

  /** Gives up a play that waits, and a seek's hold: the element stays paused. */
  cancel(): void {
    this.#pending = undefined;
    this.#isHolding = false;
    this.#stopChecking();
  }

  /** Whether a play waits for enough to be loaded ahead. */
  isWaiting(): boolean {
    return this.#pending !== undefined;
  }

  /** Whether a pause heard now is the gate's own, holding a seek until enough is loaded. */
  isOwnPause(): boolean {
    return this.#isHolding;
  }

  /**
   * What the gate's last play waited for, while the element plays on from it: undefined where it
   * played at once, or has since sought or been paused.
   */
  lastWait(): StartWait | undefined {
    return this.#wait;
  }

  /**
   * Stands aside while isFollowing says the element follows a group, whose follower plays, pauses and
   * seeks it itself; a play waiting then is given up. Returns the end of it.
   */
  standAsideWhile(isFollowing: () => boolean): () => void {
    this.#followings.add(isFollowing);
    return () => {
      this.#followings.delete(isFollowing);
    };
  }

  /**
   * Counts the element asked to play while isAsking says so, as a group that plays or waits for its
   * players asks a follower it holds paused while it loads. Returns the end of it.
   */
  asksToPlayWhile(isAsking: () => boolean): () => void {
    this.#askings.add(isAsking);
    return () => {
      this.#askings.delete(isAsking);
    };
  }

  /**
   * Whether the element is asked to play though it may be paused: a play waits for enough to be
   * loaded ahead, or its group asks it (asksToPlayWhile).
   */
  isAskedToPlay(): boolean {
    return this.isWaiting() || [...this.#askings].some((isAsking) => isAsking());
  }

  #isFollowing(): boolean {
    return [...this.#followings].some((isFollowing) => isFollowing());
  }

  /**
   * Whether enough is loaded ahead to play by ready (hasRefilled, as a follower after a seek), or
   * the browser loads no more while the element is paused, so waiting gains nothing.
   */
  #isReady(ready: ReadyAhead): boolean {
    const media = this.#media;
    return (
      media.networkState === networkIdle || hasRefilled(startingOnFirstLoaded(media), ready.aheadMs)
    );
  }

  #waitToPlay(how: () => void): void {
    this.#pending = how;
    this.#checker ??= setInterval(this.#lookAgain, startCheckEveryMs);
  }

  #playNow(how: () => void): void {
    this.#lastPlay = how;
    this.#isHolding = false;
    how();
  }

  readonly #lookAgain = (): void => {
    const how = this.#pending;
    if (how === undefined) {
      this.#stopChecking();
      return;
    }
    if (this.#isFollowing()) {
      this.cancel();
      return;
    }
    const ready = readyAheadOf(this.#media);
    if (!this.#media.paused || !this.#isReady(ready)) {
      return;
    }
    this.#pending = undefined;
    this.#stopChecking();
    this.#wait = waitOf(ready);
    this.#playNow(how);
  };

  #stopChecking(): void {
    clearInterval(this.#checker);
    this.#checker = undefined;
  }

  readonly #movedItself = (): void => {
    this.#isNextSeekThePlayers = true;
  };

  /**
   * A seek: one the player makes over a gap plays on as it is; one of a playing element to where too
   * little is loaded is paused, before the element can play there, and played on once enough is.
   */
  readonly #seeking = (): void => {
    const isThePlayers = this.#isNextSeekThePlayers;
    this.#isNextSeekThePlayers = false;
    this.#wait = undefined;
    const media = this.#media;
    if (
      isThePlayers ||
      media.paused ||
      media.ended ||
      this.#isFollowing() ||
      loadedAheadMsOf(media) >= readyAheadOf(media).aheadMs
    ) {
      return;
    }
    this.#isHolding = true;
    media.pause();
    this.#waitToPlay(
      this.#lastPlay ??
        ((): void => {
          media.play().catch(() => undefined);
        }),
    );
  };

  /**
   * The element plays: a play that waited is no longer needed. One the gate did not make, as its
   * person's press, plays now, whatever is loaded. A play already paused again as it is heard played
   * nothing, and the wait stays: the Play page's press kept for its start (keepThePressForTheStart in
   * the web app), played and paused at once, gave the gate's wait up, and the owner's next episode
   * opened paused at 0:00, it seems in a full-screen browser (production, 2026-11-11).
   */
  readonly #played = (): void => {
    if (this.#media.paused) {
      return;
    }
    if (this.#pending !== undefined) {
      this.#pending = undefined;
      this.#wait = undefined;
      this.#stopChecking();
    }
    this.#isHolding = false;
  };

  readonly #paused = (): void => {
    if (!this.#isHolding) {
      this.#wait = undefined;
    }
  };
}

/** Each element's start gate, made as it is first asked for. */
const gates = new WeakMap<object, StartGate>();

/**
 * Whether media, paused, is still asked to play: its start gate's play waits, or its group asks it
 * (StartGate.isAskedToPlay). An element with no start gate is asked by nothing but its own playing:
 * a film page's hidden player, loading its first seconds before Play, is not.
 */
export function isAskedToPlay(media: object): boolean {
  return gates.get(media)?.isAskedToPlay() ?? false;
}

/** The element's start gate: one for each element, shared by its page and its session's leader. */
export function startGateOf(media: GatedMedia): StartGate {
  let gate = gates.get(media);
  if (gate === undefined) {
    gate = new StartGate(media);
    gates.set(media, gate);
  }
  return gate;
}
