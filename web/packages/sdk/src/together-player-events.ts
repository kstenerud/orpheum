// SPDX-License-Identifier: MPL-2.0

/**
 * The events a player fires at its video for the follower of its group: that it moved itself, fell
 * behind, loaded a chunk or switched quality, and what the last two carry; and the one the follower
 * fires at it, to hold its loading while the group waits.
 */

import type { GroupView } from "./together.ts";
import type { TogetherConnection } from "./together-connection.ts";
import type { SoundMovedBy } from "./together-notes.ts";

/**
 * The event a player fires at its video as it moves the video itself, before the seeking that
 * follows: hls.js steps over a gap in what it loaded, or nudges a stalled video on. That seek is
 * the player's, not its person's, and a leading player does not send it to the group (the owner's
 * test, 2026-10-05, where each such step became a jump for everyone).
 */
export const playerMovedItselfEvent = "orpheumplayermoveditself";

/**
 * The event a player fires at its video as the first chunk of each of its playlists, the picture's
 * ("main") and the sound's ("audio"), is in its buffer: the follower's page notes what the player
 * holds then and how hls.js placed it, since an iPhone that jumped 10 s at its start, the chunks'
 * offset, left only a guess at whether that placing was wrong (the sync review, 2026-10-08).
 */
export const playerBufferedFirstEvent = "orpheumplayerbufferedfirst";

/**
 * The event a following player fires at its video as it seeks to catch up with a group it fell
 * behind, having once been in step: a stall in all but name, which a player on a slow link never
 * otherwise counts, every load being a seek's (D247 §2; the owner's test, 2026-10-05).
 */
export const fellBehindEvent = "orpheumfellbehind";

/**
 * The event a following player fires at its video just before it seeks to catch up with its group:
 * that seek is no person's or group's jump, and its player does not say a seek took it there, so it
 * waits for the run already making its quality (D250 §5, D284 §2).
 */
export const catchUpSeekEvent = "orpheumcatchupseek";

/**
 * The event a following player fires at its video as its group jumps: a player on a quality the
 * server makes starts the jump on the group's common one, one encode for all, and settles after
 * (D286 §3).
 */
export const groupJumpedEvent = "orpheumgroupjumped";

/** The jumps of its group a following player told its player of, each once (groupJumpedEvent). */
export class GroupJumpsTold {
  /** The last told, as "<from>-<to>" ms; undefined before one. */
  #told: string | undefined = undefined;

  /**
   * Tells media of the group's jump, when its last change is one not told yet; a leading player's
   * (isLeading) is only marked told: it plays as a device plays alone (D371 §1), and a cast's
   * receiver, leading, dropped from the film's own 9 Mb/s to 1.9 Mb/s on its link of 18 Mb/s at each
   * jump of its own (the owner's Fire TV, staging, 2026-10-29).
   */
  tell(
    media: Pick<EventTarget, "dispatchEvent">,
    group: Pick<GroupView, "event" | "seekedFromMs" | "positionMs">,
    isLeading = false,
  ): void {
    const jump =
      group.event === "seeked"
        ? `${String(group.seekedFromMs)}-${String(group.positionMs)}`
        : undefined;
    if (jump !== undefined && jump !== this.#told) {
      this.#told = jump;
      if (!isLeading) {
        media.dispatchEvent(new Event(groupJumpedEvent));
      }
    }
  }
}

/**
 * The event a player fires at its video as it loads a chunk: a ChunkLoaded in its detail, which a
 * following player tells the server for its log.
 */
export const playerLoadedChunkEvent = "orpheumplayerloadedchunk";

/** One chunk a player loaded, as playerLoadedChunkEvent tells it. */
export interface ChunkLoaded {
  readonly track: "video" | "audio";
  readonly loadMs: number;
  readonly loadedKbps: number;
  /** The quality the chunk is of, in kbit/s. */
  readonly qualityKbps: number;
  /** How much film the chunk plays, in ms: with loadMs, whether the link keeps up with the film. */
  readonly filmMs: number;
}

/**
 * The event a player fires at its video as it begins playing another quality: a QualitySwitched
 * in its detail, which a following player tells the server for its log.
 */
export const playerSwitchedQualityEvent = "orpheumplayerswitchedquality";

/**
 * The longest a QualitySwitched's why and linkSamples may be, as the server takes them
 * (diagnosisTextMost): a longer one is cut to it.
 */
export const qualitySwitchedTextMost = 300;

