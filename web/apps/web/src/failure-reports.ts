// SPDX-License-Identifier: MPL-2.0

/**
 * The page's failures, sent to the core's log as well as the console: on a phone no one reads the
 * console, and a playback that started over by itself left nothing to say why (staging, 2026-10-14).
 */

import { sendFailuresTo, type Api, type SentLevel } from "@orpheum/sdk";

import type { Session } from "./session.ts";

/** The longest page path the core takes with a failure. */
const pageMost = 200;

/** The most notes held while signed out for the next sign-in: a sign-in form's few presses. */
const notesHeldMost = 5;

/**
 * Sends each failure the page reports, and each note (noteForTheLog), to the core over api while
 * session is signed in, with the page's path. A failure met signed out is dropped; a note is held,
 * a few, and sent once signed in, so a sign-in form's own notes reach the log (a Fire TV's sign-in
 * that submitted on a checkbox's OK, the owner, 2026-10-30). A failure to send one is dropped, not
 * reported, or one failure would send another.
 */
export function sendFailuresToCore(api: Api, session: Session, page: () => string): void {
  const held: { readonly message: string; readonly page: string }[] = [];
  const send = (message: string, level: SentLevel, at: string): void => {
    const body = { message, page: at.slice(0, pageMost) };
    api
      .POST("/client-problems", { body: level === "debug" ? { ...body, level } : body })
      .catch(() => undefined);
  };
  session.subscribe(() => {
    if (session.token() !== undefined) {
      for (const note of held.splice(0)) {
        send(note.message, "debug", note.page);
      }
    }
  });
  sendFailuresTo((message, level) => {
    if (session.token() !== undefined) {
      send(message, level, page());
    } else if (level === "debug" && held.length < notesHeldMost) {
      held.push({ message, page: page() });
    }
  });
}
