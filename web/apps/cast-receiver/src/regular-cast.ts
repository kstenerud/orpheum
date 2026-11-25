// SPDX-License-Identifier: MPL-2.0

import { castAsGuest, guestRequestOf, reachOfGrant, type GuestAnswer } from "./guest.ts";
import type { ReceiverLog } from "./log.ts";

/**
 * The receiver's regular cast (D555 §3): a launch on `urn:x-cast:org.orpheum.cast` from the app's
 * regular pages, under a grant for that cast (guest.ts, D465 §2), taken in place of any cast it
 * takes now, its log shown where the sender says so.
 */
export class RegularCasts {
  /** The cast it takes now, while it takes one: its stopping. */
  private stopCast: (() => void) | undefined;

  constructor(
    private readonly video: HTMLVideoElement,
    private readonly log: ReceiverLog,
  ) {}

  /**
   * Takes the cast a sender's launch message asks for, in place of any it takes now, answering the
   * sender by answer, sent home by goHome as a Watch Together guest's part it took ends (D518 §2).
   */
  heard(message: unknown, answer: (message: GuestAnswer) => void, goHome: () => void): void {
    const { log } = this;
    const request = guestRequestOf(message);
    if (request === undefined) {
      log.note(`a cast it does not know: ${JSON.stringify(message).slice(0, 200)}`);
      return;
    }
    if (request.showsLog) {
      log.showLog();
    }
    this.stopCast?.();
    this.stopCast = undefined;
    const note = (line: string): void => {
      log.note(line);
    };
    void castAsGuest(request, {
      reach: reachOfGrant(request, note),
      video: this.video,
      note,
      answer,
      goHome,
    }).then((stop) => {
      this.stopCast = stop;
    });
  }
}