/**
 * A quality a player began playing, as playerSwitchedQualityEvent tells it, with the link's
 * estimate then: undefined before chunks measured it, when the estimate is only hls.js's starting
 * value; the quality it played before, absent for the first it plays; and, where its own bitrate
 * controller chose it, why ("first", a chunk given up with its bytes and wait, a step up, the link)
 * and the link's samples then. A Fire TV fell from 9018 to 696 kb/s 2.3 s into a film, its link
 * estimated at 69 kb/s though its chunks had loaded at 14–18 Mb/s, and the core's log said nothing
 * of why (the owner's cast, staging, 2026-10-28).
 */
export interface QualitySwitched {
  readonly qualityKbps: number;
  readonly linkEstimateKbps: number | undefined;
  readonly fromQualityKbps?: number;
  readonly why?: string;
  readonly linkSamples?: string;
}

/** The detail an event carries, when it is a CustomEvent whose detail has every one of fields. */
function detailWith(event: Event, fields: readonly string[]): Record<string, unknown> | undefined {
  if (!(event instanceof CustomEvent)) {
    return undefined;
  }
  const detail: unknown = event.detail;
  if (typeof detail !== "object" || detail === null) {
    return undefined;
  }
  const record: Record<string, unknown> = { ...detail };
  return fields.every((field) => field in record) ? record : undefined;
}

/** A playerLoadedChunkEvent's chunk, or undefined when the event carries none. */
export function chunkLoadedOf(event: Event): ChunkLoaded | undefined {
  const detail = detailWith(event, ["track", "loadMs", "loadedKbps", "qualityKbps", "filmMs"]);
  const { track, loadMs, loadedKbps, qualityKbps, filmMs } = detail ?? {};
  return (track === "video" || track === "audio") &&
    typeof loadMs === "number" &&
    typeof loadedKbps === "number" &&
    typeof qualityKbps === "number" &&
    typeof filmMs === "number"
    ? { track, loadMs, loadedKbps, qualityKbps, filmMs }
    : undefined;
}

/** Notes for the server's log each switch of quality the player tells of. */
export function noteQualitySwitched(connection: TogetherConnection): (event: Event) => void {
  return (event) => {
    const switched = qualitySwitchedOf(event);
    if (switched !== undefined) {
      connection.note({ kind: "qualitySwitched", ...switched });
    }
  };
}

/** A playerSwitchedQualityEvent's quality, or undefined when the event carries none. */
export function qualitySwitchedOf(event: Event): QualitySwitched | undefined {
  const { qualityKbps, linkEstimateKbps, fromQualityKbps, why, linkSamples } =
    detailWith(event, ["qualityKbps"]) ?? {};
  return typeof qualityKbps === "number" &&
    (linkEstimateKbps === undefined || typeof linkEstimateKbps === "number")
    ? {
        qualityKbps,
        linkEstimateKbps,
        ...(typeof fromQualityKbps === "number" ? { fromQualityKbps } : {}),
        ...(typeof why === "string" && why !== ""
          ? { why: why.slice(0, qualitySwitchedTextMost) }
          : {}),
        ...(typeof linkSamples === "string" && linkSamples !== ""
          ? { linkSamples: linkSamples.slice(0, qualitySwitchedTextMost) }
          : {}),
      }
    : undefined;
}

/**
 * The event a player fires at its video as it moves its sound earlier than its picture, for a
 * device that plays its sound late (audio-shift.ts in the player): a SoundMoved in its detail, which
 * a following player tells the server for its log. A Fire TV's start waited twice as its move for
 * Web Audio's latency threw its sound away, and nothing said when the move changed or by how much
 * (the owner's cast, staging, 2026-10-28).
 */
export const playerMovedSoundEvent = "orpheumplayermovedsound";

/**
 * A move of a player's sound, as playerMovedSoundEvent tells it: from earlierFromMs earlier than
 * its picture to earlierMs, why, the latency Web Audio reported then, absent where the sound is not
 * through it or not yet measured, and whether the move threw the sound loaded away, to load it
 * again at the new move. fromWhy is what moved the sound to earlierFromMs, absent where nothing had;
 * rememberedMs, the latency the device remembered as its levelling began, absent where it
 * remembered none: a move made before the player's log listened is told no one, and the Fire TV's
 * casts of 2026-10-28 left only the moves back to its latency, from where no line said.
 */
