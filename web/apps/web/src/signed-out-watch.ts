// SPDX-License-Identifier: MPL-2.0

import type { Api } from "@orpheum/sdk";

import type { Navigation } from "./navigation.ts";
import type { Session } from "./session.ts";

/** A watcher of every request and answer the API makes. */
type Middleware = Parameters<Api["use"]>[0];

/** A named field of an object, when value is one. */
function field(value: unknown, name: string): unknown {
  if (typeof value !== "object" || value === null || !(name in value)) {
    return undefined;
  }
  const found: unknown = Object.getOwnPropertyDescriptor(value, name)?.value;
  return found;
}

/** The reason an auth_required answer gives, or no_session when it gives none. */
export function signedOutReason(body: unknown): string {
  const reason = field(field(field(body, "error"), "detail"), "reason");
  return typeof reason === "string" ? reason : "no_session";
}

/**
 * Watches every answer for a session the server no longer knows: a 401 to a request that carried
 * one. The session ends with the server's reason, which the sign-in screen tells the person, and
 * the client goes home, so signing in again does not land on the page that failed.
 */
export function watchForSignOut(session: Session, navigation: Navigation): Middleware {
  return {
    async onResponse({ request, response }) {
      const hasSession = request.headers.has("Authorization") && session.token() !== undefined;
      if (response.status !== 401 || !hasSession) {
        return undefined;
      }
      let reason = "no_session";
      try {
        reason = signedOutReason(await response.clone().json());
      } catch {
        // Not our error shape: the reason stays unknown.
      }
      navigation.go({ screen: "films" });
      session.endBecause(reason);
      return undefined;
    },
  };
}
