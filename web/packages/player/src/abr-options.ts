// SPDX-License-Identifier: MPL-2.0

import type { HlsProgressivePerformanceTiming, LoaderStats } from "hls.js";

import type { ChoiceLevel, FirstPiece } from "./abr-choice.ts";
import type { RememberedLink } from "./link-estimate.ts";
import type { LongTasks } from "./main-thread.ts";

/**
 * What the chunked player's bitrate controller (abr.ts) is given: the chunks it follows, what it reads
 * of hls.js, what its playback tells it, and what it is given of the link.
 */

/** A chunk as choosing follows it: hls.js's Fragment, or a test's. */
export interface FollowedChunk {
  readonly url: string;
  readonly level: number;
  readonly duration: number;
  readonly stats: Pick<LoaderStats, "loaded" | "total" | "aborted"> & {
    readonly loading: Pick<HlsProgressivePerformanceTiming, "start" | "first" | "end">;
  };
}

/** What choosing reads of hls.js. */
export interface ChoosingHls {
  readonly levels: readonly ChoiceLevel[];
  readonly currentLevel: number;
  readonly loadLevel: number;
  readonly minAutoLevel: number;
  readonly maxAutoLevel: number;
  readonly mainForwardBufferInfo: { readonly len: number } | null;
  /** Whether it has loaded enough to start since its video was attached (resumePositionOf). */
  readonly hasEnoughToStart: boolean;
  /** The position it was asked to start from, or -1 (resumePositionOf). */
  readonly startPosition: number;
  /** The video it plays into, whose position shows whether it plays. */
  readonly media: Pick<
    HTMLMediaElement,
    "buffered" | "currentTime" | "paused" | "readyState"
  > | null;
}

/** What a playback tells its choosing. */
export interface ChoosingOptions {
  /** The highest BANDWIDTH it starts within (D247 §6, D260 §3). */
  readonly startAtBps: number;
  /**
   * Whether its first quality is chosen, before anything loads, from rememberedLink by the rule its
   * every later choice plays by (steadyLevel), in place of startAtBps (D381, amending D257 §2): the
   * server's lower first rung switched to the top as playback began, and the Fire TV's decoder,
   * reset by the new init, showed black frames. startAtBps stays for a player that remembers nothing.
   */
  readonly shouldStartByRememberedLink?: boolean | undefined;
  /** The share of a link a quality may take, as the offer says; linkShareForQuality when absent. */
  readonly shareForQuality?: number | undefined;
  /**
   * The steadiness level it starts at, as the offer says: the one its device last ended at on its
   * network (D352 Q2); 0 when absent.
   */
  readonly startSteadiness?: number | undefined;
  /**
   * The link its device last measured on its network, as the offer says, which its estimate starts
   * from as a prior (bandwidth-sampling-2026-10-29.md P2); absent for none.
   */
  readonly rememberedLink?: RememberedLink | undefined;
  readonly monotonicMs: () => number;
  /** Whether this tab's voice is live, when the film gives way to it (D262 §2). */
  readonly isVoiceLive: () => boolean;
  /**
   * Whether the browser says it is offline, which makes a standing still the link's, never a quality
   * the device cannot play (D266); absent where nothing says.
   */
  readonly isOffline?: () => boolean;
  /** Whether it plays in a Watch Together group, which keeps the film's own picture (D286 §2). */
  readonly isInAGroup?: () => boolean;
  /** Whether the browser took the chunk at url from its cache; undefined where it does not say. */
  readonly fromCache: (url: string) => boolean | undefined;
  /**
   * The first piece of the load of url now, its first progress, which measures nothing of the link
   * (D382 §2); undefined where it is not known, and a chunk is measured from its first byte.
   */
  readonly firstPieceOf?: ((url: string) => FirstPiece | undefined) | undefined;
  /**
   * The browser's long tasks, of 50 ms and more (longTasksOf): a chunk whose arrival one overlapped
   * is no sample of the link and cannot cause a give-up (D382 §3); undefined where it reports none.
   */
  readonly longTasks?: LongTasks | undefined;
  /**
   * Told each change of the ceiling it chooses under, and why, for the diagnosis's steps: a player
   * that dropped from the twin to 360p after a jump on a 77 Mb/s link said nothing of why (the owner's
   * Firefox on nas.example, 2026-10-20).
   */
  readonly noteStep?: (step: string) => void;
  /**
   * Told the link's last samples in a line (samplesText) each time one is taken, for every stall's
   * and seek's report (D352 §5).
   */
  readonly noteSamples?: (samples: string) => void;
  /**
   * Told each note on the link for the next play report, which the server logs: its prior, each
   * sample with its expected time and its bound's, each chunk given up or stuck and what carried it
   * (bandwidth-sampling-2026-10-29.md P6; the owner, 2026-10-29).
   */
  readonly noteLink?: (note: string) => void;
  /**
   * Which transport, and which WebRTC connection of it, carries the load of url now, for the notes
   * of a chunk given up or stuck (the SDK's mediaCarrierOf); absent where nothing says.
   */
  readonly carrierOf?: (url: string) => string;
  /** Told the steadiness level as it starts and at each change, for every play report (D352 Q2). */
  readonly noteSteadiness?: (level: number) => void;
  /**
   * Told the spread of the link, in natural log, with each sample once the link is reported
   * (linkKbps), for every play report, which the core keeps beside the speed (P2).
   */
  readonly noteLinkSpread?: (logSpread: number) => void;
  /** Told of a quality given up as one this device cannot play, and where (D266). */
  readonly onUnplayable?: (unplayable: {
    readonly qualityKbps: number;
    readonly positionMs: number;
    readonly isFilmsOwnPicture: boolean;
  }) => void;
}

/**
 * What choosing is given of the link: how to give a chunk up, and hls.js's estimator to feed, which
 * its other controllers read; choosing chooses by its own estimate (D352 §2).
 */
export interface ChoosingLink<Chunk extends FollowedChunk> {
  readonly giveUp: (chunk: Chunk) => void;
  /**
   * Takes what a chunk given up showed of the link, bytes over durationMs, as a measure of it: the
   * chunk loads no more, and nothing else would measure a link that slowed while the old measure kept
   * a group's player on the film's own picture, given up again every few seconds (the owner's group
   * test, 2026-10-14). hls.js's own abandon resets its measures instead, which ours does not (D265).
   */
  readonly measureGivenUp: (durationMs: number, bytes: number) => void;
  /**
   * Builds the video's decoding anew and loads again from positionS, where the video stood, at the
   * quality chosen next.
   */
  readonly loadAgain: (positionS: number) => void;
}

/**
 * Why the controller chose a quality, told with the switch to it (tellLoading): the reason its step
 * note gives ("first"; a chunk given up, with its bytes, its wait and the estimate after; a step up;
 * the link), and the link's samples then (samplesText). A Fire TV fell from 9018 to 696 kb/s 2.3 s
 * into a film with its link estimated at 69 kb/s, its chunks having loaded at 14–18 Mb/s, and the
 * core's log said nothing of why (the owner's cast, staging, 2026-10-28).
 */
export interface ChoiceWhy {
  readonly why: string;
  /** The link's samples in a line (samplesText); "" before any. */
  readonly linkSamples: string;
}
