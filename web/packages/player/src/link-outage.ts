// SPDX-License-Identifier: MPL-2.0

/**
 * The page's memory of its link's outages during its films (D487): from the first sign that its link
 * was lost (a load that failed or stopped, a chunk stuck on its way, the browser offline) to the next
 * chunk of picture that arrived, or to the end of the playback that waited. One that lasted
 * deeperAfterOutageAtLeastMs or more has a film load deeper ahead (ForwardBuffer), until the link has
 * stayed good for deeperForMs. It is the page's, not a playback's, so a playback opened again after its
 * outage (the core restarted meanwhile: playbackGone) loads deeper too, and so does the next film.
 */

/**
 * The least an outage lasts, in milliseconds, for a playback to load deeper ahead after it: longer
 * than a WebRTC connection's replacement, half a second, with hls.js's two asks again after it, 1 s
 * then 2 s (chunk-load-policy.ts's unansweredAskAgainCount; the owner's Firefox, staging,
 * 2026-11-12), and a third of the 30 s a playback holds below its top (loadsAheadBelowTopS): an outage
 * shorter than that was covered by what was held. A justified constant, the lead's (D487 §1).
 */
export const deeperAfterOutageAtLeastMs = 10_000;

/**
 * How long, in milliseconds, after its last outage ended a playback loads deeper ahead: ten minutes
 * of a link that stayed good. A dead zone comes back on a journey within minutes (the stretch
 * between two towers, the next tunnel); the cost of staying deep is a step-up shown only once what
 * was loaded plays out, and bytes loaded that a person who stops never sees. Longer than the
 * 6.4 minutes the owner's phone was out mid-film (staging, 2026-11-14), short enough that a phone
 * home on Wi-Fi is back to its 30 s within an episode. A justified constant, the lead's (D487 §2),
 * of low confidence until VQ332 shows it on a journey.
 */
export const deeperForMs = 10 * 60_000;

/** An outage that ended: how long it lasted, when it ended, and the first sign of it. */
export interface Outage {
  readonly lastedMs: number;
  readonly endedAtMs: number;
  readonly why: string;
}

/** The page's outages during its films (D487), each time told by nowMs, a monotonic clock's. */
export class LinkOutages {
  /** The outage under way: since when, and the first sign of it. */
  #lost: { readonly sinceMs: number; readonly why: string } | undefined;
  /** The outage that has a playback load deeper: the last that did, or that ended while one did. */
  #deeper: Outage | undefined;

  /** A sign that the link was lost, for why: the first since the link was last back starts an outage. */
  lost(why: string, nowMs: number): void {
    this.#lost ??= { sinceMs: nowMs, why };
  }

  /**
   * The link is back, a chunk of picture having arrived (or the playback that waited ended): the
   * outage under way ends. One of deeperAfterOutageAtLeastMs or more, or any while it loads deeper,
   * is the one that has it load deeper from now. Returns it, where it starts the deeper loading.
   */
  back(nowMs: number): Outage | undefined {
    const lost = this.#lost;
    if (lost === undefined) {
      return undefined;
    }
    const wasDeeper = this.deeperAt(nowMs) !== undefined;
    this.#lost = undefined;
    const outage = { lastedMs: nowMs - lost.sinceMs, endedAtMs: nowMs, why: lost.why };
    if (outage.lastedMs < deeperAfterOutageAtLeastMs && !wasDeeper) {
      return undefined;
    }
    this.#deeper = outage;
    return wasDeeper ? undefined : outage;
  }

  /**
   * The outage that has a playback load deeper at nowMs: the last long one, until the link has stayed
   * good deeperForMs since it ended, an outage under way stopping that count; undefined for none.
   */
  deeperAt(nowMs: number): Outage | undefined {
    const deeper = this.#deeper;
    // Through an outage under way, the link's time good is counted to its start.
    const goodUntilMs = this.#lost?.sinceMs ?? nowMs;
    return deeper !== undefined && goodUntilMs - deeper.endedAtMs < deeperForMs
      ? deeper
      : undefined;
  }
}

/**
 * The page's own: one for every film played in it, so a playback opened again after an outage, or the
 * next film, knows of it. A page loaded again starts with none.
 */
export const pageLinkOutages = new LinkOutages();

/** An outage's words for a note: "out 381.3 s (its loading stopped)". */
export function outageText(outage: Outage): string {
  return `out ${(outage.lastedMs / 1000).toFixed(1)} s (${outage.why})`;
}
