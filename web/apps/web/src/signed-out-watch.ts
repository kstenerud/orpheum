// SPDX-License-Identifier: MPL-2.0

import { problemFromAnswer, type Api } from "@orpheum/sdk";

import type { Navigation } from "./navigation.ts";
import { sessionEndReason } from "./problem.ts";
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
      navigation.go({ screen: "home" });
      session.signOutBecause(sessionEndReason(problem));
      return undefined;
    },
  };
}

/**
 * session, but signing out only once no entry is held over the page (Navigation.whenNoneHeld): a
 * sign-out that leaves the document, as a client origin's to the discovery's sign-in, is then never
 * cancelled by the step back off the entry its sign-out question held, which the question's closing
 * took after the leaving had begun (the owner's Fire TV and Chrome, 2026-11-02).
 */
export function signingOutWithNoneHeld(
  session: Session,
  navigation: Pick<Navigation, "whenNoneHeld">,
): Session {
  return {
    ...session,
    signOut: () => {
      navigation.whenNoneHeld(session.signOut);
    },
    signOutBecause: (reason) => {
      navigation.whenNoneHeld(() => {
        session.signOutBecause(reason);
      });
    },
  };
}

/** How often a visible page asks whether its session still works. */
export const sessionCheckEveryMs = 60_000;

/** A page that says when it is shown or hidden: a browser's document. */
export interface CheckedPage {
  readonly visibilityState: DocumentVisibilityState;
  addEventListener(type: "visibilitychange", listener: () => void): void;
  removeEventListener(type: "visibilitychange", listener: () => void): void;
}

/**
 * Whether the page's live connection is open now, which its provider (live.tsx) marks: while it is,
 * the server tells it a sign-in that ended and its refusal asks (useChecksTheSessionWhenRefused), so
 * the minute's check rests (the efficiency audit of 2026-10-28, finding 4).
 */
export interface LiveOpenMark {
  readonly isOpen: () => boolean;
  /** Marks the live connection open or not. */
  readonly mark: (isOpen: boolean) => void;
}

/** A mark of the live connection, not open until marked. */
export function createLiveOpenMark(): LiveOpenMark {
  let isOpen = false;
  return {
    isOpen: () => isOpen,
    mark: (isNowOpen) => {
      isOpen = isNowOpen;
    },
  };
}

/**
 * Asks the server whether the session still works as the page is shown again and every
 * sessionCheckEveryMs while it is visible and its live connection is not open (told.isLiveOpen), so a
 * device removed elsewhere goes to sign in at once, the answer's 401 handled by watchForSignOut: one
 * removed from another device showed it was signed out only at its person's next click (the owner's
 * VQ7 run, 2026-10-12). Shown again it asks whether the live connection is open or not, which may
 * have died unheard while hidden. Asks nothing while signed out. Returns the stopping.
 */
export function checkSessionWhileShown(
  check: () => Promise<unknown>,
  told: Pick<Session, "token"> & { readonly isLiveOpen: () => boolean },
  page: CheckedPage,
): () => void {
  const ask = (): void => {
    if (told.token() !== undefined && page.visibilityState === "visible") {
      check().catch(() => undefined); // a failure is the next check's, or the next click's
    }
  };
  const onVisibility = (): void => {
    ask();
  };
  const onMinute = (): void => {
    if (!told.isLiveOpen()) {
      ask();
    }
  };
  page.addEventListener("visibilitychange", onVisibility);
  const timer = setInterval(onMinute, sessionCheckEveryMs);
  return () => {
    clearInterval(timer);
    page.removeEventListener("visibilitychange", onVisibility);
  };
}
