// SPDX-License-Identifier: MPL-2.0

import { problemFromAnswer, type Api } from "@orpheum/sdk";

import type { Navigation } from "./navigation.ts";
import type { Session } from "./session.ts";

/** A watcher of every request and answer the API makes. */
type Middleware = Parameters<Api["use"]>[0];

/**
 * Watches every answer for a session the server no longer knows: a 401 to a request that carried
 * one. The session ends with the server's reason, which the sign-in screen tells the person, and
 * the client goes home, so signing in again does not land on the page that failed.
 */
export function watchForSignOut(session: Session, navigation: Navigation): Middleware {
  return {
    async onResponse({ request, response }) {
      const didCarrySession = request.headers.has("Authorization") && session.token() !== undefined;
      if (response.status !== 401 || !didCarrySession) {
        return undefined;
      }
      const problem = await problemFromAnswer(response.clone());
      navigation.go({ screen: "films" });
      session.signOutBecause(problem.detail["reason"] ?? "no_session");
      return undefined;
    },
  };
}
