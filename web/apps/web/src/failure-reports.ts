// SPDX-License-Identifier: MPL-2.0

/**
 * The page's failures, sent to the core's log as well as the console: on a phone no one reads the
 * console, and a playback that started over by itself left nothing to say why (staging, 2026-10-14).
 */

import { sendFailuresTo, type Api } from "@orpheum/sdk";

import type { Session } from "./session.ts";

/** The longest page path the core takes with a failure. */
const pageMost = 200;

/**
 * Sends each failure the page reports to the core over api while session is signed in, with the
 * page's path. A failure to send it is dropped, not reported, or one failure would send another.
 */
export function sendFailuresToCore(api: Api, session: Session, page: () => string): void {
  sendFailuresTo((message) => {
    if (session.token() === undefined) {
      return;
    }
    api
      .POST("/client-problems", { body: { message, page: page().slice(0, pageMost) } })
      .catch(() => undefined);
  });
}