export interface SoundMoved {
  readonly earlierFromMs: number;
  readonly earlierMs: number;
  readonly why: SoundMovedBy;
  readonly latencyMs?: number;
  readonly isFlushed: boolean;
  readonly fromWhy?: SoundMovedBy;
  readonly rememberedMs?: number;
}

/** The words a SoundMoved's why may be. */
const soundMovedBys: readonly string[] = ["remembered", "measured", "offset", "direct"];

/** Whether word is a SoundMovedBy. */
function isSoundMovedBy(word: unknown): word is SoundMovedBy {
  return typeof word === "string" && soundMovedBys.includes(word);
}

/** Whether value is a number or absent. */
function isOptionalNumber(value: unknown): value is number | undefined {
  return value === undefined || typeof value === "number";
}

/** Whether value is a SoundMovedBy or absent. */
function isOptionalSoundMovedBy(value: unknown): value is SoundMovedBy | undefined {
  return value === undefined || isSoundMovedBy(value);
}

/** A SoundMoved's fields that may be absent, each only where it is present. */
function soundMovedOptionals(optionals: {
  readonly latencyMs: number | undefined;
  readonly fromWhy: SoundMovedBy | undefined;
  readonly rememberedMs: number | undefined;
}): Pick<SoundMoved, "latencyMs" | "fromWhy" | "rememberedMs"> {
  const { latencyMs, fromWhy, rememberedMs } = optionals;
  return {
    ...(latencyMs === undefined ? {} : { latencyMs }),
    ...(fromWhy === undefined ? {} : { fromWhy }),
    ...(rememberedMs === undefined ? {} : { rememberedMs }),
  };
}

/** A playerMovedSoundEvent's move, or undefined when the event carries none. */
export function soundMovedOf(event: Event): SoundMoved | undefined {
  const { earlierFromMs, earlierMs, why, latencyMs, isFlushed, fromWhy, rememberedMs } =
    detailWith(event, ["earlierFromMs", "earlierMs", "why", "isFlushed"]) ?? {};
  return typeof earlierFromMs === "number" &&
    typeof earlierMs === "number" &&
    isSoundMovedBy(why) &&
    typeof isFlushed === "boolean" &&
    isOptionalNumber(latencyMs) &&
    isOptionalSoundMovedBy(fromWhy) &&
    isOptionalNumber(rememberedMs)
    ? {
        earlierFromMs,
        earlierMs,
        why,
        isFlushed,
        ...soundMovedOptionals({ latencyMs, fromWhy, rememberedMs }),
      }
    : undefined;
}

/** Notes for the server's log each move of its sound the player tells of. */
export function noteSoundMoved(connection: TogetherConnection): (event: Event) => void {
  return (event) => {
    const moved = soundMovedOf(event);
    if (moved !== undefined) {
      connection.note({ kind: "soundMoved", ...moved });
    }
  };
}

/**
 * The event a following player fires at its video to hold its loading, or let it go on: a
 * LoadingHold in its detail. While its group waits at a sync point and it is ready, it holds, so
 * the players not yet ready have the server's upload to themselves; it goes on as the group plays
 * (the owner's jumps, 2026-10-12, where the ready players went on filling 30 s ahead).
 */
export const holdLoadingEvent = "orpheumholdloading";

/** Whether a player is to hold its loading, as holdLoadingEvent tells it. */
export interface LoadingHold {
  readonly isHeld: boolean;
}

/** The LoadingHold an event carries, if it is one. */
export function loadingHoldOf(event: Event): LoadingHold | undefined {
  const { isHeld } = detailWith(event, ["isHeld"]) ?? {};
  return typeof isHeld === "boolean" ? { isHeld } : undefined;
}

/**
 * The event a following player fires at its video to have it load no more than a runway ahead, or
 * as far as it would: a LoadingRunway in its detail. While another player in its group is still
 * loading, it keeps a short runway, so the one behind has the server's upload (D285 §2).
 */
export const loadingRunwayEvent = "orpheumloadingrunway";

/** How far ahead a player is to load at most, in seconds; undefined for as far as it would. */
export interface LoadingRunway {
  readonly aheadS: number | undefined;
}

/** The LoadingRunway an event carries, if it is one. */
export function loadingRunwayOf(event: Event): LoadingRunway | undefined {
  const detail = detailWith(event, ["aheadS"]);
  if (detail === undefined) {
    return undefined;
  }
  const { aheadS } = detail;
  return aheadS === undefined || typeof aheadS === "number" ? { aheadS } : undefined;
}
