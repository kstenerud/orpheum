// SPDX-License-Identifier: MPL-2.0

/**
 * The types AirPlay's grants are asked and revoked by (D473, D519 §1, D521 §2), apart from
 * airplay.ts so that airplay-here.ts, which the home page loads, names them without importing it.
 */

import type { Api } from "@orpheum/sdk";

/** A line for the page's log. */
export type Log = (line: string) => void;

/** The grant made for a playback: its master playlist's whole address, and when it ends. */
export interface AirPlayGrant {
  readonly url: string;
  readonly expiresAt: string;
}

/** What a step is given: the server, and the log it writes each step to. */
export interface AirPlayDoing {
  readonly api: Api;
  readonly log: Log;
}

/**
 * What a grant is asked for: the playback its media token names, and the audio track the page plays
 * now, by its place among the film's, which the grant's master playlist names its default (D521 §2);
 * none, the playback's starting track.
 */
export interface AirPlayGrantAsked {
  readonly mediaToken: string;
  readonly audioTrackIndex?: number | undefined;
}

/** How AirPlay's grant is asked for (make) and revoked at AirPlay's end (revoke), and by whom. */
export interface AirPlayGrants {
  readonly make: (
    doing: AirPlayDoing,
    asked: AirPlayGrantAsked,
    origin: string,
  ) => Promise<AirPlayGrant>;
  readonly revoke: (doing: AirPlayDoing) => Promise<number>;
}
